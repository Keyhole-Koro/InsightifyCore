package utils

import (
	"regexp"
	"testing"
)

func TestUIDGenerator_Generate_UniqueWithIncrement(t *testing.T) {
	g := NewUIDGenerator()

	first := g.Generate("Plan Node")
	second := g.Generate("Plan Node")
	third := g.Generate("Plan Node")

	if first == second || second == third || first == third {
		t.Fatalf("uids must be unique: %q %q %q", first, second, third)
	}
	if second != first+"-2" {
		t.Fatalf("expected second uid to increment: first=%q second=%q", first, second)
	}
	if third != first+"-3" {
		t.Fatalf("expected third uid to increment: first=%q third=%q", first, third)
	}
}

func TestUIDGenerator_Generate_UsesSlugAndHash(t *testing.T) {
	g := NewUIDGenerator()
	uid := g.Generate("My Fancy/Node#01")

	ok, err := regexp.MatchString(`^my-fancy-node-01-[0-9a-f]{8}$`, uid)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !ok {
		t.Fatalf("unexpected uid format: %q", uid)
	}
}

func TestUIDGenerator_Generate_RespectsExistingUIDs(t *testing.T) {
	g := NewUIDGenerator("node-aaaaaaaa")

	uid := g.Generate("node")
	if uid == "node-aaaaaaaa" {
		t.Fatalf("generated uid must avoid existing uid")
	}
}

func TestUIDGenerator_Generate_EmptyInputFallback(t *testing.T) {
	g := NewUIDGenerator()
	uid := g.Generate("   ")

	ok, err := regexp.MatchString(`^node-[0-9a-f]{8}$`, uid)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !ok {
		t.Fatalf("unexpected uid format: %q", uid)
	}
}
