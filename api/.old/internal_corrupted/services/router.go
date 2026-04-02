package services

import (
	"strings"
)

type RouterService struct {
	codeKeywords []string
}

func NewRouterService() *RouterService {
	return &RouterService{
		codeKeywords: []string{
			"func ", "package ", "golang", "go code", "implement", "refactor",
			"struct ", "interface ", "error handling", "goroutine", "channel",
			"import ", "return ", "if err != nil", "fmt.", "http.",
		},
	}
}

func (s *RouterService) SelectModel(prompt string) string {
	lowerP := strings.ToLower(prompt)

	for _, kw := range s.codeKeywords {
		if strings.Contains(lowerP, kw) {
			return "deepseek-coder:6.7b"
		}
	}

	if len(prompt) > 300 {
		return "qwen2.5:7b"
	}

	return "gemma:2b"
}


import (
	"strings"
)

type RouterService struct {
	codeKeywords []string
}

func NewRouterService() *RouterService {
	return &RouterService{
		codeKeywords: []string{
			"func ", "package ", "golang", "go code", "implement", "refactor",
			"struct ", "interface ", "error handling", "goroutine", "channel",
			"import ", "return ", "if err != nil", "fmt.", "http.",
		},
	}
}

func (r *RouterService) SelectModel(prompt string) string {
	lowerP := strings.ToLower(prompt)

	for _, kw := range r.codeKeywords {
		if strings.Contains(lowerP, kw) {
			return "deepseek-coder:6.7b"
		}
	}

	if len(prompt) > 300 {
		return "qwen2.5:7b"
	}

	return "gemma:2b"
}
