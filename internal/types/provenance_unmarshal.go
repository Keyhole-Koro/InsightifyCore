package types

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// UnmarshalJSON makes ProvenanceRef accept either:
// 1) object: {"file":"path","lines":[10,20]}
// 2) string: "path:10-20" | "path:10" | "path"
func (p *ProvenanceRef) UnmarshalJSON(data []byte) error {
	// Try object first
	var obj struct {
		File  string `json:"file"`
		Lines any    `json:"lines"` // allow mixed types
	}
	if err := json.Unmarshal(data, &obj); err == nil && obj.File != "" {
		p.File = obj.File
		p.Lines = parseLinesAny(obj.Lines)
		return nil
	}

	// Fallback: string form
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		file, a, b := parseProvString(s)
		p.File = file
		p.Lines = [2]int{a, b}
		return nil
	}

	// Last resort: try generic map to coerce "lines" like ["10","20"]
	var m map[string]any
	if err := json.Unmarshal(data, &m); err == nil {
		if f, ok := m["file"].(string); ok {
			p.File = f
			p.Lines = parseLinesAny(m["lines"])
			return nil
		}
	}

	// If nothing matched, keep zero value but don't error harshly
	// (prevents pipeline from crashing on odd payloads)
	p.File = ""
	p.Lines = [2]int{0, 0}
	return nil
}

var provRe = regexp.MustCompile(`^(.+?)(?::(\d+)(?:-(\d+))?)?$`)

func parseProvString(s string) (file string, a, b int) {
	s = strings.TrimSpace(s)
	a, b = 0, 0
	m := provRe.FindStringSubmatch(s)
	if m == nil {
		return s, 0, 0
	}
	file = strings.TrimSpace(m[1])
	if m[2] != "" {
		a = atoiSafe(m[2])
	}
	if m[3] != "" {
		b = atoiSafe(m[3])
	} else if m[2] != "" {
		// "file:10" -> [10,10]
		b = a
	}
	return
}

func parseLinesAny(v any) [2]int {
	switch x := v.(type) {
	case nil:
		return [2]int{0, 0}
	case []any:
		if len(x) >= 2 {
			return [2]int{toInt(x[0]), toInt(x[1])}
		}
	case []int:
		if len(x) >= 2 {
			return [2]int{x[0], x[1]}
		}
	case []float64:
		if len(x) >= 2 {
			return [2]int{int(x[0]), int(x[1])}
		}
	case string:
		_, a, b := parseProvString(x)
		return [2]int{a, b}
	}
	return [2]int{0, 0}
}

func toInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		return atoiSafe(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return 0
	}
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
