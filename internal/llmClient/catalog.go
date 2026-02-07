package llmclient

import "context"

type ModelLevel string

const (
	ModelLevelLow    ModelLevel = "low"
	ModelLevelMiddle ModelLevel = "middle"
	ModelLevelHigh   ModelLevel = "high"
	ModelLevelXHigh  ModelLevel = "xhigh"
)

type ClientFactory func(ctx context.Context, tokenCap int) (LLMClient, error)

type ModelRegistration struct {
	Provider       string
	Model          string
	Level          ModelLevel
	MaxTokens      int
	ParameterCount int64
	Factory        ClientFactory
}

type ModelRegistrar interface {
	RegisterModel(spec ModelRegistration) error
}
