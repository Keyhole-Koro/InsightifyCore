package llm

import (
	llmclient "insightify/internal/llmClient"
)

// Middleware decorates an LLMClient to inject cross-cutting concerns
// (rate limiting, retries, logging, hooks, etc.).
type Middleware func(llmclient.LLMClient) llmclient.LLMClient

// Wrap applies middlewares in left-to-right order.
// Example: Wrap(inner, A, B) => A(B(inner))
func Wrap(inner llmclient.LLMClient, mws ...Middleware) llmclient.LLMClient {
	out := inner
	for i := len(mws) - 1; i >= 0; i-- {
		out = mws[i](out)
	}
	return out
}

// Chain is an alias for Wrap for convenience.
func Chain(inner llmclient.LLMClient, mws ...Middleware) llmclient.LLMClient {
	return Wrap(inner, mws...)
}
