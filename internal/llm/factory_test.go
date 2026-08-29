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

func TestNewAppliesProviderPresetsAndUsesOneAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cfg             config.Config
		expectedBaseURL string
		expectedModel   string
	}{
		{
			name: "deepseek defaults",
			cfg: config.Config{
				Provider: config.ProviderDeepSeek,
				APIKey:   "key",
			},
			expectedBaseURL: config.DefaultDeepSeekBaseURL,
			expectedModel:   config.DefaultDeepSeekModel,
		},
		{
			name: "openai default base url",
			cfg: config.Config{
				Provider: config.ProviderOpenAI,
				APIKey:   "key",
				Model:    "gpt-test",
			},
			expectedBaseURL: config.DefaultOpenAIBaseURL,
			expectedModel:   "gpt-test",
		},
		{
			name: "custom settings",
			cfg: config.Config{
				Provider: config.ProviderCustom,
				APIKey:   "key",
				BaseURL:  "https://compatible.example/v1",
				Model:    "custom-model",
			},
			expectedBaseURL: "https://compatible.example/v1",
			expectedModel:   "custom-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := New(tt.cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			adapter, ok := client.(*openAICompatibleClient)
			if !ok {
				t.Fatalf("New() type = %T, want *openAICompatibleClient", client)
			}
			if adapter.baseURL != tt.expectedBaseURL {
				t.Errorf("baseURL = %q, want %q", adapter.baseURL, tt.expectedBaseURL)
			}
			if adapter.defaultModel != tt.expectedModel {
				t.Errorf("defaultModel = %q, want %q", adapter.defaultModel, tt.expectedModel)
			}
		})
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
			name:      "missing api key",
			cfg:       config.Config{Provider: config.ProviderDeepSeek},
			errorText: "api_key is required",
		},
		{
			name: "openai missing model",
			cfg: config.Config{
				Provider: config.ProviderOpenAI,
				APIKey:   "key",
			},
			errorText: "model is required",
		},
		{
			name: "custom missing base url",
			cfg: config.Config{
				Provider: config.ProviderCustom,
				APIKey:   "key",
				Model:    "model",
			},
			errorText: "base_url is required",
		},
		{
			name: "custom missing model",
			cfg: config.Config{
				Provider: config.ProviderCustom,
				APIKey:   "key",
				BaseURL:  "https://example.invalid",
			},
			errorText: "model is required",
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

func TestNewCustomMapsLLMDTOs(t *testing.T) {
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
		Provider: config.ProviderCustom,
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

func TestOpenAICompatibleClientSatisfiesClient(t *testing.T) {
	t.Parallel()

	var _ Client = (*openAICompatibleClient)(nil)
	var _ interface {
		Chat(context.Context, ChatRequest) (ChatResponse, error)
	} = (*openAICompatibleClient)(nil)
}
