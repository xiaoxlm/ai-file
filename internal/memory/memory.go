package memory

import "github.com/xiaoxlm/ai-file/internal/llm"

// Memory stores conversation messages and key-value session state.
type Memory interface {
	Append(message llm.Message) error
	Messages() []llm.Message
	Set(key, value string)
	Get(key string) (string, bool)
	Clear()
}
