package runner

import "testing"

func TestTestRegistryIncludesTestLLMChatWorker(t *testing.T) {
	env := &Env{}
	reg := BuildRegistryTest(env)

	spec, ok := reg["testllmChar"]
	if !ok {
		t.Fatalf("expected testllmChar worker spec in test registry")
	}
	if spec.Key != "testllmChar" {
		t.Fatalf("unexpected testllmChar key: %q", spec.Key)
	}
}
