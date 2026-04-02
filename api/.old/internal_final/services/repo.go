package services

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type RepoService struct {
	ollamaService *OllamaService
	repoRoot      string
}

func NewRepoService(ollamaService *OllamaService, repoRoot string) *RepoService {
	return &RepoService{
		ollamaService: ollamaService,
		repoRoot:      repoRoot,
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
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type RepoService struct {
	ollamaService *OllamaService
	repoRoot      string
}

func NewRepoService(ollamaService *OllamaService, repoRoot string) *RepoService {
	return &RepoService{
		ollamaService: ollamaService,
		repoRoot:      repoRoot,
	}
}

func (r *RepoService) ValidatePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	allowedRoot, _ := filepath.Abs(r.repoRoot)
	if !strings.HasPrefix(absPath, allowedRoot+string(os.PathSeparator)) && absPath != allowedRoot {
		return "", filepath.ErrBadPattern
	}
	return absPath, nil
}

func (r *RepoService) Refactor(path string) (string, error) {
	absPath, err := r.ValidatePath(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	prompt := "Eres un senior Go developer. Mejora este código:\n" + string(data)
	return r.ollamaService.Generate("deepseek-coder:6.7b", prompt)
}

func (r *RepoService) AnalyzeRepo() (string, error) {
	var files []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})

	if len(files) > 10 {
		files = files[:10]
	}

	var wg sync.WaitGroup
	results := make([]string, len(files))

	for i, f := range files {
		wg.Add(1)
		go func(idx int, file string) {
			defer wg.Done()
			data, _ := os.ReadFile(file)
			prompt := "Revisa este file y dime algo sobre su arquitectura y mejoras:\n" + string(data)
			resp, _ := r.ollamaService.Generate("qwen2.5:7b", prompt)
			results[idx] = resp
		}(i, f)
	}
	wg.Wait()

	finalResults := []string{}
	for _, r := range results {
		if r != "" {
			finalResults = append(finalResults, r)
		}
	}

	prompt := "Resume estos análisis:\n" + strings.Join(finalResults, "\n")
	return r.ollamaService.Generate("qwen2.5:7b", prompt)
}
