package project

import (
	"context"
	"time"

	memcache "insightify/internal/cache/memory"
	"insightify/internal/gateway/entity"
	projectrepo "insightify/internal/gateway/repository/project"
)

type State = projectrepo.State
type ProjectArtifact = projectrepo.ProjectArtifact
type Repository = projectrepo.Repository
type ArtifactRepository = projectrepo.ArtifactRepository

type CacheConfig struct {
	StateTTL        time.Duration
	StateMaxEntries int
	ListTTL         time.Duration
	ListMaxEntries  int
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		StateTTL:        5 * time.Minute,
		StateMaxEntries: 2048,
		ListTTL:         30 * time.Second,
		ListMaxEntries:  1024,
	}
}

type CachedStore struct {
	origin Repository
	meta   ArtifactRepository

	byProject  *memcache.LRUTTL[string, State]
	byUserList *memcache.LRUTTL[string, []State]
	byUserAct  *memcache.LRUTTL[string, State]
	artifacts  *memcache.LRUTTL[string, []ProjectArtifact]
}

func NewCachedStore(origin Repository, meta ArtifactRepository, cfg CacheConfig) *CachedStore {
	if cfg.StateTTL <= 0 || cfg.StateMaxEntries <= 0 || cfg.ListTTL <= 0 || cfg.ListMaxEntries <= 0 {
		def := DefaultCacheConfig()
		if cfg.StateTTL <= 0 {
			cfg.StateTTL = def.StateTTL
		}
		if cfg.StateMaxEntries <= 0 {
			cfg.StateMaxEntries = def.StateMaxEntries
		}
		if cfg.ListTTL <= 0 {
			cfg.ListTTL = def.ListTTL
		}
		if cfg.ListMaxEntries <= 0 {
			cfg.ListMaxEntries = def.ListMaxEntries
		}
	}
	return &CachedStore{
		origin:     origin,
		meta:       meta,
		byProject:  memcache.NewLRUTTL[string, State](cfg.StateMaxEntries, 0, cfg.StateTTL),
		byUserList: memcache.NewLRUTTL[string, []State](cfg.ListMaxEntries, 0, cfg.ListTTL),
		byUserAct:  memcache.NewLRUTTL[string, State](cfg.ListMaxEntries, 0, cfg.ListTTL),
		artifacts:  memcache.NewLRUTTL[string, []ProjectArtifact](cfg.ListMaxEntries, 0, cfg.ListTTL),
	}
}

func (s *CachedStore) EnsureLoaded(ctx context.Context) {
	s.origin.EnsureLoaded(ctx)
}

func (s *CachedStore) Save(ctx context.Context) error {
	return s.origin.Save(ctx)
}

func (s *CachedStore) Get(ctx context.Context, projectID string) (State, bool) {
	if st, ok := s.byProject.Get(projectID); ok {
		return st, true
	}
	st, ok := s.origin.Get(ctx, projectID)
	if ok {
		s.byProject.Set(projectID, st, 1)
	}
	return st, ok
}

func (s *CachedStore) Put(ctx context.Context, state State) error {
	if err := s.origin.Put(ctx, state); err != nil {
		return err
	}
	s.byProject.Set(state.ProjectID, state, 1)
	s.byUserList.Delete(state.UserID.String())
	s.byUserAct.Delete(state.UserID.String())
	return nil
}

func (s *CachedStore) Update(ctx context.Context, projectID string, update func(*State)) (State, bool, error) {
	st, ok, err := s.origin.Update(ctx, projectID, update)
	if err != nil {
		return State{}, false, err
	}
	if !ok {
		return State{}, false, nil
	}
	s.byProject.Set(projectID, st, 1)
	s.byUserList.Delete(st.UserID.String())
	s.byUserAct.Delete(st.UserID.String())
	return st, true, nil
}

func (s *CachedStore) ListByUser(ctx context.Context, userID entity.UserID) ([]State, error) {
	k := userID.String()
	if list, ok := s.byUserList.Get(k); ok {
		return cloneStates(list), nil
	}
	list, err := s.origin.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	copied := cloneStates(list)
	s.byUserList.Set(k, copied, len(copied))
	for _, st := range copied {
		s.byProject.Set(st.ProjectID, st, 1)
	}
	return cloneStates(copied), nil
}

func (s *CachedStore) GetActiveByUser(ctx context.Context, userID entity.UserID) (State, bool, error) {
	k := userID.String()
	if st, ok := s.byUserAct.Get(k); ok {
		return st, true, nil
	}
	st, ok, err := s.origin.GetActiveByUser(ctx, userID)
	if err != nil {
		return State{}, false, err
	}
	if ok {
		s.byUserAct.Set(k, st, 1)
		s.byProject.Set(st.ProjectID, st, 1)
	}
	return st, ok, nil
}

func (s *CachedStore) SetActiveForUser(ctx context.Context, userID entity.UserID, projectID string) (State, bool, error) {
	st, ok, err := s.origin.SetActiveForUser(ctx, userID, projectID)
	if err != nil {
		return State{}, false, err
	}
	if !ok {
		return State{}, false, nil
	}
	s.byUserAct.Set(userID.String(), st, 1)
	s.byUserList.Delete(userID.String())
	s.byProject.Set(st.ProjectID, st, 1)
	return st, true, nil
}

func (s *CachedStore) AddArtifact(ctx context.Context, artifact ProjectArtifact) error {
	if err := s.meta.AddArtifact(ctx, artifact); err != nil {
		return err
	}
	s.artifacts.Delete(artifact.ProjectID)
	return nil
}

func (s *CachedStore) ListArtifacts(ctx context.Context, projectID string) ([]ProjectArtifact, error) {
	if list, ok := s.artifacts.Get(projectID); ok {
		return cloneArtifacts(list), nil
	}
	list, err := s.meta.ListArtifacts(ctx, projectID)
	if err != nil {
		return nil, err
	}
	copied := cloneArtifacts(list)
	s.artifacts.Set(projectID, copied, len(copied))
	return cloneArtifacts(copied), nil
}

func cloneStates(in []State) []State {
	if len(in) == 0 {
		return nil
	}
	out := make([]State, len(in))
	copy(out, in)
	return out
}

func cloneArtifacts(in []ProjectArtifact) []ProjectArtifact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProjectArtifact, len(in))
	copy(out, in)
	return out
}
