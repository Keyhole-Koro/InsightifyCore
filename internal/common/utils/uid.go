package utils

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
)

// UIDGenerator creates stable-ish UIDs from source IDs and resolves collisions.
// A generated UID shape is: "<slug>-<hash>" (or "<slug>-<hash>-N" on collision).
type UIDGenerator struct {
	used    map[string]struct{}
	counter map[string]int
	byKey   map[string]string
}

// NewUIDGenerator creates a generator with optional pre-reserved UIDs.
func NewUIDGenerator(existing ...string) *UIDGenerator {
	g := &UIDGenerator{
		used:    make(map[string]struct{}, len(existing)+8),
		counter: make(map[string]int, len(existing)+8),
		byKey:   make(map[string]string, len(existing)+8),
	}
	for _, uid := range existing {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		g.used[uid] = struct{}{}
	}
	return g
}

// Generate returns a unique UID for id.
func (g *UIDGenerator) Generate(id string) string {
	if g == nil {
		g = NewUIDGenerator()
	}
	base := baseUIDFromID(id)
	if _, ok := g.used[base]; !ok {
		g.used[base] = struct{}{}
		g.counter[base] = 1
		return base
	}
	n := g.counter[base]
	if n < 1 {
		n = 1
	}
	for {
		n++
		candidate := fmt.Sprintf("%s-%d", base, n)
		if _, exists := g.used[candidate]; exists {
			continue
		}
		g.used[candidate] = struct{}{}
		g.counter[base] = n
		return candidate
	}
}

// GenerateForKey returns a stable UID for a logical key.
// If the same key is passed again, the previously generated UID is returned.
func (g *UIDGenerator) GenerateForKey(key, id string) string {
	if g == nil {
		g = NewUIDGenerator()
	}
	key = strings.TrimSpace(key)
	if key != "" {
		if uid, ok := g.byKey[key]; ok {
			return uid
		}
	}
	uid := g.Generate(id)
	if key != "" {
		g.byKey[key] = uid
	}
	return uid
}

func baseUIDFromID(id string) string {
	id = strings.TrimSpace(id)
	slug := slugifyASCII(id)
	if slug == "" {
		slug = "node"
	}
	return fmt.Sprintf("%s-%s", slug, shortHashHex(id))
}

func shortHashHex(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	sum := h.Sum64()
	return fmt.Sprintf("%08x", uint32(sum&0xffffffff))
}

func slugifyASCII(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	return out
}
