package service

import (
	"bytes"
	"strings"
)

func joinToolDescriptions(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, line := range lines {
		b.WriteString(strings.TrimSpace(line))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
