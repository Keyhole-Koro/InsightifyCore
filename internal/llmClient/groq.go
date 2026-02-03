package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// GroqClient calls the Groq Chat Completions API (OpenAI-compatible) and asks for JSON.
// See: https://console.groq.com/docs/api-reference
type GroqClient struct {
	http     *http.Client
	apiKey   string
	model    string
	baseURL  string
	tokenCap int
}

// NewGroqClient creates a Groq client. If apiKey is empty, it falls back to GROQ_API_KEY env var.
func NewGroqClient(apiKey, model string, tokenCap int) (*GroqClient, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GROQ_API_KEY")
	}
	if tokenCap <= 0 {
		tokenCap = 6000
	}
	return &GroqClient{
		http:     &http.Client{Timeout: 60 * time.Second},
		apiKey:   apiKey,
		model:    model,
		baseURL:  "https://api.groq.com/openai/v1/chat/completions",
		tokenCap: tokenCap,
	}, nil
}

func (g *GroqClient) Name() string { return "Groq:" + g.model }
func (g *GroqClient) Close() error { return nil }
func (g *GroqClient) CountTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return CountTokens(text)
}
func (g *GroqClient) TokenCapacity() int { return g.tokenCap }

type groqChatReq struct {
	Model          string            `json:"model"`
	Messages       []groqMessage     `json:"messages"`
	Temperature    float32           `json:"temperature,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}
type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type groqChatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// GenerateJSON assembles a single user message from prompt + input and requests JSON output.
func (g *GroqClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	userContent := "[INPUT JSON]\n" + string(in)

	reqBody := groqChatReq{
		Model: g.model,
		Messages: []groqMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: userContent},
		},
		Temperature:    0,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		const max = 2048
		if len(body) > max {
			body = body[:max]
		}
		err := fmt.Errorf("groq: unexpected status %s: %s", resp.Status, string(body))
		// Check for context length exceeded (permanent error)
		if resp.StatusCode == 400 && strings.Contains(string(body), `"code":"context_length_exceeded"`) {
			return nil, NewPermanentError(err)
		}
		return nil, err
	}
	var out groqChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return nil, ErrInvalidJSON
	}
	// Ensure the content is valid JSON; if not, wrap with an error
	raw := json.RawMessage(out.Choices[0].Message.Content)
	var scratch any
	if err := json.Unmarshal(raw, &scratch); err != nil {
		return nil, ErrInvalidJSON
	}
	return raw, nil
}

func (g *GroqClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	raw, err := g.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return nil, err
	}
	if onChunk != nil {
		onChunk(string(raw))
	}
	return raw, nil
}
