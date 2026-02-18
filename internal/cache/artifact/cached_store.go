package artifact

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	memcache "insightify/internal/cache/memory"
	artifactrepo "insightify/internal/gateway/repository/artifact"
)

type Store = artifactrepo.Store

type CacheConfig struct {
	BlobTTL        time.Duration
	BlobMaxEntries int
	BlobMaxBytes   int

	ListTTL        time.Duration
	ListMaxEntries int

	URLTTL        time.Duration
	URLMaxEntries int
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		BlobTTL:        5 * time.Minute,
		BlobMaxEntries: 1024,
		BlobMaxBytes:   64 * 1024 * 1024, // 64MiB
		ListTTL:        30 * time.Second,
		ListMaxEntries: 512,
		URLTTL:         5 * time.Minute,
		URLMaxEntries:  1024,
	}
}

type MetricsSnapshot struct {
	BlobHits       uint64
	BlobMisses     uint64
	ListHits       uint64
	ListMisses     uint64
	URLHits        uint64
	URLMisses      uint64
	OriginReads    uint64
	OriginWrites   uint64
	OriginReadErr  uint64
	OriginWriteErr uint64
}

type Metrics struct {
	blobHits       atomic.Uint64
	blobMisses     atomic.Uint64
	listHits       atomic.Uint64
	listMisses     atomic.Uint64
	urlHits        atomic.Uint64
	urlMisses      atomic.Uint64
	originReads    atomic.Uint64
	originWrites   atomic.Uint64
	originReadErr  atomic.Uint64
	originWriteErr atomic.Uint64
}

func (m *Metrics) snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		BlobHits:       m.blobHits.Load(),
		BlobMisses:     m.blobMisses.Load(),
		ListHits:       m.listHits.Load(),
		ListMisses:     m.listMisses.Load(),
		URLHits:        m.urlHits.Load(),
		URLMisses:      m.urlMisses.Load(),
		OriginReads:    m.originReads.Load(),
		OriginWrites:   m.originWrites.Load(),
		OriginReadErr:  m.originReadErr.Load(),
		OriginWriteErr: m.originWriteErr.Load(),
	}
}

type CachedStore struct {
	origin Store

	blobCache *memcache.LRUTTL[string, []byte]
	listCache *memcache.LRUTTL[string, []string]
	urlCache  *memcache.LRUTTL[string, string]
	metrics   Metrics
}

func NewCachedStore(origin Store, cfg CacheConfig) *CachedStore {
	if cfg.BlobTTL <= 0 || cfg.BlobMaxEntries <= 0 || cfg.BlobMaxBytes < 0 {
		def := DefaultCacheConfig()
		if cfg.BlobTTL <= 0 {
			cfg.BlobTTL = def.BlobTTL
		}
		if cfg.BlobMaxEntries <= 0 {
			cfg.BlobMaxEntries = def.BlobMaxEntries
		}
		if cfg.BlobMaxBytes < 0 {
			cfg.BlobMaxBytes = def.BlobMaxBytes
		}
	}
	if cfg.ListTTL <= 0 || cfg.ListMaxEntries <= 0 {
		def := DefaultCacheConfig()
		if cfg.ListTTL <= 0 {
			cfg.ListTTL = def.ListTTL
		}
		if cfg.ListMaxEntries <= 0 {
			cfg.ListMaxEntries = def.ListMaxEntries
		}
	}
	if cfg.URLTTL <= 0 || cfg.URLMaxEntries <= 0 {
		def := DefaultCacheConfig()
		if cfg.URLTTL <= 0 {
			cfg.URLTTL = def.URLTTL
		}
		if cfg.URLMaxEntries <= 0 {
			cfg.URLMaxEntries = def.URLMaxEntries
		}
	}

	return &CachedStore{
		origin:    origin,
		blobCache: memcache.NewLRUTTL[string, []byte](cfg.BlobMaxEntries, cfg.BlobMaxBytes, cfg.BlobTTL),
		listCache: memcache.NewLRUTTL[string, []string](cfg.ListMaxEntries, 0, cfg.ListTTL),
		urlCache:  memcache.NewLRUTTL[string, string](cfg.URLMaxEntries, 0, cfg.URLTTL),
	}
}

func (s *CachedStore) Put(ctx context.Context, runID, path string, content []byte) error {
	s.metrics.originWrites.Add(1)
	if err := s.origin.Put(ctx, runID, path, content); err != nil {
		s.metrics.originWriteErr.Add(1)
		return err
	}

	key := artifactKey(runID, path)
	copied := append([]byte(nil), content...)
	s.blobCache.Set(key, copied, len(copied))
	s.listCache.Delete(strings.TrimSpace(runID))
	s.urlCache.Delete(key)
	return nil
}

func (s *CachedStore) Get(ctx context.Context, runID, path string) ([]byte, error) {
	key := artifactKey(runID, path)
	if raw, ok := s.blobCache.Get(key); ok {
		s.metrics.blobHits.Add(1)
		return append([]byte(nil), raw...), nil
	}
	s.metrics.blobMisses.Add(1)
	s.metrics.originReads.Add(1)

	raw, err := s.origin.Get(ctx, runID, path)
	if err != nil {
		s.metrics.originReadErr.Add(1)
		return nil, err
	}
	copied := append([]byte(nil), raw...)
	s.blobCache.Set(key, copied, len(copied))
	return append([]byte(nil), copied...), nil
}

func (s *CachedStore) GetURL(ctx context.Context, runID, path string) (string, error) {
	key := artifactKey(runID, path)
	if cached, ok := s.urlCache.Get(key); ok {
		s.metrics.urlHits.Add(1)
		return cached, nil
	}
	s.metrics.urlMisses.Add(1)
	s.metrics.originReads.Add(1)

	url, err := s.origin.GetURL(ctx, runID, path)
	if err != nil {
		s.metrics.originReadErr.Add(1)
		return "", err
	}
	if strings.TrimSpace(url) != "" {
		s.urlCache.Set(key, url, len(url))
	}
	return url, nil
}

func (s *CachedStore) List(ctx context.Context, runID string) ([]string, error) {
	runID = strings.TrimSpace(runID)
	if list, ok := s.listCache.Get(runID); ok {
		s.metrics.listHits.Add(1)
		return append([]string(nil), list...), nil
	}
	s.metrics.listMisses.Add(1)
	s.metrics.originReads.Add(1)

	list, err := s.origin.List(ctx, runID)
	if err != nil {
		s.metrics.originReadErr.Add(1)
		return nil, err
	}
	copied := append([]string(nil), list...)
	approxBytes := 0
	for _, v := range copied {
		approxBytes += len(v)
	}
	s.listCache.Set(runID, copied, approxBytes)
	return append([]string(nil), copied...), nil
}

func artifactKey(runID, path string) string {
	return strings.TrimSpace(runID) + "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
}

func (s *CachedStore) Metrics() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	return s.metrics.snapshot()
}
