package domain

type RouteDefinition struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	Description   string `json:"description"`
	ExampleBody   string `json:"example_body"`
	Protected     bool   `json:"protected"`
	LocalhostOnly bool   `json:"localhost_only"`
	SSE           bool   `json:"sse"`
}
