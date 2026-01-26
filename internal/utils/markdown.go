package utils

import (
	"regexp"
	"strings"
)

var (
	// reImageMD matches markdown images: ![alt](url)
	reImageMD = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	// reImageHTML matches HTML image tags: <img ...>
	reImageHTML = regexp.MustCompile(`(?is)<img[^>]*>`)
	// reComment matches HTML comments: <!-- ... -->
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// reExcessiveNewlines matches 3 or more newlines to compress them
	reExcessiveNewlines = regexp.MustCompile(`\n{3,}`)
)

// Clean removes content that is generally not useful for LLM context,
// such as images, HTML comments, and excessive whitespace.
func MarkDownClean(text string) string {
	// Remove images
	text = reImageMD.ReplaceAllString(text, "")
	text = reImageHTML.ReplaceAllString(text, "")

	// Remove comments
	text = reComment.ReplaceAllString(text, "")

	// Normalize newlines (max 2 consecutive newlines for paragraph separation)
	text = reExcessiveNewlines.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}
