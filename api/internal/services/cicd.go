package services

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"ollama-gateway/internal/domain"
)

type CICDService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewCICDService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *CICDService {
	if logger == nil {
		logger = slog.Default()
	}
	return &CICDService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *CICDService) GeneratePipeline(platform, repoRoot string) (string, error) {
	platform = normalizePipelinePlatform(platform)
	if platform == "" {
		return "", fmt.Errorf("platform inválida (permitidas: github-actions, gitlab-ci, jenkins)")
	}

	root, err := s.resolveRepoRoot(repoRoot)
	if err != nil {
		return "", err
	}

	contextText := s.collectBuildContext(root)
	prompt := strings.Join([]string{
		fmt.Sprintf("Analyze this project structure and generate a CI/CD pipeline for %s. Include: lint, test, build, docker build, deploy stages. Use caching for dependencies.", platform),
		"Return ONLY the pipeline file content with no explanations and no markdown fences.",
		"Prefer practical defaults for a Go backend and include conditional steps if files are missing.",
		"Project build context:",
		contextText,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripMarkdownFence(out)), nil
}

func (s *CICDService) OptimizePipeline(existing string, platform string) (string, error) {
	platform = normalizePipelinePlatform(platform)
	if platform == "" {
		return "", fmt.Errorf("platform inválida (permitidas: github-actions, gitlab-ci, jenkins)")
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return "", fmt.Errorf("pipeline requerido")
	}

	contextText := s.collectBuildContext(s.repoRoot)
	prompt := strings.Join([]string{
		fmt.Sprintf("Optimize this %s CI/CD pipeline.", platform),
		"Improve reliability, speed, cache strategy, and security hardening.",
		"Keep lint, test, build, docker build, and deploy stages.",
		"Return ONLY the optimized pipeline content with no explanations and no markdown fences.",
		"Current pipeline:",
		existing,
		"Project build context:",
		contextText,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripMarkdownFence(out)), nil
}

func (s *CICDService) ApplyPipeline(platform, repoRoot, content string) (string, string, error) {
	platform = normalizePipelinePlatform(platform)
	if platform == "" {
		return "", "", fmt.Errorf("platform inválida (permitidas: github-actions, gitlab-ci, jenkins)")
	}
	if strings.TrimSpace(content) == "" {
		return "", "", fmt.Errorf("pipeline vacío")
	}
	root, err := s.resolveRepoRoot(repoRoot)
	if err != nil {
		return "", "", err
	}

	targetRel := pipelineTargetPath(platform)
	targetAbs, err := s.resolvePathWithinRoot(root, targetRel)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return "", "", err
	}

	backup := ""
	if existing, readErr := os.ReadFile(targetAbs); readErr == nil {
		backup = targetAbs + ".bak"
		if err := os.WriteFile(backup, existing, 0o644); err != nil {
			return "", "", err
		}
	}

	if err := os.WriteFile(targetAbs, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	return targetAbs, backup, nil
}

func normalizePipelinePlatform(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "github-actions", "github", "gha":
		return "github-actions"
	case "gitlab-ci", "gitlab", "gitlabci":
		return "gitlab-ci"
	case "jenkins", "jenkinsfile":
		return "jenkins"
	default:
		return ""
	}
}

func pipelineTargetPath(platform string) string {
	switch normalizePipelinePlatform(platform) {
	case "github-actions":
		return filepath.ToSlash(filepath.Join(".github", "workflows", "ci.yml"))
	case "gitlab-ci":
		return ".gitlab-ci.yml"
	case "jenkins":
		return "Jenkinsfile"
	default:
		return ""
	}
}

func (s *CICDService) resolveRepoRoot(input string) (string, error) {
	base, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return "", fmt.Errorf("REPO_ROOT inválido")
	}

	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return base, nil
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("repoRoot inválido")
	}

	if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("repoRoot fuera de REPO_ROOT")
	}
	return abs, nil
}

func (s *CICDService) resolvePathWithinRoot(root string, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("ruta de pipeline inválida")
	}
	joined := filepath.Join(root, rel)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("ruta de pipeline inválida")
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("ruta de pipeline fuera de REPO_ROOT")
	}
	return abs, nil
}

func (s *CICDService) collectBuildContext(root string) string {
	resolvedRoot, err := s.resolveRepoRoot(root)
	if err != nil {
		resolvedRoot = s.repoRoot
	}
	files := []string{"Makefile", "Dockerfile", "go.mod", "package.json", "docker-compose.yml", "docker-compose.yaml"}

	var b strings.Builder
	for _, f := range files {
		abs, pathErr := s.resolvePathWithinRoot(resolvedRoot, f)
		if pathErr != nil {
			continue
		}
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(resolvedRoot, abs)
		if relErr != nil {
			rel = f
		}
		b.WriteString("FILE: ")
		b.WriteString(filepath.ToSlash(rel))
		b.WriteString("\n")
		content := string(data)
		if len(content) > 5000 {
			content = content[:5000]
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	if b.Len() == 0 {
		return "No build files found (Makefile, Dockerfile, go.mod, package.json)."
	}
	return strings.TrimSpace(b.String())
}
