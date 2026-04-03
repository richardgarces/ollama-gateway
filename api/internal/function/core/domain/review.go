package domain

type ReviewComment struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Comment  string `json:"comment"`
}

type ReviewResult struct {
	Comments []ReviewComment `json:"comments"`
	Summary  string          `json:"summary"`
}
