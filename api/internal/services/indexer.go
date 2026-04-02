package services

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	ignorepkg "github.com/sabhiram/go-gitignore"
)

// IndexerService indexes files under a repo, computes embeddings via OllamaService
// and upserts them to Qdrant if configured.
type IndexerService struct {
	repoRoot string
	// path to state file (configurable)
	statePath string
	ollama    *OllamaService
	qdrant    *QdrantService

	watcher *fsnotify.Watcher
	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
	stop    chan struct{}
	// fileHashes stores last seen hash to skip unchanged files (in-memory)
	fileHashes map[string]string
	// gitignore matcher
	ign *ignorepkg.GitIgnore
}

func NewIndexerService(repoRoot string, statePath string, ollama *OllamaService, qdrant *QdrantService) (*IndexerService, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	s := &IndexerService{
		repoRoot:   repoRoot,
		statePath:  statePath,
		ollama:     ollama,
		qdrant:     qdrant,
		watcher:    w,
		stop:       make(chan struct{}),
		fileHashes: make(map[string]string),
	}
	// try load .gitignore
	gi := filepath.Join(repoRoot, ".gitignore")
	if _, err := os.Stat(gi); err == nil {
		ign, err := ignorepkg.CompileIgnoreFile(gi)
		if err == nil {
			s.ign = ign
		}
	}
	// try load persisted indexer state regardless of .gitignore
	s.loadState()
	return s, nil
}

// IndexRepo walks the repoRoot and indexes supported files.
func (s *IndexerService) IndexRepo() error {
	files := []string{}
	err := filepath.Walk(s.repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// skip .git and node_modules
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(s.repoRoot, path)
		if s.ign != nil && s.ign.MatchesPath(rel) {
			return nil
		}
		if shouldIndexFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := s.indexFile(f); err != nil {
			// continue on error
		}
	}
	return nil
}

func shouldIndexFile(path string) bool {
	lower := strings.ToLower(path)
	exts := []string{".go", ".py", ".js", ".ts", ".java", ".md"}
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			return true
		}
	}
	return false
}

func (s *IndexerService) indexFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// compute hash and skip if unchanged
	hasher := sha1.New()
	if _, err := io.Copy(hasher, f); err == nil {
		sum := hex.EncodeToString(hasher.Sum(nil))
		s.mu.Lock()
		prev, ok := s.fileHashes[path]
		if ok && prev == sum {
			s.mu.Unlock()
			return nil
		}
		s.fileHashes[path] = sum
		s.mu.Unlock()
		// rewind
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	} else {
		// rewind just in case
		f.Seek(0, io.SeekStart)
	}
	// read file and chunk by lines
	rd := bufio.NewReader(f)
	var buf strings.Builder
	const chunkSize = 2000 // chars approx
	idx := 0
	for {
		part, err := rd.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}
		buf.WriteString(part)
		if buf.Len() >= chunkSize || err == io.EOF {
			text := buf.String()
			buf.Reset()
			id := s.idFor(path, idx)
			// compute embedding
			if s.ollama != nil {
				vec, embErr := s.ollama.GetEmbedding("default", text)
				if embErr == nil && s.qdrant != nil {
					payload := map[string]interface{}{"path": path, "chunk_index": idx, "code": text}
					_ = s.qdrant.UpsertPoint("repo_docs", id, vec, payload)
				}
			}
			// persist state asynchronously to avoid blocking
			go s.saveState()
			idx++
		}
		if err == io.EOF {
			break
		}
	}
	return nil
}

// state file path (configurable)
func (s *IndexerService) stateFilePath() string {
	if s.statePath != "" {
		return s.statePath
	}
	return filepath.Join(s.repoRoot, ".indexer_state.json")
}

func (s *IndexerService) saveState() {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.fileHashes, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return
	}
	tmp := s.stateFilePath() + ".tmp"
	_ = os.WriteFile(tmp, data, 0644)
	_ = os.Rename(tmp, s.stateFilePath())
}

func (s *IndexerService) loadState() {
	p := s.stateFilePath()
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	m := make(map[string]string)
	if err := json.Unmarshal(b, &m); err != nil {
		return
	}
	s.mu.Lock()
	s.fileHashes = m
	s.mu.Unlock()
}

func (s *IndexerService) idFor(path string, idx int) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s:%d", path, idx)))
	return hex.EncodeToString(h.Sum(nil))
}

// StartWatcher begins watching repoRoot for changes and indexes modifications.
func (s *IndexerService) StartWatcher() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// add root recursively
		filepath.Walk(s.repoRoot, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				s.watcher.Add(path)
			}
			return nil
		})
		for {
			select {
			case ev := <-s.watcher.Events:
				if ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
					if shouldIndexFile(ev.Name) {
						s.indexFile(ev.Name)
					}
				}
			case err := <-s.watcher.Errors:
				_ = err
			case <-s.stop:
				return
			}
		}
	}()
	return nil
}

func (s *IndexerService) StopWatcher() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()
	close(s.stop)
	s.watcher.Close()
	s.wg.Wait()
}

// ClearState removes persisted state file and clears in-memory hashes.
func (s *IndexerService) ClearState() error {
	s.mu.Lock()
	s.fileHashes = make(map[string]string)
	s.mu.Unlock()
	p := s.stateFilePath()
	if _, err := os.Stat(p); err == nil {
		return os.Remove(p)
	}
	return nil
}
