package services

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ollama-gateway/internal/domain"
)

type PatchService struct {
	logger *slog.Logger
}

func NewPatchService(logger *slog.Logger) *PatchService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PatchService{logger: logger}
}

func (s *PatchService) ExtractCodeBlocks(response string) []domain.CodeBlock {
	re := regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\n(.*?)```")
	matches := re.FindAllStringSubmatch(response, -1)
	blocks := make([]domain.CodeBlock, 0, len(matches))
	for _, m := range matches {
		blocks = append(blocks, domain.CodeBlock{
			Language: strings.TrimSpace(m[1]),
			Code:     strings.TrimSuffix(m[2], "\n"),
		})
	}
	return blocks
}

func (s *PatchService) ExtractDiff(response string) []domain.UnifiedDiff {
	lines := splitLines(response)
	out := make([]domain.UnifiedDiff, 0)

	i := 0
	for i < len(lines) {
		line := lines[i]
		if !strings.HasPrefix(line, "--- ") {
			i++
			continue
		}
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
			i++
			continue
		}

		d := domain.UnifiedDiff{
			OldPath: normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "--- "))),
			NewPath: normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[i+1], "+++ "))),
			Hunks:   make([]domain.DiffHunk, 0),
		}
		i += 2

		for i < len(lines) {
			if strings.HasPrefix(lines[i], "--- ") {
				break
			}
			if !strings.HasPrefix(lines[i], "@@ ") {
				i++
				continue
			}

			h, next, err := parseHunk(lines, i)
			if err != nil {
				i++
				continue
			}
			d.Hunks = append(d.Hunks, h)
			i = next
		}

		if len(d.Hunks) > 0 {
			out = append(out, d)
		}
	}

	return out
}

func (s *PatchService) ApplyPatch(repoRoot string, diff domain.UnifiedDiff) error {
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("repo root invalido: %w", err)
	}

	if diff.NewPath == "/dev/null" {
		target, err := resolveSafePath(absRepo, diff.OldPath)
		if err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	target, err := resolveSafePath(absRepo, diff.NewPath)
	if err != nil {
		return err
	}

	original := ""
	if diff.OldPath != "/dev/null" {
		b, err := os.ReadFile(target)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			original = string(b)
		}
	}

	lines := splitFileLines(original)
	hadTrailingNewline := strings.HasSuffix(original, "\n")
	delta := 0

	for _, h := range diff.Hunks {
		start := h.OldStart - 1 + delta
		if start < 0 {
			start = 0
		}
		if start > len(lines) {
			return fmt.Errorf("hunk fuera de rango para %s", target)
		}

		cursor := start
		replacement := make([]string, 0, len(h.Lines))
		for _, raw := range h.Lines {
			if raw == "" {
				raw = " "
			}
			prefix := raw[0]
			text := ""
			if len(raw) > 1 {
				text = raw[1:]
			}
			switch prefix {
			case ' ':
				if cursor >= len(lines) || lines[cursor] != text {
					return fmt.Errorf("context mismatch al aplicar patch en %s", target)
				}
				replacement = append(replacement, text)
				cursor++
			case '-':
				if cursor >= len(lines) || lines[cursor] != text {
					return fmt.Errorf("delete mismatch al aplicar patch en %s", target)
				}
				cursor++
			case '+':
				replacement = append(replacement, text)
			case '\\':
				// No newline at end of file marker
			default:
				return fmt.Errorf("linea de hunk invalida: %q", raw)
			}
		}

		lines = spliceLines(lines, start, cursor, replacement)
		delta += len(replacement) - (cursor - start)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}

	content := strings.Join(lines, "\n")
	if hadTrailingNewline || (len(lines) > 0 && diff.NewPath != "/dev/null") {
		content += "\n"
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return err
	}
	return nil
}

func parseHunk(lines []string, start int) (domain.DiffHunk, int, error) {
	header := lines[start]
	re := regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
	m := re.FindStringSubmatch(header)
	if len(m) == 0 {
		return domain.DiffHunk{}, start + 1, fmt.Errorf("hunk header invalido")
	}

	h := domain.DiffHunk{
		OldStart: mustAtoi(m[1]),
		OldLines: parseOptionalInt(m[2], 1),
		NewStart: mustAtoi(m[3]),
		NewLines: parseOptionalInt(m[4], 1),
		Lines:    make([]string, 0),
	}

	i := start + 1
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "--- ") {
			break
		}
		if strings.HasPrefix(line, "diff --git ") {
			break
		}
		h.Lines = append(h.Lines, line)
		i++
	}
	return h, i, nil
}

func normalizeDiffPath(raw string) string {
	if raw == "" {
		return raw
	}
	fields := strings.Fields(raw)
	path := fields[0]
	path = strings.Trim(path, "\"")
	if path == "/dev/null" {
		return path
	}
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func resolveSafePath(absRepoRoot, candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || candidate == "/dev/null" {
		return "", fmt.Errorf("path diff invalido")
	}

	var joined string
	if filepath.IsAbs(candidate) {
		joined = candidate
	} else {
		joined = filepath.Join(absRepoRoot, candidate)
	}

	absTarget, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	if absTarget != absRepoRoot && !strings.HasPrefix(absTarget, absRepoRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("acceso denegado: path fuera de repo root")
	}
	return absTarget, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	scanner := bufio.NewScanner(strings.NewReader(s))
	out := make([]string, 0)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	return out
}

func splitFileLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return []string{}
	}
	return strings.Split(normalized, "\n")
}

func spliceLines(lines []string, start, end int, replacement []string) []string {
	left := append([]string{}, lines[:start]...)
	left = append(left, replacement...)
	left = append(left, lines[end:]...)
	return left
}

func mustAtoi(v string) int {
	return parseOptionalInt(v, 0)
}

func parseOptionalInt(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	n := 0
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return fallback
		}
		n = n*10 + int(v[i]-'0')
	}
	if n == 0 {
		return fallback
	}
	return n
}
