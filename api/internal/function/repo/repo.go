package service

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	coreservice "ollama-gateway/internal/function/core"
)

type RepoService struct {
	ollamaService *coreservice.OllamaService
	repoRoot      string
	logger        *slog.Logger
}

func NewRepoService(ollamaService *coreservice.OllamaService, repoRoot string, logger *slog.Logger) *RepoService {
	if logger == nil {
		logger = slog.Default()
	}
	return &RepoService{
		ollamaService: ollamaService,
		repoRoot:      repoRoot,
		logger:        logger,
	}
}

func (s *RepoService) Refactor(absPath string) (string, error) {
	allowedRoot, _ := filepath.Abs(s.repoRoot)
	if !strings.HasPrefix(absPath, allowedRoot+string(os.PathSeparator)) && absPath != allowedRoot {
		return "", filepath.ErrBadPattern
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	prompt := "Eres un senior Go developer. Mejora este código:\n" + string(data)
	return s.ollamaService.Generate("deepseek-coder:6.7b", prompt)
}

func (s *RepoService) AnalyzeRepo() (string, error) {
	var files []string
	filepath.Walk(s.repoRoot, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})

	results := make([]string, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, f := range files {
		if i > 5 {
			break
		}
		wg.Add(1)
		go func(file string) {
			defer wg.Done()
			data, _ := os.ReadFile(file)
			prompt := "Revisa este file y dime algo sobre su arquitectura y mejoras:\n" + string(data)
			resp, _ := s.ollamaService.Generate("qwen2.5:7b", prompt)
			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
		}(f)
	}

	wg.Wait()

	prompt := "Resume estos análisis:\n" + strings.Join(results, "\n")
	return s.ollamaService.Generate("qwen2.5:7b", prompt)
}
