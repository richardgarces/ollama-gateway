package reposcope

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CanonicalizeRoots(roots []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

func HashRepo(repoRoot string) string {
	h := sha1.New()
	h.Write([]byte(strings.TrimSpace(repoRoot)))
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}

func CollectionName(repoRoot string) string {
	h := HashRepo(repoRoot)
	if h == "" {
		return "repo_docs"
	}
	return "repo_" + h
}

func StatePathForRepo(baseStatePath, repoRoot string) string {
	h := HashRepo(repoRoot)
	if h == "" {
		h = "default"
	}

	name := ".indexer_state"
	ext := ".json"
	if strings.TrimSpace(baseStatePath) != "" {
		base := filepath.Base(baseStatePath)
		if base != "" && base != "." {
			name = strings.TrimSuffix(base, filepath.Ext(base))
			ext = filepath.Ext(base)
		}
	}
	if ext == "" {
		ext = ".json"
	}
	return filepath.Join(repoRoot, fmt.Sprintf("%s_%s%s", name, h, ext))
}

func MatchRepoFilter(filter string, roots []string) (string, bool) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return "", false
	}

	absFilter := ""
	if filepath.IsAbs(filter) {
		if abs, err := filepath.Abs(filter); err == nil {
			absFilter = abs
		}
	}

	for _, root := range roots {
		if absFilter != "" && root == absFilter {
			return root, true
		}
		if strings.EqualFold(filepath.Base(root), filter) {
			return root, true
		}
		if root == filter {
			return root, true
		}
	}
	return "", false
}

func RepoForPath(path string, roots []string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	longest := ""
	for _, root := range roots {
		if absPath == root || strings.HasPrefix(absPath, root+string(os.PathSeparator)) {
			if len(root) > len(longest) {
				longest = root
			}
		}
	}
	return longest
}
