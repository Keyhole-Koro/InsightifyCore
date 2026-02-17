package llmclient

import "testing"

type collectRegistrar struct {
	specs []ModelRegistration
}

func (c *collectRegistrar) RegisterModel(spec ModelRegistration) error {
	c.specs = append(c.specs, spec)
	return nil
}

func TestRegisterGroqModelsForTier_AnnotatesTier(t *testing.T) {
	reg := &collectRegistrar{}
	if err := RegisterGroqModelsForTier(reg, "developer"); err != nil {
		t.Fatalf("register groq developer: %v", err)
	}
	if len(reg.specs) == 0 {
		t.Fatalf("expected registered models")
	}
	for _, spec := range reg.specs {
		if spec.Provider != "groq" {
			t.Fatalf("provider: got=%s", spec.Provider)
		}
		if spec.Tier != "developer" {
			t.Fatalf("tier: got=%s want=developer", spec.Tier)
		}
	}
}

func TestRegisterGeminiModelsForTier_AnnotatesTier(t *testing.T) {
	reg := &collectRegistrar{}
	if err := RegisterGeminiModelsForTier(reg, "tier1"); err != nil {
		t.Fatalf("register gemini tier1: %v", err)
	}
	if len(reg.specs) == 0 {
		t.Fatalf("expected registered models")
	}
	for _, spec := range reg.specs {
		if spec.Provider != "gemini" {
			t.Fatalf("provider: got=%s", spec.Provider)
		}
		if spec.Tier != "tier1" {
			t.Fatalf("tier: got=%s want=tier1", spec.Tier)
		}
	}
}
