package domain

type CodeBlock struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type UnifiedDiff struct {
	OldPath string     `json:"old_path"`
	NewPath string     `json:"new_path"`
	Hunks   []DiffHunk `json:"hunks"`
}

type DiffHunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}
