package service

import (
	"bufio"
	"errors"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ollama-gateway/pkg/reposcope"
)

const (
	defaultTopK          = 8
	defaultMaxDepth      = 2
	defaultGraphCacheTTL = 45 * time.Second
)

type ResolveInput struct {
	FilePath string `json:"file_path"`
	Prompt   string `json:"prompt"`
	TopK     int    `json:"top_k"`
	MaxDepth int    `json:"max_depth"`
}

type ResolvedFile struct {
	Path     string  `json:"path"`
	RepoPath string  `json:"repo_path"`
	Depth    int     `json:"depth"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
}

type fileMeta struct {
	RepoRoot string
	RepoPath string
}

type graphSnapshot struct {
	adjacency map[string]map[string]struct{}
	meta      map[string]fileMeta
	allFiles  []string
	builtAt   time.Time
}

type Service struct {
	logger    *slog.Logger
	repoRoots []string
	cacheTTL  time.Duration

	mu    sync.RWMutex
	graph graphSnapshot
}

func NewService(repoRoots []string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	roots := reposcope.CanonicalizeRoots(repoRoots)
	if len(roots) == 0 {
		roots = []string{"."}
	}
	return &Service{
		logger:    logger,
		repoRoots: roots,
		cacheTTL:  defaultGraphCacheTTL,
	}
}

func (s *Service) ResolveContextFiles(input ResolveInput) ([]ResolvedFile, error) {
	graph, err := s.getGraph()
	if err != nil {
		return nil, err
	}
	if len(graph.allFiles) == 0 {
		return []ResolvedFile{}, nil
	}

	topK := input.TopK
	if topK <= 0 {
		topK = defaultTopK
	}
	maxDepth := input.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}

	prompt := strings.TrimSpace(input.Prompt)
	seedScores := make(map[string]float64)
	bestDepth := make(map[string]int)

	if path, ok := s.normalizeInputFile(input.FilePath, graph.meta); ok {
		seedScores[path] += 4.0
		bestDepth[path] = 0
	}
	for _, guessed := range guessPathSeeds(prompt, graph.allFiles) {
		seedScores[guessed] += 2.4
		if _, ok := bestDepth[guessed]; !ok {
			bestDepth[guessed] = 0
		}
	}

	tokens := keywordBoost(prompt)
	if len(seedScores) == 0 && len(tokens) == 0 {
		return nil, errors.New("se requiere file_path o prompt para resolver contexto")
	}

	scores := make(map[string]float64)
	for path, score := range seedScores {
		scores[path] += score
	}

	if len(seedScores) > 0 {
		type qItem struct {
			path  string
			depth int
		}
		queue := make([]qItem, 0, len(seedScores))
		visited := make(map[string]int)
		for seed := range seedScores {
			queue = append(queue, qItem{path: seed, depth: 0})
			visited[seed] = 0
		}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if current.depth >= maxDepth {
				continue
			}
			nextDepth := current.depth + 1
			for neighbor := range graph.adjacency[current.path] {
				scores[neighbor] += 1.8 / float64(nextDepth+1)
				if d, ok := bestDepth[neighbor]; !ok || nextDepth < d {
					bestDepth[neighbor] = nextDepth
				}
				if prev, ok := visited[neighbor]; ok && prev <= nextDepth {
					continue
				}
				visited[neighbor] = nextDepth
				queue = append(queue, qItem{path: neighbor, depth: nextDepth})
			}
		}
	}

	if len(tokens) > 0 {
		for _, path := range graph.allFiles {
			boost := scorePathByTokens(path, tokens)
			if boost <= 0 {
				continue
			}
			scores[path] += boost
			if _, ok := bestDepth[path]; !ok {
				bestDepth[path] = maxDepth + 1
			}
		}
	}

	if len(scores) == 0 {
		return []ResolvedFile{}, nil
	}

	results := make([]ResolvedFile, 0, len(scores))
	for path, score := range scores {
		meta := graph.meta[path]
		depth := bestDepth[path]
		reason := "prompt"
		if depth == 0 {
			reason = "seed"
		} else if depth <= maxDepth {
			reason = "dependency-neighbor"
		}
		results = append(results, ResolvedFile{
			Path:     path,
			RepoPath: meta.RepoPath,
			Depth:    depth,
			Score:    score,
			Reason:   reason,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].Depth == results[j].Depth {
				return results[i].Path < results[j].Path
			}
			return results[i].Depth < results[j].Depth
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (s *Service) getGraph() (graphSnapshot, error) {
	s.mu.RLock()
	cached := s.graph
	cacheValid := len(cached.allFiles) > 0 && time.Since(cached.builtAt) < s.cacheTTL
	s.mu.RUnlock()
	if cacheValid {
		return cached, nil
	}

	built, err := s.buildGraph()
	if err != nil {
		if len(cached.allFiles) > 0 {
			s.logger.Warn("context graph build falló; se usa snapshot previo", slog.String("error", err.Error()))
			return cached, nil
		}
		return graphSnapshot{}, err
	}

	s.mu.Lock()
	s.graph = built
	s.mu.Unlock()
	return built, nil
}

func (s *Service) buildGraph() (graphSnapshot, error) {
	type parsedFile struct {
		imports  []string
		repoRoot string
		repoPath string
	}

	files := make(map[string]parsedFile)
	importIndex := make(map[string][]string)
	allFiles := make([]string, 0, 512)

	for _, root := range s.repoRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		modulePath := readModulePath(absRoot)
		_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			lower := strings.ToLower(path)
			if !strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			imports := parseImportsWithTreeSitter(path)
			if len(imports) == 0 {
				parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
				if err != nil {
					return nil
				}
				for _, imp := range parsed.Imports {
					v := strings.Trim(imp.Path.Value, "\"")
					if v != "" {
						imports = append(imports, v)
					}
				}
			}

			relPath, err := filepath.Rel(absRoot, path)
			if err != nil {
				return nil
			}
			relPath = filepath.ToSlash(relPath)
			relDir := filepath.ToSlash(filepath.Dir(relPath))
			if relDir == "." {
				relDir = ""
			}

			files[path] = parsedFile{imports: imports, repoRoot: absRoot, repoPath: relPath}
			allFiles = append(allFiles, path)

			registerImportKey(importIndex, relDir, path)
			base := filepath.Base(relDir)
			if base != "." && base != string(filepath.Separator) {
				registerImportKey(importIndex, base, path)
			}
			if modulePath != "" {
				if relDir == "" {
					registerImportKey(importIndex, modulePath, path)
				} else {
					registerImportKey(importIndex, modulePath+"/"+relDir, path)
				}
			}
			return nil
		})
	}

	sort.Strings(allFiles)
	meta := make(map[string]fileMeta, len(files))
	adj := make(map[string]map[string]struct{}, len(files))
	for path, info := range files {
		meta[path] = fileMeta{RepoRoot: info.repoRoot, RepoPath: info.repoPath}
		adj[path] = make(map[string]struct{})
	}
	for src, info := range files {
		for _, imp := range info.imports {
			targets := resolveImportTargets(importIndex, imp)
			for _, dst := range targets {
				if src == dst {
					continue
				}
				adj[src][dst] = struct{}{}
				adj[dst][src] = struct{}{}
			}
		}
	}

	return graphSnapshot{adjacency: adj, meta: meta, allFiles: allFiles, builtAt: time.Now()}, nil
}

func registerImportKey(index map[string][]string, key, path string) {
	key = strings.TrimSpace(strings.Trim(key, "/"))
	if key == "" {
		return
	}
	index[key] = append(index[key], path)
}

func resolveImportTargets(index map[string][]string, imp string) []string {
	imp = strings.TrimSpace(strings.Trim(imp, "/"))
	if imp == "" {
		return nil
	}
	if direct := index[imp]; len(direct) > 0 {
		return dedupe(direct)
	}
	if tail := filepath.Base(imp); tail != "" {
		if byTail := index[tail]; len(byTail) > 0 {
			return dedupe(byTail)
		}
	}
	return nil
}

func dedupe(items []string) []string {
	if len(items) < 2 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (s *Service) normalizeInputFile(value string, meta map[string]fileMeta) (string, bool) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", false
	}
	if filepath.IsAbs(candidate) {
		if abs, ok := s.ensureInsideRoots(candidate); ok {
			if _, exists := meta[abs]; exists {
				return abs, true
			}
		}
	} else {
		for _, root := range s.repoRoots {
			joined := filepath.Join(root, candidate)
			if abs, ok := s.ensureInsideRoots(joined); ok {
				if _, exists := meta[abs]; exists {
					return abs, true
				}
			}
		}
	}

	suffix := filepath.ToSlash(candidate)
	for file := range meta {
		if strings.HasSuffix(filepath.ToSlash(file), suffix) {
			return file, true
		}
	}
	return "", false
}

func (s *Service) ensureInsideRoots(candidate string) (string, bool) {
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	for _, root := range s.repoRoots {
		if abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			return abs, true
		}
	}
	return "", false
}

var pathTokenRegexp = regexp.MustCompile(`[A-Za-z0-9_./\\-]+\.go`)

func guessPathSeeds(prompt string, allFiles []string) []string {
	tokens := pathTokenRegexp.FindAllString(prompt, -1)
	if len(tokens) == 0 {
		return nil
	}
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		norm := filepath.ToSlash(strings.Trim(token, "\"'()[]{}<>"))
		if norm == "" {
			continue
		}
		for _, file := range allFiles {
			normFile := filepath.ToSlash(file)
			if strings.HasSuffix(normFile, norm) || strings.Contains(normFile, norm) {
				out = append(out, file)
			}
		}
	}
	return dedupe(out)
}

var keywordRegexp = regexp.MustCompile(`[A-Za-z0-9_]{3,}`)

func keywordBoost(prompt string) map[string]float64 {
	matches := keywordRegexp.FindAllString(strings.ToLower(prompt), -1)
	boost := make(map[string]float64, len(matches))
	for _, match := range matches {
		boost[match] += 1
	}
	return boost
}

func scorePathByTokens(path string, tokens map[string]float64) float64 {
	if len(tokens) == 0 {
		return 0
	}
	full := strings.ToLower(filepath.ToSlash(path))
	total := 0.0
	for token, weight := range tokens {
		if strings.Contains(full, token) {
			total += 0.25 * weight
		}
	}
	if total > 1.8 {
		total = 1.8
	}
	return total
}

func readModulePath(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "module "))
		}
	}
	return ""
}

// parseImportsWithTreeSitter uses the `tree-sitter` CLI when available.
// Expected command output includes import path literals; fallback is handled by caller.
func parseImportsWithTreeSitter(filePath string) []string {
	out, err := exec.Command("tree-sitter", "parse", filePath).Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	imports := make([]string, 0, 8)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "import") && !strings.Contains(line, "interpreted_string_literal") {
			continue
		}
		for _, lit := range extractQuotedLiterals(line) {
			if lit != "" {
				imports = append(imports, lit)
			}
		}
	}
	return dedupe(imports)
}

var quotedLiteralRegexp = regexp.MustCompile(`"([^"]+)"`)

func extractQuotedLiterals(line string) []string {
	matches := quotedLiteralRegexp.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}
