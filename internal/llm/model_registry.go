package llm

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	llmclient "insightify/internal/llmClient"
)

// ModelLevel represents the capability tier of a model.
type ModelLevel string

const (
	ModelLevelLow    ModelLevel = "low"
	ModelLevelMiddle ModelLevel = "middle"
	ModelLevelHigh   ModelLevel = "high"
	ModelLevelXHigh  ModelLevel = "xhigh"
)

// ModelRole represents the intended use of a model.
type ModelRole string

const (
	ModelRoleWorker  ModelRole = "worker"
	ModelRolePlanner ModelRole = "planner"
)

// TokenCountFunc counts tokens in a string.
type TokenCountFunc func(string) int

// ModelProfile describes a registered model's properties.
type ModelProfile struct {
	Provider    string
	Tier        string
	Model       string
	Name        string
	Level       ModelLevel
	MaxTokens   int
	CountTokens TokenCountFunc
	Meta        map[string]any
	RateLimit   *llmclient.RateLimitConfig
}

// RegisteredModel pairs a profile with its factory.
type RegisteredModel struct {
	Profile ModelProfile
	Factory llmclient.ClientFactory
}

var (
	ErrModelNotRegistered = errors.New("llm model profile is not registered")
	ErrModelLevelRequired = errors.New("llm model level is required")
)

// InMemoryModelRegistry stores model registrations in memory.
type InMemoryModelRegistry struct {
	mu       sync.RWMutex
	models   map[string]RegisteredModel
	defaults map[ModelRole]map[ModelLevel]string
	byLevel  map[ModelLevel][]string
}

// NewInMemoryModelRegistry creates a new empty registry.
func NewInMemoryModelRegistry() *InMemoryModelRegistry {
	return &InMemoryModelRegistry{
		models:   map[string]RegisteredModel{},
		defaults: map[ModelRole]map[ModelLevel]string{},
		byLevel:  map[ModelLevel][]string{},
	}
}

func normalizeRole(role ModelRole) ModelRole {
	switch role {
	case ModelRolePlanner:
		return ModelRolePlanner
	case ModelRoleWorker:
		return ModelRoleWorker
	default:
		return ModelRoleWorker
	}
}

func normalizeLevel(level ModelLevel) ModelLevel {
	switch level {
	case ModelLevelLow:
		return ModelLevelLow
	case ModelLevelHigh:
		return ModelLevelHigh
	case ModelLevelXHigh:
		return ModelLevelXHigh
	case ModelLevelMiddle:
		return ModelLevelMiddle
	default:
		return ""
	}
}

func keyFor(provider, model string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	return provider + "::" + model
}

// RegisterModel adds a model to the registry.
func (r *InMemoryModelRegistry) RegisterModel(spec llmclient.ModelRegistration) error {
	if spec.Factory == nil {
		return fmt.Errorf("register model: factory is nil")
	}
	level := normalizeLevel(ModelLevel(spec.Level))
	if level == "" {
		return fmt.Errorf("register model: invalid level %q", spec.Level)
	}
	provider := strings.ToLower(strings.TrimSpace(spec.Provider))
	model := strings.TrimSpace(spec.Model)
	if provider == "" || model == "" {
		return fmt.Errorf("register model: provider and model are required")
	}

	entry := RegisteredModel{
		Profile: ModelProfile{
			Provider:  provider,
			Tier:      strings.TrimSpace(spec.Tier),
			Model:     model,
			Name:      provider + ":" + model,
			Level:     level,
			MaxTokens: spec.MaxTokens,
			Meta:      spec.Meta,
			RateLimit: spec.RateLimit,
		},
		Factory: spec.Factory,
	}

	k := keyFor(provider, model)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[k]; !ok {
		r.byLevel[level] = append(r.byLevel[level], k)
	}
	r.models[k] = entry
	return nil
}

// SetDefault sets the default model for a role/level combination.
func (r *InMemoryModelRegistry) SetDefault(role ModelRole, level ModelLevel, provider, model string) error {
	role = normalizeRole(role)
	level = normalizeLevel(level)
	if level == "" {
		return ErrModelLevelRequired
	}
	k := keyFor(provider, model)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[k]; !ok {
		return fmt.Errorf("%w: provider=%s model=%s", ErrModelNotRegistered, provider, model)
	}
	bucket, ok := r.defaults[role]
	if !ok {
		bucket = map[ModelLevel]string{}
		r.defaults[role] = bucket
	}
	bucket[level] = k
	return nil
}

