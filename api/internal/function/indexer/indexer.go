package service

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	architectservice "ollama-gateway/internal/function/architect"
	coreservice "ollama-gateway/internal/function/core"
	"ollama-gateway/internal/function/core/domain"
	eventservice "ollama-gateway/internal/function/events"
	"ollama-gateway/pkg/reposcope"

	"github.com/fsnotify/fsnotify"
	ignorepkg "github.com/sabhiram/go-gitignore"
)

// IndexerService indexes files under a repo, computes embeddings via OllamaService
// and upserts them to Qdrant if configured.
type IndexerService struct {
	repoRoot  string
	repoRoots []string
	// path to state file (configurable)
	statePath string
	ollama    *coreservice.OllamaService
	qdrant    *coreservice.QdrantService

	watcher *fsnotify.Watcher
	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
	stop    chan struct{}
	// fileHashes stores last seen hash to skip unchanged files (in-memory)
	fileHashes map[string]string
	// gitignore matcher
	ign           *ignorepkg.GitIgnore
	logger        *slog.Logger
	onChange      func()
	onFileIndexed func(path string)
	events        eventservice.Publisher
	analyzer      *architectservice.ASTAnalyzer
	reindexing    bool
	lastReindexAt time.Time
}

var _ domain.Indexer = (*IndexerService)(nil)

func NewIndexerService(repoRoots []string, statePath string, ollama *coreservice.OllamaService, qdrant *coreservice.QdrantService, logger *slog.Logger) (*IndexerService, error) {
	if logger == nil {
		logger = slog.Default()
	}
	roots := reposcope.CanonicalizeRoots(repoRoots)
	if len(roots) == 0 {
		roots = []string{"."}
	}
	primaryRoot := roots[0]
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	s := &IndexerService{
		repoRoot:   primaryRoot,
		repoRoots:  roots,
		statePath:  statePath,
		ollama:     ollama,
		qdrant:     qdrant,
		watcher:    w,
		stop:       make(chan struct{}),
		fileHashes: make(map[string]string),
		logger:     logger,
		analyzer:   architectservice.NewASTAnalyzer(roots),
	}
	// try load .gitignore
	gi := filepath.Join(primaryRoot, ".gitignore")
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

func (s *IndexerService) SetOnContentChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

func (s *IndexerService) SetOnFileIndexed(fn func(path string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFileIndexed = fn
}

func (s *IndexerService) SetEventPublisher(p eventservice.Publisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = p
}

func (s *IndexerService) notifyContentChange() {
	s.mu.Lock()
	fn := s.onChange
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (s *IndexerService) notifyFileIndexed(path string) {
	s.mu.Lock()
	fn := s.onFileIndexed
	pub := s.events
	s.mu.Unlock()
	if fn != nil {
		fn(path)
	}
	if pub != nil {
		repoRoot := reposcope.RepoForPath(path, s.repoRoots)
		_ = pub.Publish(context.Background(), eventservice.FileIndexed{
			Path:     path,
			RepoRoot: repoRoot,
			At:       time.Now().UTC(),
		})
	}
}

// IndexRepo walks the repoRoot and indexes supported files.
func (s *IndexerService) IndexRepo() error {
	s.mu.Lock()
	s.reindexing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.reindexing = false
		s.lastReindexAt = time.Now().UTC()
		s.mu.Unlock()
	}()

	for _, repoRoot := range s.repoRoots {
		files := []string{}
		err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				base := info.Name()
				if base == ".git" || base == "node_modules" || base == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(repoRoot, path)
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
	}
	return nil
}

func (s *IndexerService) Status() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	lastReindex := ""
	if !s.lastReindexAt.IsZero() {
		lastReindex = s.lastReindexAt.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"indexed_files":   len(s.fileHashes),
		"watcher_active":  s.running,
		"reindexing":      s.reindexing,
		"last_reindex_at": lastReindex,
	}
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

func (s *IndexerService) indexFile(path string) (retErr error) {
	changed := false
	defer func() {
		if changed && retErr == nil {
			s.notifyContentChange()
			s.notifyFileIndexed(path)
		}
	}()

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
		changed = true
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
	if strings.HasSuffix(strings.ToLower(path), ".go") && s.analyzer != nil {
		analysis, analyzeErr := s.analyzer.AnalyzeFile(path)
		if analyzeErr == nil {
			summary, marshalErr := json.Marshal(analysis)
			if marshalErr == nil {
				repoRoot := reposcope.RepoForPath(path, s.repoRoots)
				collection := reposcope.CollectionName(repoRoot)
				if s.ollama != nil {
					text := string(summary)
					vec, embErr := s.ollama.GetEmbedding("default", text)
					if embErr == nil && s.qdrant != nil {
						payload := map[string]interface{}{
							"path":        path,
							"chunk_index": 0,
							"code":        text,
							"repo_root":   repoRoot,
							"ast_summary": analysis,
						}
						_ = s.qdrant.UpsertPoint(collection, s.idFor(path, 0), vec, payload)
					}
				}
				go s.saveState(repoRoot)
				return nil
			}
		}
	}

	// fallback: index plain text chunks for non-Go files
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
			repoRoot := reposcope.RepoForPath(path, s.repoRoots)
			collection := reposcope.CollectionName(repoRoot)
			// compute embedding
			if s.ollama != nil {
				vec, embErr := s.ollama.GetEmbedding("default", text)
				if embErr == nil && s.qdrant != nil {
					payload := map[string]interface{}{"path": path, "chunk_index": idx, "code": text, "repo_root": repoRoot}
					_ = s.qdrant.UpsertPoint(collection, id, vec, payload)
				}
			}
			// persist state asynchronously to avoid blocking
			go s.saveState(repoRoot)
			idx++
		}
		if err == io.EOF {
			break
		}
	}
	return nil
}

// state file path (configurable)
func (s *IndexerService) stateFilePath(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = s.repoRoot
	}
	if s.statePath != "" {
		return reposcope.StatePathForRepo(s.statePath, repoRoot)
	}
	return reposcope.StatePathForRepo(filepath.Join(repoRoot, ".indexer_state.json"), repoRoot)
}

func (s *IndexerService) saveState(repoRoot string) {
	s.mu.Lock()
	state := make(map[string]string)
	for path, hash := range s.fileHashes {
		if path == repoRoot || strings.HasPrefix(path, repoRoot+string(os.PathSeparator)) {
			state[path] = hash
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return
	}
	tmp := s.stateFilePath(repoRoot) + ".tmp"
	_ = os.WriteFile(tmp, data, 0644)
	_ = os.Rename(tmp, s.stateFilePath(repoRoot))
}

func (s *IndexerService) loadState() {
	merged := make(map[string]string)
	for _, repoRoot := range s.repoRoots {
		p := s.stateFilePath(repoRoot)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		m := make(map[string]string)
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		for path, hash := range m {
			merged[path] = hash
		}
	}
	s.mu.Lock()
	s.fileHashes = merged
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
		for _, repoRoot := range s.repoRoots {
			filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
				if err == nil && info.IsDir() {
					s.watcher.Add(path)
				}
				return nil
			})
		}
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
	var firstErr error
	for _, repoRoot := range s.repoRoots {
		p := s.stateFilePath(repoRoot)
		if _, err := os.Stat(p); err == nil {
			if rmErr := os.Remove(p); rmErr != nil && firstErr == nil {
				firstErr = rmErr
			}
		}
	}
	return firstErr
}
