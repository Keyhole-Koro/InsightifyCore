package llmclient

import "strings"

// CountTokens provides a rough token count for text, useful for weighting scheduler tasks.
// It counts whitespace-delimited words and falls back to a character-based heuristic.
func CountTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	words := strings.Fields(text)
	if len(words) > 0 {
		return len(words)
	}
	n := len(text) / 4
	if n == 0 {
		n = 1
	}
	return n
}
