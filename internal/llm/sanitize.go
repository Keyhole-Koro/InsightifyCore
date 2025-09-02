// internal/llm/sanitize.go（新規）
package llm

import (
	"encoding/base64"
	"regexp"
)

var (
	reDataURL = regexp.MustCompile(`(?is)\bdata:(image|video|audio)/[a-z0-9+.-]+;base64,[a-z0-9+/=\r\n]+`)
	reImgTag  = regexp.MustCompile(`(?is)<img[^>]*src=["']data:(image)/[^"']+["'][^>]*>`)
)

// RedactMedia walks any JSON-like value and replaces media payloads with a marker.
func RedactMedia(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = RedactMedia(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = RedactMedia(vv)
		}
		return out
	case string:
		s := x
		// data URL (image/video/audio)
		if reDataURL.MatchString(s) || reImgTag.MatchString(s) || looksLikeBase64Image(s) {
			return "[REDACTED media]"
		}
		return s
	default:
		return v
	}
}

func looksLikeBase64Image(s string) bool {
	if len(s) < 512 { return false }
	// quick base64 check
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}
