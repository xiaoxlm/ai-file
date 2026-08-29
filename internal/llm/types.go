package llm

import "encoding/json"

// Role identifies a chat message author.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is the shared chat message contract for Memory and LLM clients.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall describes a model-requested tool invocation.
type ToolCall struct {
	ID            string
	Name          string
	ArgumentsJSON string
}

// ToolSpec describes a tool exposed to the model.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ChatRequest is a vendor-neutral chat completion request.
type ChatRequest struct {
	Messages []Message
	Tools    []ToolSpec
	Model    string
}

// ChatResponse is a vendor-neutral chat completion response.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}
