package prompts

import "strings"

func Get(lang, key string) string {
	requested := strings.ToLower(strings.TrimSpace(lang))
	if requested == "" {
		requested = "en"
	}

	var table map[string]string
	switch requested {
	case "es":
		table = es
	case "pt":
		table = pt
	default:
		table = en
	}

	if v, ok := table[key]; ok && strings.TrimSpace(v) != "" {
		return v
	}
	if v, ok := en[key]; ok {
		return v
	}
	return ""
}
