package split

import (
	"regexp"
	"strings"
)

var blankLines = regexp.MustCompile(`(?m)\n[ \t]*\n(?:[ \t]*\n)*`)

// Paragraphs splits content at blank lines and removes surrounding whitespace.
func Paragraphs(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	parts := blankLines.Split(content, -1)
	paragraphs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paragraphs = append(paragraphs, part)
		}
	}

	return paragraphs
}
