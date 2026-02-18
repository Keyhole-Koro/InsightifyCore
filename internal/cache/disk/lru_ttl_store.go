package disk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type LRUTTLConfig struct {
	Root       string
	IndexFile  string
	MaxEntries int
	MaxBytes   int64
	TTL        time.Duration
}

type diskEntry struct {
	File       string    `json:"file"`
	Size       int64     `json:"size"`
	ExpiresAt  time.Time `json:"expires_at"`
	AccessedAt time.Time `json:"accessed_at"`
}

type diskIndex struct {
	Entries map[string]diskEntry `json:"entries"`
}

// LRUTTLStore persists values on disk and keeps an index for TTL/LRU eviction.
type LRUTTLStore struct {
	mu sync.Mutex

	root      string
	dataDir   string
	indexPath string

	maxEntries int
	maxBytes   int64
	ttl        time.Duration

	totalBytes int64
	entries    map[string]diskEntry
}

func NewLRUTTLStore(cfg LRUTTLConfig) (*LRUTTLStore, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, fmt.Errorf("root is required")
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * time.Second
	}
	indexFile := strings.TrimSpace(cfg.IndexFile)
	if indexFile == "" {
		indexFile = "index.json"
	}

	s := &LRUTTLStore{
		root:       root,
		dataDir:    filepath.Join(root, "data"),
		indexPath:  filepath.Join(root, indexFile),
		maxEntries: cfg.MaxEntries,
		maxBytes:   cfg.MaxBytes,
		ttl:        cfg.TTL,
		entries:    map[string]diskEntry{},
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := s.loadIndex(); err != nil {
		return nil, err
	}
	if err := s.cleanupAndEvictLocked(time.Now()); err != nil {
		return nil, err
	}
	if err := s.persistIndexLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *LRUTTLStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("store is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false, fmt.Errorf("key is required")
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	ent, ok := s.entries[key]
	if !ok {
		return nil, false, nil
	}
	if now.After(ent.ExpiresAt) {
		s.removeEntryLocked(key, ent)
		_ = s.persistIndexLocked()
		return nil, false, nil
	}
	path := filepath.Join(s.dataDir, ent.File)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.removeEntryLocked(key, ent)
			_ = s.persistIndexLocked()
			return nil, false, nil
		}
		return nil, false, err
	}
	ent.AccessedAt = now
	s.entries[key] = ent
	if err := s.persistIndexLocked(); err != nil {
		return nil, false, err
	}
	return append([]byte(nil), raw...), true, nil
}

func (s *LRUTTLStore) Set(_ context.Context, key string, value []byte, sizeBytes int) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if sizeBytes < 0 {
		sizeBytes = 0
	}

	now := time.Now()
	file := hashedName(key)
	path := filepath.Join(s.dataDir, file)

	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.entries[key]; ok {
		s.totalBytes -= old.Size
	}
	if err := os.WriteFile(path, value, 0o644); err != nil {
		return err
	}
	s.entries[key] = diskEntry{
		File:       file,
		Size:       int64(sizeBytes),
		ExpiresAt:  now.Add(s.ttl),
		AccessedAt: now,
	}
	s.totalBytes += int64(sizeBytes)

	if err := s.cleanupAndEvictLocked(now); err != nil {
		return err
	}
	return s.persistIndexLocked()
}

func (s *LRUTTLStore) Delete(_ context.Context, key string) error {
	if s == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ent, ok := s.entries[key]; ok {
		s.removeEntryLocked(key, ent)
		return s.persistIndexLocked()
	}
	return nil
}

func (s *LRUTTLStore) Clear(_ context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ent := range s.entries {
		_ = os.Remove(filepath.Join(s.dataDir, ent.File))
	}
	s.entries = map[string]diskEntry{}
	s.totalBytes = 0
	return s.persistIndexLocked()
}

func (s *LRUTTLStore) loadIndex() error {
	raw, err := os.ReadFile(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.entries = map[string]diskEntry{}
			s.totalBytes = 0
			return nil
		}
		return err
	}
	var idx diskIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]diskEntry{}
	}
	s.entries = idx.Entries
	s.totalBytes = 0
	for _, ent := range s.entries {
		s.totalBytes += ent.Size
	}
	return nil
}

func (s *LRUTTLStore) cleanupAndEvictLocked(now time.Time) error {
	for key, ent := range s.entries {
		if now.After(ent.ExpiresAt) {
			s.removeEntryLocked(key, ent)
			continue
		}
		if _, err := os.Stat(filepath.Join(s.dataDir, ent.File)); err != nil {
			if os.IsNotExist(err) {
				s.removeEntryLocked(key, ent)
				continue
			}
			return err
		}
	}

	for s.needsEvictionLocked() {
		key, ent, ok := s.leastRecentlyUsedLocked()
		if !ok {
			break
		}
		s.removeEntryLocked(key, ent)
	}
	return nil
}

func (s *LRUTTLStore) needsEvictionLocked() bool {
	if len(s.entries) == 0 {
		return false
	}
	if len(s.entries) > s.maxEntries {
		return true
	}
	if s.maxBytes > 0 && s.totalBytes > s.maxBytes {
		return true
	}
	return false
}

func (s *LRUTTLStore) leastRecentlyUsedLocked() (string, diskEntry, bool) {
	if len(s.entries) == 0 {
		return "", diskEntry{}, false
	}
	keys := make([]string, 0, len(s.entries))
	for key := range s.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		li := s.entries[keys[i]].AccessedAt
		lj := s.entries[keys[j]].AccessedAt
		if li.Equal(lj) {
			return keys[i] < keys[j]
		}
		return li.Before(lj)
	})
	k := keys[0]
	return k, s.entries[k], true
}

func (s *LRUTTLStore) removeEntryLocked(key string, ent diskEntry) {
	delete(s.entries, key)
	s.totalBytes -= ent.Size
	if s.totalBytes < 0 {
		s.totalBytes = 0
	}
	_ = os.Remove(filepath.Join(s.dataDir, ent.File))
}

func (s *LRUTTLStore) persistIndexLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.indexPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(diskIndex{Entries: s.entries}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.indexPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.indexPath)
}

func hashedName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:]) + ".bin"
}
