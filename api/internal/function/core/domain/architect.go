package domain

type Recommendation struct {
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
}

type ArchReport struct {
	Score1To10      int                 `json:"score_1_10"`
	Strengths       []string            `json:"strengths"`
	Weaknesses      []string            `json:"weaknesses"`
	Recommendations []Recommendation    `json:"recommendations"`
	DependencyGraph map[string][]string `json:"dependency_graph"`
}

type PatternSuggestion struct {
	Pattern    string `json:"pattern"`
	Severity   string `json:"severity"`
	Reason     string `json:"reason"`
	Suggestion string `json:"suggestion"`
	DiffHint   string `json:"diff_hint,omitempty"`
}

type PatternReport struct {
	Path        string              `json:"path"`
	RiskScore   int                 `json:"risk_score"`
	RiskLevel   string              `json:"risk_level"`
	Suggestions []PatternSuggestion `json:"suggestions"`
}
