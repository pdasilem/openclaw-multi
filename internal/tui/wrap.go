package tui

import (
	"strings"
	"unicode/utf8"
)

const statusLineWidth = 100

func writeWrappedStatusLine(b *strings.Builder, prefix string, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		b.WriteString("  ")
		b.WriteString(prefix)
		b.WriteByte('\n')
		return
	}
	firstPrefix := "  " + prefix + " "
	nextPrefix := strings.Repeat(" ", utf8.RuneCountInString(firstPrefix))
	for i, line := range wrapText(message, statusLineWidth-utf8.RuneCountInString(firstPrefix)) {
		if i == 0 {
			b.WriteString(firstPrefix)
		} else {
			b.WriteString(nextPrefix)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func wrapText(s string, width int) []string {
	if width < 20 {
		width = 20
	}
	words := strings.Fields(strings.ReplaceAll(s, "\n", " "))
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width {
			line += " " + word
			continue
		}
		lines = append(lines, line)
		line = word
	}
	lines = append(lines, line)
	return lines
}
