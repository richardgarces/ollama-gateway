package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WriteOptions struct {
	RepoRoot string
}

// WriteChangelog escribe el changelog en CHANGELOG.md si apply=true
func WriteChangelog(notes *ReleaseNotes, opts WriteOptions) error {
	rootAbs, err := filepath.Abs(strings.TrimSpace(opts.RepoRoot))
	if err != nil {
		return fmt.Errorf("REPO_ROOT inválido: %w", err)
	}

	changelogPath := filepath.Join(rootAbs, "CHANGELOG.md")
	absPath, err := filepath.Abs(changelogPath)
	if err != nil {
		return fmt.Errorf("no se pudo obtener ruta absoluta: %w", err)
	}
	if absPath != rootAbs && !strings.HasPrefix(absPath, rootAbs+string(os.PathSeparator)) {
		return fmt.Errorf("ruta fuera del repo root: %s", absPath)
	}
	content := RenderMarkdown(notes)

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(absPath); statErr == nil {
		mode = info.Mode().Perm()
	}

	return os.WriteFile(absPath, []byte(content), mode)
}

func RenderMarkdown(notes *ReleaseNotes) string {
	if notes == nil {
		return "# Release Notes\n"
	}

	content := "# Release Notes\n\n"
	if len(notes.Features) > 0 {
		content += "## Features\n"
		for _, f := range notes.Features {
			content += "- " + strings.TrimSpace(f) + "\n"
		}
	}
	if len(notes.Fixes) > 0 {
		content += "\n## Fixes\n"
		for _, f := range notes.Fixes {
			content += "- " + strings.TrimSpace(f) + "\n"
		}
	}
	if len(notes.BreakingChanges) > 0 {
		content += "\n## Breaking Changes\n"
		for _, b := range notes.BreakingChanges {
			content += "- " + strings.TrimSpace(b) + "\n"
		}
	}
	if len(notes.Security) > 0 {
		content += "\n## Security\n"
		for _, s := range notes.Security {
			content += "- " + strings.TrimSpace(s) + "\n"
		}
	}

	if len(notes.Features) == 0 && len(notes.Fixes) == 0 && len(notes.BreakingChanges) == 0 && len(notes.Security) == 0 {
		content += "_No se detectaron cambios convencionales entre las referencias indicadas._\n"
	}

	return content
}
