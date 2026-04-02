package domain

import "time"

type SecurityFinding struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	Fix         string `json:"fix"`
	Path        string `json:"path,omitempty"`
}

type SecurityReport struct {
	ScannedFiles    int               `json:"scanned_files"`
	TotalFindings   int               `json:"total_findings"`
	HighOrCritical  int               `json:"high_or_critical"`
	FindingsByLevel map[string]int    `json:"findings_by_level"`
	Findings        []SecurityFinding `json:"findings"`
	GeneratedAt     time.Time         `json:"generated_at"`
	FileErrors      map[string]string `json:"file_errors,omitempty"`
}
