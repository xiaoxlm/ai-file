package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/config"
)

func TestNewDeepSeekDefaults(t *testing.T) {
	t.Parallel()

	client, err := New(config.Config{
		Provider: config.ProviderDeepSeek,
		APIKey:   "key",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := client.(*deepseekClient); !ok {
		t.Fatalf("New() type = %T, want *deepseekClient", client)
	}
}

func TestNewDeepSeekExplicitOverrides(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	client, err := New(config.Config{
		Provider: config.ProviderDeepSeek,
		APIKey:   "key",
		BaseURL:  server.URL,
		Model:    "override-model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response, err := client.Chat(t.Context(), ChatRequest{})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Content != "ok" {
		t.Errorf("Content = %q, want ok", response.Content)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       config.Config
		errorText string
	}{
		{
			name:      "unknown provider",
			cfg:       config.Config{Provider: "other", APIKey: "key"},
			errorText: "unknown provider",
		},
		{
			name: "openai provider",
			cfg: config.Config{
				Provider: "openai",
				APIKey:   "key",
				Model:    "gpt-test",
			},
			errorText: "supported: deepseek",
		},
		{
			name:      "missing api key",
			cfg:       config.Config{Provider: config.ProviderDeepSeek},
			errorText: "api_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tt.cfg)
			if err == nil {
				t.Fatal("New() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.errorText) {
				t.Errorf("New() error = %q, want containing %q", err, tt.errorText)
			}
		})
	}
}

func TestNewDeepSeekChatMapsDTOs(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{
			"choices": [{
				"message": {
					"content": "done",
					"tool_calls": [{
						"id": "call_2",
						"type": "function",
						"function": {"name": "finish", "arguments": "{\"items\":[]}"}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`)
	}))
	defer server.Close()

	client, err := New(config.Config{
		Provider: config.ProviderDeepSeek,
		APIKey:   "key",
		BaseURL:  server.URL,
		Model:    "configured-model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response, err := client.Chat(t.Context(), ChatRequest{
		Messages: []Message{{
			Role:    RoleAssistant,
			Content: "thought",
			ToolCalls: []ToolCall{{
				ID:            "call_1",
				Name:          "read_file",
				ArgumentsJSON: `{"path":"file"}`,
			}},
		}},
		Tools: []ToolSpec{{
			Name:        "read_file",
			Description: "read",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if gotBody["model"] != "configured-model" {
		t.Errorf("model = %v, want configured-model", gotBody["model"])
	}
	if response.Content != "done" || response.FinishReason != "tool_calls" {
		t.Errorf("response = %#v", response)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "finish" {
		t.Errorf("ToolCalls = %#v", response.ToolCalls)
	}
}

func TestDeepSeekClientSatisfiesClient(t *testing.T) {
	t.Parallel()

	var _ Client = (*deepseekClient)(nil)
	var _ interface {
		Chat(context.Context, ChatRequest) (ChatResponse, error)
	} = (*deepseekClient)(nil)
}
