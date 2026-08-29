package llm

import "context"

// Client performs vendor-neutral chat completions with optional tool calling.
type Client interface {
	Chat(ctx context.Context, request ChatRequest) (ChatResponse, error)
}
