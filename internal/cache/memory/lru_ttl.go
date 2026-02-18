package memory

import (
	"container/list"
	"sync"
	"time"
)

type entry[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time
	size      int
}

// LRUTTL is a threadsafe LRU cache with per-entry TTL.
type LRUTTL[K comparable, V any] struct {
	mu         sync.Mutex
	ll         *list.List
	items      map[K]*list.Element
	maxEntries int
	maxBytes   int
	totalBytes int
	ttl        time.Duration
}

func NewLRUTTL[K comparable, V any](maxEntries int, maxBytes int, ttl time.Duration) *LRUTTL[K, V] {
	if maxEntries <= 0 {
		maxEntries = 1
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &LRUTTL[K, V]{
		ll:         list.New(),
		items:      make(map[K]*list.Element),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
	}
}

func (c *LRUTTL[K, V]) Get(key K) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ele, ok := c.items[key]
	if !ok {
		return zero, false
	}
	ent := ele.Value.(*entry[K, V])
	if time.Now().After(ent.expiresAt) {
		c.removeElement(ele)
		return zero, false
	}
	c.ll.MoveToFront(ele)
	return ent.value, true
}

func (c *LRUTTL[K, V]) Set(key K, value V, sizeBytes int) {
	if c == nil {
		return
	}
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.items[key]; ok {
		ent := ele.Value.(*entry[K, V])
		c.totalBytes -= ent.size
		ent.value = value
		ent.size = sizeBytes
		ent.expiresAt = time.Now().Add(c.ttl)
		c.totalBytes += ent.size
		c.ll.MoveToFront(ele)
		c.evictLocked()
		return
	}

	ent := &entry[K, V]{
		key:       key,
		value:     value,
		size:      sizeBytes,
		expiresAt: time.Now().Add(c.ttl),
	}
	ele := c.ll.PushFront(ent)
	c.items[key] = ele
	c.totalBytes += sizeBytes
	c.evictLocked()
}

func (c *LRUTTL[K, V]) Delete(key K) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, ok := c.items[key]; ok {
		c.removeElement(ele)
	}
}

func (c *LRUTTL[K, V]) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll = list.New()
	c.items = make(map[K]*list.Element)
	c.totalBytes = 0
}

func (c *LRUTTL[K, V]) evictLocked() {
	for {
		if c.ll.Len() == 0 {
			return
		}
		if c.ll.Len() <= c.maxEntries && (c.maxBytes <= 0 || c.totalBytes <= c.maxBytes) {
			return
		}
		c.removeElement(c.ll.Back())
	}
}

func (c *LRUTTL[K, V]) removeElement(ele *list.Element) {
	if ele == nil {
		return
	}
	c.ll.Remove(ele)
	ent := ele.Value.(*entry[K, V])
	delete(c.items, ent.key)
	c.totalBytes -= ent.size
	if c.totalBytes < 0 {
		c.totalBytes = 0
	}
}
