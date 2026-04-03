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
