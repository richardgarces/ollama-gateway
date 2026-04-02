package domain

type DebugAnalysis struct {
	RootCause      string   `json:"root_cause"`
	Explanation    string   `json:"explanation"`
	SuggestedFixes []string `json:"suggested_fixes"`
	RelatedFiles   []string `json:"related_files"`
}
