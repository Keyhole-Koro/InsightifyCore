package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	llmclient "insightify/internal/llm/client"
)

// UsageLedger tracks LLM usage statistics to a JSON file.
type UsageLedger struct {
	mu   sync.Mutex
	path string
}

type usageLedgerFile struct {
	UpdatedAt string              `json:"updated_at"`
	Days      map[string]usageDay `json:"days"`
}

type usageDay struct {
	Requests int64                `json:"requests"`
	Tokens   int64                `json:"tokens"`
	Errors   int64                `json:"errors"`
	Models   map[string]usageStat `json:"models"`
}

type usageStat struct {
	Requests int64 `json:"requests"`
	Tokens   int64 `json:"tokens"`
	Errors   int64 `json:"errors"`
}

// NewUsageLedger creates a new usage ledger that writes to path.
func NewUsageLedger(path string) *UsageLedger {
	return &UsageLedger{path: path}
}

// WithUsageLedger returns a middleware that tracks usage to the given path.
func WithUsageLedger(path string) Middleware {
	ledger := NewUsageLedger(path)
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &usageLedgerClient{next: next, ledger: ledger}
	}
}

type usageLedgerClient struct {
	next   llmclient.LLMClient
	ledger *UsageLedger
}

func (u *usageLedgerClient) Name() string { return u.next.Name() }
func (u *usageLedgerClient) Close() error { return u.next.Close() }
func (u *usageLedgerClient) CountTokens(text string) int {
	return u.next.CountTokens(text)
}
func (u *usageLedgerClient) TokenCapacity() int { return u.next.TokenCapacity() }

func (u *usageLedgerClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	tokens := estimateCallTokens(u.next, prompt, input)
	out, err := u.next.GenerateJSON(ctx, prompt, input)
	u.writeUsage(ctx, tokens, err)
	return out, err
}

func (u *usageLedgerClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	tokens := estimateCallTokens(u.next, prompt, input)
	out, err := u.next.GenerateJSONStream(ctx, prompt, input, onChunk)
	u.writeUsage(ctx, tokens, err)
	return out, err
}

func estimateCallTokens(cli llmclient.LLMClient, prompt string, input any) int {
	in, _ := json.Marshal(input)
	payload := prompt + "\n" + string(in)
	t := cli.CountTokens(payload)
	if t < 1 {
		t = 1
	}
	return t
}

func (u *usageLedgerClient) writeUsage(ctx context.Context, tokens int, err error) {
	if u.ledger == nil || u.ledger.path == "" {
		return
	}
	modelKey := "unknown"
	if selected, ok := SelectedClientFrom(ctx); ok {
		modelKey = selected.Name()
	}
	u.ledger.record(modelKey, int64(tokens), err != nil)
}

func (l *UsageLedger) record(model string, tokens int64, hasErr bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	dayKey := time.Now().UTC().Format("2006-01-02")
	f := usageLedgerFile{Days: map[string]usageDay{}}
	if b, err := os.ReadFile(l.path); err == nil {
		_ = json.Unmarshal(b, &f)
		if f.Days == nil {
			f.Days = map[string]usageDay{}
		}
	}

	d := f.Days[dayKey]
	if d.Models == nil {
		d.Models = map[string]usageStat{}
	}
	d.Requests++
	d.Tokens += tokens
	if hasErr {
		d.Errors++
	}
	m := d.Models[model]
	m.Requests++
	m.Tokens += tokens
	if hasErr {
		m.Errors++
	}
	d.Models[model] = m
	f.Days[dayKey] = d
	f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return
	}
	tmp := l.path + ".tmp"
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, l.path)
}
