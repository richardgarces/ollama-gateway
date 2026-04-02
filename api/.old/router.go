package main

import (
	"strings"
)

// codeKeywords son palabras clave que indican que el prompt es sobre código.
var codeKeywords = []string{
	"func ", "package ", "golang", "go code", "implement", "refactor",
	"struct ", "interface ", "error handling", "goroutine", "channel",
	"import ", "return ", "if err != nil", "fmt.", "http.",
}

func selectModel(prompt string) string {
	lowerP := strings.ToLower(prompt)

	for _, kw := range codeKeywords {
		if strings.Contains(lowerP, kw) {
			return "deepseek-coder:6.7b"
		}
	}

	if len(prompt) > 300 {
		return "qwen2.5:7b"
	}

	return "gemma:2b"
}
