package jsonutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// Unmarshal is a compatibility wrapper around UnmarshalFlex.
// Use this when you previously called jsonutil.Unmarshal(...) in the pipeline.
func Unmarshal(data []byte, v any) error {
	return UnmarshalFlex(data, v)
}

// UnmarshalRaw accepts json.RawMessage directly.
func UnmarshalRaw(raw json.RawMessage, v any) error {
	return UnmarshalFlex([]byte(raw), v)
}

// MarshalNoEscape encodes v into JSON without escaping <, >, & into \u003c, etc.
func MarshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Remove trailing newline from json.Encoder.Encode
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return out, nil
}

// MarshalNoEscapeIndent encodes v into JSON with indentation but without HTML escaping.
func MarshalNoEscapeIndent(v any, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	// encoding/json doesn't provide SetIndent on Encoder, so use intermediate step
	var tmp any
	if b, err := json.Marshal(v); err == nil {
		if err := json.Unmarshal(b, &tmp); err == nil {
			b2, err := json.MarshalIndent(tmp, prefix, indent)
			if err != nil {
				return nil, err
			}
			var tmp2 any
			if err := json.Unmarshal(b2, &tmp2); err != nil {
				return nil, err
			}
			return MarshalNoEscape(tmp2)
		}
	}
	// Fallback
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// UnescapeUnicodeString converts JSON unicode escapes like "\u003e" into actual characters.
// Handles double-escaped sequences like "\\u003e" -> "\u003e" -> ">".
func UnescapeUnicodeString(s string) (string, error) {
	// Trick: force JSON to treat the string as a quoted JSON string
	esc := strings.ReplaceAll(s, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	var out string
	if err := json.Unmarshal([]byte(`"`+esc+`"`), &out); err != nil {
		return "", err
	}
	return out, nil
}

// NormalizeJSONUnicode parses JSON bytes and recursively unescapes any remaining
// double-escaped unicode sequences (e.g. "\\u003e") inside string values.
// Useful before unmarshalling into a struct to remove HTML escape sequences.
func NormalizeJSONUnicode(raw []byte) ([]byte, error) {
	var anyVal any
	if err := json.Unmarshal(raw, &anyVal); err != nil {
		// Handle the case where the entire JSON is a quoted string
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, err
		}
		raw = []byte(s)
		if err := json.Unmarshal(raw, &anyVal); err != nil {
			// Try one more level of unwrapping if still encoded
			var s2 string
			if err3 := json.Unmarshal(raw, &s2); err3 == nil {
				if err := json.Unmarshal([]byte(s2), &anyVal); err == nil {
					goto DONE
				}
			}
			return nil, errors.New("NormalizeJSONUnicode: cannot parse JSON payload")
		}
	}
DONE:
	normalized := deepUnescape(anyVal)
	return MarshalNoEscape(normalized)
}

// UnmarshalFlex tries to unmarshal JSON bytes into v with best effort:
// 1) Direct unmarshal
// 2) Normalize and unmarshal
// This helps when JSON contains double-escaped unicode sequences.
func UnmarshalFlex(raw []byte, v any) error {
	// First try direct unmarshal
	if err := json.Unmarshal(raw, v); err == nil {
		return nil
	}
	// Normalize and try again
	norm, err := NormalizeJSONUnicode(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(norm, v)
}

// deepUnescape recursively traverses maps and slices,
// unescaping unicode sequences in all string values.
func deepUnescape(v any) any {
	switch x := v.(type) {
	case string:
		if s, err := UnescapeUnicodeString(x); err == nil {
			return s
		}
		return x
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = deepUnescape(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = deepUnescape(vv)
		}
		return out
	default:
		return v
	}
}
