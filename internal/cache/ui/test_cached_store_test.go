package ui

import (
	"context"
	"testing"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type fakeUIOrigin struct {
	getCalls int
	opsCalls int
	doc      *insightifyv1.UiDocument
}

func (f *fakeUIOrigin) GetDocument(_ context.Context, runID string) (*insightifyv1.UiDocument, error) {
	f.getCalls++
	if f.doc == nil {
		return &insightifyv1.UiDocument{RunId: runID}, nil
	}
	return f.doc, nil
}

func (f *fakeUIOrigin) ApplyOps(_ context.Context, runID string, _ int64, _ []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error) {
	f.opsCalls++
	if f.doc == nil {
		f.doc = &insightifyv1.UiDocument{RunId: runID, Version: 1}
	} else {
		f.doc.Version++
	}
	return f.doc, false, nil
}

func TestUICachedStore_ReadThroughAndWriteThrough(t *testing.T) {
	origin := &fakeUIOrigin{
		doc: &insightifyv1.UiDocument{RunId: "r1", Version: 1},
	}
	store := NewCachedStore(origin, CacheConfig{DocTTL: time.Minute, DocMaxEntries: 8})

	doc1, err := store.GetDocument(context.Background(), "r1")
	if err != nil {
		t.Fatalf("get1 failed: %v", err)
	}
	doc2, err := store.GetDocument(context.Background(), "r1")
	if err != nil {
		t.Fatalf("get2 failed: %v", err)
	}
	if doc1.GetVersion() != 1 || doc2.GetVersion() != 1 {
		t.Fatalf("unexpected versions: %d %d", doc1.GetVersion(), doc2.GetVersion())
	}
	if origin.getCalls != 1 {
		t.Fatalf("expected one origin get, got %d", origin.getCalls)
	}

	updated, conflict, err := store.ApplyOps(context.Background(), "r1", 1, nil)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if conflict {
		t.Fatalf("unexpected conflict")
	}
	if updated.GetVersion() != 2 {
		t.Fatalf("expected version 2 after apply, got %d", updated.GetVersion())
	}

	doc3, err := store.GetDocument(context.Background(), "r1")
	if err != nil {
		t.Fatalf("get3 failed: %v", err)
	}
	if doc3.GetVersion() != 2 {
		t.Fatalf("expected cached updated version 2, got %d", doc3.GetVersion())
	}
}
