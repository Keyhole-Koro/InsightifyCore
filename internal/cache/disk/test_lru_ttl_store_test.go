package disk

import (
	"context"
	"testing"
	"time"
)

func TestLRUTTLStoreTTLExpiry(t *testing.T) {
	root := t.TempDir()
	store, err := NewLRUTTLStore(LRUTTLConfig{Root: root, TTL: 30 * time.Millisecond, MaxEntries: 10})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	if err := store.Set(ctx, "k1", []byte("v1"), 2); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, ok, err := store.Get(ctx, "k1"); err != nil || !ok {
		t.Fatalf("get before expiry: ok=%v err=%v", ok, err)
	}

	time.Sleep(60 * time.Millisecond)
	if _, ok, err := store.Get(ctx, "k1"); err != nil {
		t.Fatalf("get after expiry: %v", err)
	} else if ok {
		t.Fatalf("expected miss after ttl expiry")
	}
}

func TestLRUTTLStoreEvictsLeastRecentlyUsed(t *testing.T) {
	root := t.TempDir()
	store, err := NewLRUTTLStore(LRUTTLConfig{Root: root, TTL: time.Minute, MaxEntries: 2})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	if err := store.Set(ctx, "a", []byte("aa"), 2); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := store.Set(ctx, "b", []byte("bb"), 2); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if _, ok, err := store.Get(ctx, "a"); err != nil || !ok {
		t.Fatalf("touch a: ok=%v err=%v", ok, err)
	}
	if err := store.Set(ctx, "c", []byte("cc"), 2); err != nil {
		t.Fatalf("set c: %v", err)
	}

	if _, ok, err := store.Get(ctx, "b"); err != nil {
		t.Fatalf("get b: %v", err)
	} else if ok {
		t.Fatalf("expected b to be evicted")
	}
	if _, ok, err := store.Get(ctx, "a"); err != nil || !ok {
		t.Fatalf("expected a to remain: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Get(ctx, "c"); err != nil || !ok {
		t.Fatalf("expected c to remain: ok=%v err=%v", ok, err)
	}
}

func TestLRUTTLStoreRestoresFromIndex(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	store, err := NewLRUTTLStore(LRUTTLConfig{Root: root, TTL: time.Minute, MaxEntries: 10})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Set(ctx, "persist", []byte("value"), 5); err != nil {
		t.Fatalf("set persist: %v", err)
	}

	store2, err := NewLRUTTLStore(LRUTTLConfig{Root: root, TTL: time.Minute, MaxEntries: 10})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	raw, ok, err := store2.Get(ctx, "persist")
	if err != nil {
		t.Fatalf("get persist: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted key to exist")
	}
	if string(raw) != "value" {
		t.Fatalf("unexpected value: %q", string(raw))
	}
}
