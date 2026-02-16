package projectstore

import (
	"insightify/internal/gateway/application/projectport"
	"insightify/internal/gateway/entity"
)

type Adapter struct {
	store *Store
}

func NewAdapter(store *Store) *Adapter {
	return &Adapter{store: store}
}

func (a *Adapter) EnsureLoaded() {
	if a == nil || a.store == nil {
		return
	}
	a.store.EnsureLoaded()
}

func (a *Adapter) Save() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Save()
}

func (a *Adapter) Get(projectID string) (projectport.ProjectState, bool) {
	if a == nil || a.store == nil {
		return projectport.ProjectState{}, false
	}
	st, ok := a.store.Get(projectID)
	if !ok {
		return projectport.ProjectState{}, false
	}
	return toPortState(st), true
}

func (a *Adapter) Put(state projectport.ProjectState) {
	if a == nil || a.store == nil {
		return
	}
	a.store.Put(toStoreState(state))
}

func (a *Adapter) Update(projectID string, update func(*projectport.ProjectState)) (projectport.ProjectState, bool) {
	if a == nil || a.store == nil {
		return projectport.ProjectState{}, false
	}
	st, ok := a.store.Update(projectID, func(s *State) {
		ps := toPortState(*s)
		update(&ps)
		*s = toStoreState(ps)
	})
	if !ok {
		return projectport.ProjectState{}, false
	}
	return toPortState(st), true
}

func (a *Adapter) ListByUser(userID entity.UserID) []projectport.ProjectState {
	if a == nil || a.store == nil {
		return nil
	}
	list := a.store.ListByUser(userID.String())
	out := make([]projectport.ProjectState, 0, len(list))
	for _, st := range list {
		out = append(out, toPortState(st))
	}
	return out
}

func (a *Adapter) GetActiveByUser(userID entity.UserID) (projectport.ProjectState, bool) {
	if a == nil || a.store == nil {
		return projectport.ProjectState{}, false
	}
	st, ok := a.store.GetActiveByUser(userID.String())
	if !ok {
		return projectport.ProjectState{}, false
	}
	return toPortState(st), true
}

func (a *Adapter) SetActiveForUser(userID entity.UserID, projectID string) (projectport.ProjectState, bool) {
	if a == nil || a.store == nil {
		return projectport.ProjectState{}, false
	}
	st, ok := a.store.SetActiveForUser(userID.String(), projectID)
	if !ok {
		return projectport.ProjectState{}, false
	}
	return toPortState(st), true
}

func (a *Adapter) AddArtifact(artifact projectport.ProjectArtifact) error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.AddArtifact(ProjectArtifact{
		ID:        artifact.ID,
		ProjectID: artifact.ProjectID,
		RunID:     artifact.RunID,
		Path:      artifact.Path,
		CreatedAt: artifact.CreatedAt,
	})
}

func (a *Adapter) ListArtifacts(projectID string) ([]projectport.ProjectArtifact, error) {
	if a == nil || a.store == nil {
		return nil, nil
	}
	list, err := a.store.ListArtifacts(projectID)
	if err != nil {
		return nil, err
	}
	out := make([]projectport.ProjectArtifact, 0, len(list))
	for _, a := range list {
		out = append(out, projectport.ProjectArtifact{
			ID:        a.ID,
			ProjectID: a.ProjectID,
			RunID:     a.RunID,
			Path:      a.Path,
			CreatedAt: a.CreatedAt,
		})
	}
	return out, nil
}

func toPortState(st State) projectport.ProjectState {
	return projectport.ProjectState{
		ProjectID:   st.ProjectID,
		ProjectName: st.ProjectName,
		UserID:      entity.NormalizeUserID(st.UserID),
		Repo:        st.Repo,
		IsActive:    st.IsActive,
	}
}

func toStoreState(st projectport.ProjectState) State {
	return State{
		ProjectID:   st.ProjectID,
		ProjectName: st.ProjectName,
		UserID:      st.UserID.String(),
		Repo:        st.Repo,
		IsActive:    st.IsActive,
	}
}
