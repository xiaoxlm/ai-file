package tools

import (
	"context"
	"encoding/json"

	"github.com/xiaoxlm/ai-file/internal/llm"
)

// Tool executes a named capability exposed to the model.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, argumentsJSON string) (string, error)
}

// Executor lists tools, executes them, and exposes structured completion.
type Executor interface {
	List() []llm.ToolSpec
	Execute(ctx context.Context, name, argumentsJSON string) (string, error)
	Completion() (items []string, ok bool)
}