// Resolve finds a model for the given role/level, optionally overriding provider/model.
func (r *InMemoryModelRegistry) Resolve(role ModelRole, level ModelLevel, provider, model string) (RegisteredModel, error) {
	role = normalizeRole(role)
	level = normalizeLevel(level)
	if level == "" {
		return RegisteredModel{}, ErrModelLevelRequired
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	defaultKey := ""
	if byRole, ok := r.defaults[role]; ok {
		defaultKey = byRole[level]
	}

	if strings.TrimSpace(provider) != "" || strings.TrimSpace(model) != "" {
		if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
			if defaultKey == "" {
				return RegisteredModel{}, fmt.Errorf("%w: role=%s level=%s", ErrModelNotRegistered, role, level)
			}
			parts := strings.SplitN(defaultKey, "::", 2)
			if len(parts) == 2 {
				if strings.TrimSpace(provider) == "" {
					provider = parts[0]
				}
				if strings.TrimSpace(model) == "" {
					model = parts[1]
				}
			}
		}
		k := keyFor(provider, model)
		if m, ok := r.models[k]; ok {
			return m, nil
		}
		return RegisteredModel{}, fmt.Errorf("%w: provider=%s model=%s", ErrModelNotRegistered, provider, model)
	}

	if defaultKey != "" {
		if m, ok := r.models[defaultKey]; ok {
			return m, nil
		}
	}

	if keys := r.byLevel[level]; len(keys) > 0 {
		k := keys[0]
		if m, ok := r.models[k]; ok {
			return m, nil
		}
	}
	return RegisteredModel{}, fmt.Errorf("%w: role=%s level=%s", ErrModelNotRegistered, role, level)
}

// Candidates returns all models for a given role/level.
func (r *InMemoryModelRegistry) Candidates(role ModelRole, level ModelLevel) []RegisteredModel {
	role = normalizeRole(role)
	level = normalizeLevel(level)
	if level == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]string, 0)
	seen := map[string]struct{}{}
	if byRole, ok := r.defaults[role]; ok {
		if k := byRole[level]; k != "" {
			keys = append(keys, k)
			seen[k] = struct{}{}
		}
	}
	for _, k := range r.byLevel[level] {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	out := make([]RegisteredModel, 0, len(keys))
	for _, k := range keys {
		if m, ok := r.models[k]; ok {
			out = append(out, m)
		}
	}
	return out
}

// BuildClient creates a client for the resolved model with rate limits applied.
func (r *InMemoryModelRegistry) BuildClient(
	ctx context.Context,
	role ModelRole,
	level ModelLevel,
	provider, model string,
	tokenCap int,
) (llmclient.LLMClient, error) {
	entry, err := r.Resolve(role, level, provider, model)
	if err != nil {
		return nil, err
	}
	cli, err := entry.Factory(ctx, tokenCap)
	if err != nil {
		return nil, err
	}

	if rl := entry.Profile.RateLimit; rl != nil {
		if rl.RPM > 0 || rl.RPD > 0 || rl.TPM > 0 {
			cli = MultiLimit(rl.RPM, rl.RPD, rl.TPM)(cli)
		}
		if rl.TPD > 0 {
			cli = TokenDayLimit(rl.TPD)(cli)
		}
		if rl.RPS > 0 || rl.Burst > 0 {
			cli = RateLimit(rl.RPS, rl.Burst)(cli)
		}
	}
	return cli, nil
}

// DefaultsSalt returns a deterministic string representing the current defaults.
func (r *InMemoryModelRegistry) DefaultsSalt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	parts := make([]string, 0)
	roles := []ModelRole{ModelRoleWorker, ModelRolePlanner}
	levels := []ModelLevel{ModelLevelLow, ModelLevelMiddle, ModelLevelHigh, ModelLevelXHigh}
	for _, role := range roles {
		bucket := r.defaults[role]
		for _, level := range levels {
			if bucket == nil {
				continue
			}
			if key := bucket[level]; key != "" {
				parts = append(parts, fmt.Sprintf("%s_%s=%s", role, level, key))
			}
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
