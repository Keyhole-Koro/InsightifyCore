package delta

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Delta captures changes between two JSON-compatible values.
type Delta struct {
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []Mod    `json:"modified"`
}

// Mod records a single field change.
type Mod struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Options controls diff/apply behavior.
type Options struct {
	MaxChanges int
}

// Diff computes a delta between before and after.
func Diff(before, after any, opts Options) Delta {
	max := opts.MaxChanges
	if max <= 0 {
		max = 200
	}
	d := Delta{}
	diffAny("", normalizeJSON(before), normalizeJSON(after), &d, &max)
	Normalize(&d)
	return d
}

// Apply applies a delta to a JSON-compatible value.
// If mod.After is nil, map fields are deleted and array elements are set to nil.
func Apply(root any, d Delta) (any, error) {
	Normalize(&d)
	cur := normalizeJSON(root)
	for _, mod := range d.Modified {
		field := strings.TrimSpace(mod.Field)
		if field == "" || field == "$" {
			cur = normalizeJSON(mod.After)
			continue
		}
		if err := setJSONValue(cur, field, mod.After); err != nil {
			return nil, err
		}
	}
	return cur, nil
}

func diffAny(path string, before, after any, d *Delta, remaining *int) {
	if remaining != nil && *remaining <= 0 {
		return
	}
	if reflect.DeepEqual(before, after) {
		return
	}
	if beforeMap, ok := before.(map[string]any); ok {
		if afterMap, ok := after.(map[string]any); ok {
			for k, bv := range beforeMap {
				av, exists := afterMap[k]
				field := joinPath(path, k)
				if !exists {
					recordRemoved(d, field, bv, remaining)
					continue
				}
				diffAny(field, bv, av, d, remaining)
				if remaining != nil && *remaining <= 0 {
					return
				}
			}
			for k, av := range afterMap {
				if _, exists := beforeMap[k]; exists {
					continue
				}
				field := joinPath(path, k)
				recordAdded(d, field, av, remaining)
				if remaining != nil && *remaining <= 0 {
					return
				}
			}
			return
		}
	}
	if beforeArr, ok := before.([]any); ok {
		if afterArr, ok := after.([]any); ok {
			recordModified(d, path, beforeArr, afterArr, remaining)
			return
		}
	}
	recordModified(d, path, before, after, remaining)
}

func recordAdded(d *Delta, field string, after any, remaining *int) {
	if !recordModified(d, field, nil, after, remaining) {
		return
	}
	d.Added = append(d.Added, field)
}

func recordRemoved(d *Delta, field string, before any, remaining *int) {
	if !recordModified(d, field, before, nil, remaining) {
		return
	}
	d.Removed = append(d.Removed, field)
}

func recordModified(d *Delta, field string, before, after any, remaining *int) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		field = "$"
	}
	if remaining != nil {
		if *remaining <= 0 {
			return false
		}
		*remaining--
	}
	d.Modified = append(d.Modified, Mod{Field: field, Before: before, After: after})
	return true
}

// Normalize ensures delta slices are non-nil for downstream stability.
func Normalize(d *Delta) {
	if d == nil {
		return
	}
	if d.Added == nil {
		d.Added = []string{}
	}
	if d.Removed == nil {
		d.Removed = []string{}
	}
	if d.Modified == nil {
		d.Modified = []Mod{}
	}
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func normalizeJSON(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case json.RawMessage:
		var out any
		if err := json.Unmarshal(t, &out); err == nil {
			return out
		}
	case map[string]any, []any, string, float64, bool:
		return v
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

type pathToken struct {
	Key   string
	Index *int
}

func parsePathTokens(field string) ([]pathToken, error) {
	field = strings.TrimSpace(field)
	if field == "" {
		return nil, fmt.Errorf("delta: empty field path")
	}
	var tokens []pathToken
	parts := strings.Split(field, ".")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for len(part) > 0 {
			idx := strings.Index(part, "[")
			if idx == -1 {
				tokens = append(tokens, pathToken{Key: part})
				break
			}
			key := strings.TrimSpace(part[:idx])
			end := strings.Index(part[idx:], "]")
			if end == -1 {
				tokens = append(tokens, pathToken{Key: part})
				break
			}
			endIdx := idx + end
			numStr := strings.TrimSpace(part[idx+1 : endIdx])
			num := 0
			if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
				return nil, fmt.Errorf("delta: invalid index %q", numStr)
			}
			idxCopy := num
			tokens = append(tokens, pathToken{Key: key, Index: &idxCopy})
			part = part[endIdx+1:]
			if len(part) == 0 {
				break
			}
			if part[0] == '.' {
				part = part[1:]
			}
		}
	}
	return tokens, nil
}

func setJSONValue(root any, field string, value any) error {
	tokens, err := parsePathTokens(field)
	if err != nil {
		return err
	}
	var current any = root
	for idx, tok := range tokens {
		last := idx == len(tokens)-1
		switch node := current.(type) {
		case map[string]any:
			next, exists := node[tok.Key]
			if tok.Index != nil {
				arr, ok := next.([]any)
				if !exists || !ok {
					arr = make([]any, 0, *tok.Index+1)
				}
				targetIdx := *tok.Index
				for len(arr) <= targetIdx {
					arr = append(arr, nil)
				}
				if last {
					if value == nil {
						arr[targetIdx] = nil
					} else {
						arr[targetIdx] = value
					}
					node[tok.Key] = arr
					return nil
				}
				if arr[targetIdx] == nil {
					arr[targetIdx] = map[string]any{}
				}
				current = arr[targetIdx]
				node[tok.Key] = arr
				continue
			}
			if !exists {
				if last {
					if value == nil {
						delete(node, tok.Key)
					} else {
						node[tok.Key] = value
					}
					return nil
				}
				next = map[string]any{}
				node[tok.Key] = next
			}
			if last {
				if value == nil {
					delete(node, tok.Key)
				} else {
					node[tok.Key] = value
				}
				return nil
			}
			if child, ok := next.(map[string]any); ok {
				current = child
			} else {
				child = map[string]any{}
				node[tok.Key] = child
				current = child
			}
		case []any:
			if tok.Index == nil {
				return fmt.Errorf("delta: array segment missing index for %s", tok.Key)
			}
			targetIdx := *tok.Index
			arr := node
			for len(arr) <= targetIdx {
				arr = append(arr, nil)
			}
			if last {
				arr[targetIdx] = value
				current = arr
				continue
			}
			if arr[targetIdx] == nil {
				arr[targetIdx] = map[string]any{}
			}
			current = arr[targetIdx]
		default:
			return fmt.Errorf("delta: invalid type in path")
		}
	}
	return nil
}
