package artifact

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type fakeOriginStore struct {
	mu sync.Mutex

	data map[string][]byte
	urls map[string]string

	getCalls  int
	putCalls  int
	listCalls int
	urlCalls  int

	failPut bool
}

func newFakeOriginStore() *fakeOriginStore {
	return &fakeOriginStore{
		data: map[string][]byte{},
		urls: map[string]string{},
	}
}

func (s *fakeOriginStore) Put(_ context.Context, runID, path string, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCalls++
	if s.failPut {
		return fmt.Errorf("put failed")
	}
	s.data[runID+"/"+path] = append([]byte(nil), content...)
	return nil
}

func (s *fakeOriginStore) Get(_ context.Context, runID, path string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	raw, ok := s.data[runID+"/"+path]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return append([]byte(nil), raw...), nil
}

func (s *fakeOriginStore) GetURL(_ context.Context, runID, path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.urlCalls++
	return s.urls[runID+"/"+path], nil
}

func (s *fakeOriginStore) List(_ context.Context, runID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	out := make([]string, 0, 8)
	prefix := runID + "/"
	for k := range s.data {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k[len(prefix):])
		}
	}
	return out, nil
}

func TestCachedStoreReadThroughAndMetrics(t *testing.T) {
	origin := newFakeOriginStore()
	origin.data["r1/a.txt"] = []byte("hello")
	store := NewCachedStore(origin, CacheConfig{
		BlobTTL: 1 * time.Minute, BlobMaxEntries: 8, BlobMaxBytes: 1024,
		ListTTL: 1 * time.Minute, ListMaxEntries: 8,
		URLTTL: 1 * time.Minute, URLMaxEntries: 8,
	})

	got1, err := store.Get(context.Background(), "r1", "a.txt")
	if err != nil {
		t.Fatalf("first get failed: %v", err)
	}
	got2, err := store.Get(context.Background(), "r1", "a.txt")
	if err != nil {
		t.Fatalf("second get failed: %v", err)
	}
	if string(got1) != "hello" || string(got2) != "hello" {
		t.Fatalf("unexpected content: %q %q", got1, got2)
	}
	if origin.getCalls != 1 {
		t.Fatalf("expected one origin get call, got %d", origin.getCalls)
	}
	m := store.Metrics()
	if m.BlobHits != 1 || m.BlobMisses != 1 || m.OriginReads != 1 {
		t.Fatalf("unexpected metrics: %+v", m)
	}
}

func TestCachedStoreWriteThrough(t *testing.T) {
	origin := newFakeOriginStore()
	store := NewCachedStore(origin, DefaultCacheConfig())

	if err := store.Put(context.Background(), "r1", "a.txt", []byte("new")); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	got, err := store.Get(context.Background(), "r1", "a.txt")
	if err != nil {
		t.Fatalf("get after put failed: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("unexpected content: %q", got)
	}
	if origin.putCalls != 1 {
		t.Fatalf("expected one origin put call, got %d", origin.putCalls)
	}

	origin.failPut = true
	if err := store.Put(context.Background(), "r1", "b.txt", []byte("bad")); err == nil {
		t.Fatalf("expected put error")
	}
	if _, err := store.Get(context.Background(), "r1", "b.txt"); err == nil {
		t.Fatalf("expected cache/origin miss for failed write")
	}
}

func TestCachedStoreTTLAndLRU(t *testing.T) {
	origin := newFakeOriginStore()
	origin.data["r1/a.txt"] = []byte("A")
	origin.data["r1/b.txt"] = []byte("B")

	store := NewCachedStore(origin, CacheConfig{
		BlobTTL: 20 * time.Millisecond, BlobMaxEntries: 1, BlobMaxBytes: 1024,
		ListTTL: 1 * time.Minute, ListMaxEntries: 8,
		URLTTL: 1 * time.Minute, URLMaxEntries: 8,
	})

	if _, err := store.Get(context.Background(), "r1", "a.txt"); err != nil {
		t.Fatalf("get a failed: %v", err)
	}
	if _, err := store.Get(context.Background(), "r1", "b.txt"); err != nil {
		t.Fatalf("get b failed: %v", err)
	}
	// LRU maxEntries=1, so a.txt should be evicted and hit origin again.
	if _, err := store.Get(context.Background(), "r1", "a.txt"); err != nil {
		t.Fatalf("get a(again) failed: %v", err)
	}
	if origin.getCalls != 3 {
		t.Fatalf("expected 3 origin get calls with LRU eviction, got %d", origin.getCalls)
	}

	origin.getCalls = 0
	store2 := NewCachedStore(origin, CacheConfig{
		BlobTTL: 10 * time.Millisecond, BlobMaxEntries: 8, BlobMaxBytes: 1024,
		ListTTL: 1 * time.Minute, ListMaxEntries: 8,
		URLTTL: 1 * time.Minute, URLMaxEntries: 8,
	})
	if _, err := store2.Get(context.Background(), "r1", "a.txt"); err != nil {
		t.Fatalf("ttl get first failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := store2.Get(context.Background(), "r1", "a.txt"); err != nil {
		t.Fatalf("ttl get second failed: %v", err)
	}
	if origin.getCalls != 2 {
		t.Fatalf("expected 2 origin reads after ttl expiry, got %d", origin.getCalls)
	}
}

func TestCachedStoreListAndURL(t *testing.T) {
	origin := newFakeOriginStore()
	origin.data["run-1/p1"] = []byte("x")
	origin.data["run-1/p2"] = []byte("y")
	origin.urls["run-1/p1"] = "https://example/p1"

	store := NewCachedStore(origin, DefaultCacheConfig())

	l1, err := store.List(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("list1 failed: %v", err)
	}
	l2, err := store.List(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("list2 failed: %v", err)
	}
	if !reflect.DeepEqual(l1, l2) {
		t.Fatalf("list mismatch: %#v %#v", l1, l2)
	}
	if origin.listCalls != 1 {
		t.Fatalf("expected one origin list call, got %d", origin.listCalls)
	}

	u1, err := store.GetURL(context.Background(), "run-1", "p1")
	if err != nil {
		t.Fatalf("url1 failed: %v", err)
	}
	u2, err := store.GetURL(context.Background(), "run-1", "p1")
	if err != nil {
		t.Fatalf("url2 failed: %v", err)
	}
	if u1 != u2 {
		t.Fatalf("url mismatch: %s vs %s", u1, u2)
	}
	if origin.urlCalls != 1 {
		t.Fatalf("expected one origin url call, got %d", origin.urlCalls)
	}
}
