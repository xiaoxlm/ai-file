package tools

import (
	"context"
	"fmt"

	"github.com/xiaoxlm/ai-file/internal/llm"
	"github.com/xiaoxlm/ai-file/internal/memory"
)

// Registry holds registered tools and structured completion state.
type Registry struct {
	tools      []Tool
	completion []string
	hasDone    bool
}

// NewRegistry returns an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterCoreTools registers read_file and finish for a single run.
func (r *Registry) RegisterCoreTools(mem memory.Memory, maxParaChars int) error {
	if err := r.Register(NewReadFile(mem, maxParaChars)); err != nil {
		return err
	}
	return r.Register(NewFinish(r, mem))
}

// Register adds a tool. Duplicate names return an error.
func (r *Registry) Register(tool Tool) error {
	for _, existing := range r.tools {
		if existing.Name() == tool.Name() {
			return fmt.Errorf("tool %q already registered", tool.Name())
		}
	}
	r.tools = append(r.tools, tool)
	return nil
}

// List returns tool specifications in registration order.
func (r *Registry) List() []llm.ToolSpec {
	if len(r.tools) == 0 {
		return nil
	}
	specs := make([]llm.ToolSpec, len(r.tools))
	for i, tool := range r.tools {
		specs[i] = llm.ToolSpec{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		}
	}
	return specs
}

// Execute runs a registered tool by name.
func (r *Registry) Execute(
	ctx context.Context,
	name string,
	argumentsJSON string,
) (string, error) {
	for _, tool := range r.tools {
		if tool.Name() == name {
			return tool.Execute(ctx, argumentsJSON)
		}
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

// Completion returns structured completion items when available.
func (r *Registry) Completion() ([]string, bool) {
	if !r.hasDone {
		return nil, false
	}
	return append([]string(nil), r.completion...), true
}

func (r *Registry) setCompletion(items []string) {
	r.completion = append([]string(nil), items...)
	r.hasDone = true
}
