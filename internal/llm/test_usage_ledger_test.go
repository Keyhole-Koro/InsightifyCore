package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	llmclient "insightify/internal/llmClient"
)

type usageMockClient struct {
	name      string
	tokenCap  int
	failCalls int
}

func (m *usageMockClient) Name() string { return m.name }
func (m *usageMockClient) Close() error { return nil }
func (m *usageMockClient) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 2
}
func (m *usageMockClient) TokenCapacity() int { return m.tokenCap }
func (m *usageMockClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	_ = ctx
	_ = prompt
	_ = input
	if m.failCalls > 0 {
		m.failCalls--
		return nil, fmt.Errorf("mock failure")
	}
	return json.RawMessage(`{"ok":true}`), nil
}
func (m *usageMockClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	raw, err := m.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return nil, err
	}
	if onChunk != nil {
		onChunk(string(raw))
	}
	return raw, nil
}

var _ llmclient.LLMClient = (*usageMockClient)(nil)

func TestUsageLedgerDaily_AggregatesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llm_usage_daily.json")

	base := &usageMockClient{name: "mock:model-a", tokenCap: 1024}
	cli := Wrap(base, WithUsageLedger(path))

	ctx := withSelectedModel(context.Background(), selectedModel{client: base})
	if _, err := cli.GenerateJSON(ctx, "prompt", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("generate success call: %v", err)
	}

	base.failCalls = 1
	if _, err := cli.GenerateJSON(ctx, "prompt", map[string]any{"k": "v2"}); err == nil {
		t.Fatalf("expected error call")
	}

	// New wrapped instance should append to same JSON file.
	base2 := &usageMockClient{name: "mock:model-b", tokenCap: 1024}
	cli2 := Wrap(base2, WithUsageLedger(path))
	ctx2 := withSelectedModel(context.Background(), selectedModel{client: base2})
	if _, err := cli2.GenerateJSON(ctx2, "prompt", map[string]any{"z": 1}); err != nil {
		t.Fatalf("generate second model: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger file: %v", err)
	}
	var got usageLedgerFile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal ledger: %v", err)
	}

	dayKey := time.Now().UTC().Format("2006-01-02")
	day, ok := got.Days[dayKey]
	if !ok {
		t.Fatalf("missing day bucket %s", dayKey)
	}
	if day.Requests != 3 {
		t.Fatalf("requests: got=%d want=3", day.Requests)
	}
	if day.Errors != 1 {
		t.Fatalf("errors: got=%d want=1", day.Errors)
	}
	if day.Tokens <= 0 {
		t.Fatalf("tokens should be > 0")
	}

	modelA := day.Models["mock:model-a"]
	if modelA.Requests != 2 || modelA.Errors != 1 {
		t.Fatalf("model-a stats: %+v", modelA)
	}
	modelB := day.Models["mock:model-b"]
	if modelB.Requests != 1 || modelB.Errors != 0 {
		t.Fatalf("model-b stats: %+v", modelB)
	}
}
