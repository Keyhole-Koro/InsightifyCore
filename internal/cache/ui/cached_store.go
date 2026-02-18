package ui

import (
	"context"
	"strings"
	"time"

	memcache "insightify/internal/cache/memory"
	uirepo "insightify/internal/gateway/repository/ui"

	"google.golang.org/protobuf/proto"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type Store = uirepo.Store

type CacheConfig struct {
	DocTTL        time.Duration
	DocMaxEntries int
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		DocTTL:        2 * time.Minute,
		DocMaxEntries: 2048,
	}
}

type CachedStore struct {
	origin Store
	docs   *memcache.LRUTTL[string, *insightifyv1.UiDocument]
}

func NewCachedStore(origin Store, cfg CacheConfig) *CachedStore {
	if cfg.DocTTL <= 0 || cfg.DocMaxEntries <= 0 {
		def := DefaultCacheConfig()
		if cfg.DocTTL <= 0 {
			cfg.DocTTL = def.DocTTL
		}
		if cfg.DocMaxEntries <= 0 {
			cfg.DocMaxEntries = def.DocMaxEntries
		}
	}
	return &CachedStore{
		origin: origin,
		docs:   memcache.NewLRUTTL[string, *insightifyv1.UiDocument](cfg.DocMaxEntries, 0, cfg.DocTTL),
	}
}

func (s *CachedStore) GetDocument(ctx context.Context, runID string) (*insightifyv1.UiDocument, error) {
	key := strings.TrimSpace(runID)
	if doc, ok := s.docs.Get(key); ok {
		return cloneDoc(doc), nil
	}
	doc, err := s.origin.GetDocument(ctx, key)
	if err != nil {
		return nil, err
	}
	cloned := cloneDoc(doc)
	if cloned != nil {
		s.docs.Set(key, cloned, len(cloned.GetNodes()))
	}
	return cloneDoc(cloned), nil
}

func (s *CachedStore) ApplyOps(ctx context.Context, runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error) {
	doc, conflict, err := s.origin.ApplyOps(ctx, runID, baseVersion, ops)
	if err != nil {
		return nil, false, err
	}
	key := strings.TrimSpace(runID)
	cloned := cloneDoc(doc)
	if cloned != nil {
		s.docs.Set(key, cloned, len(cloned.GetNodes()))
	} else {
		s.docs.Delete(key)
	}
	return cloneDoc(cloned), conflict, nil
}

func cloneDoc(doc *insightifyv1.UiDocument) *insightifyv1.UiDocument {
	if doc == nil {
		return nil
	}
	cloned, ok := proto.Clone(doc).(*insightifyv1.UiDocument)
	if !ok {
		return nil
	}
	return cloned
}
