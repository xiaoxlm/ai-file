package memory

import (
	"github.com/xiaoxlm/ai-file/internal/llm"
)

const maxToolObservationBytes = 32 * 1024

const observationTruncatedSuffix = "\n[observation truncated]"

// InMemory stores messages and key-value session state for a single run.
type InMemory struct {
	messages []llm.Message
	kv       map[string]string
}

// NewInMemory returns an empty in-memory store.
func NewInMemory() *InMemory {
	return &InMemory{
		kv: make(map[string]string),
	}
}

// Append adds a message to the conversation history.
func (m *InMemory) Append(message llm.Message) error {
	if message.Role == llm.RoleTool {
		message.Content = limitToolObservation(message.Content)
	}
	m.messages = append(m.messages, message)
	return nil
}

// Messages returns a deep copy of stored messages.
func (m *InMemory) Messages() []llm.Message {
	if len(m.messages) == 0 {
		return nil
	}
	out := make([]llm.Message, len(m.messages))
	for i, msg := range m.messages {
		out[i] = cloneMessage(msg)
	}
	return out
}

// Set stores a key-value pair.
func (m *InMemory) Set(key, value string) {
	m.kv[key] = value
}

// Get returns a stored value.
func (m *InMemory) Get(key string) (string, bool) {
	value, ok := m.kv[key]
	return value, ok
}

// Clear removes all messages and key-value state.
func (m *InMemory) Clear() {
	m.messages = nil
	m.kv = make(map[string]string)
}

func limitToolObservation(content string) string {
	if len(content) <= maxToolObservationBytes {
		return content
	}
	return content[:maxToolObservationBytes] + observationTruncatedSuffix
}

func cloneMessage(msg llm.Message) llm.Message {
	out := msg
	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = append([]llm.ToolCall(nil), msg.ToolCalls...)
	}
	return out
}
