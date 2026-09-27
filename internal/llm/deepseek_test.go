package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeepSeekClientMapsRequestAndResponse(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAuthorization string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{
			"choices":[{
				"message":{
					"content":"thinking",
					"tool_calls":[{
						"id":"response_call",
						"type":"function",
						"function":{"name":"finish","arguments":"{\"items\":[\"summary\"]}"}
					}]
				},
				"finish_reason":"tool_calls"
			}]
		}`)
	}))
	defer server.Close()

	client := newTestDeepSeekClient(t, server.URL+"///", "secret-key")
	response, err := client.Chat(t.Context(), ChatRequest{
		Model: "request-model",
		Messages: []Message{
			{Role: RoleSystem, Content: "system prompt"},
			{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{{
					ID:            "request_call",
					Name:          "read_file",
					ArgumentsJSON: `{"path":"/tmp/input"}`,
				}},
			},
			{Role: RoleTool, Content: "observation", ToolCallID: "request_call"},
		},
		Tools: []ToolSpec{{
			Name:        "read_file",
			Description: "read the goal file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Errorf("request path = %q, want /chat/completions", gotPath)
	}
	if gotAuthorization != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want Bearer secret-key", gotAuthorization)
	}
	if gotBody["model"] != "request-model" ||
		gotBody["tool_choice"] != "auto" ||
		gotBody["stream"] != false {
		t.Errorf("request controls = %#v", gotBody)
	}
	messages := gotBody["messages"].([]any)
	requestCall := messages[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	requestFunction := requestCall["function"].(map[string]any)
	if requestFunction["name"] != "read_file" ||
		requestFunction["arguments"] != `{"path":"/tmp/input"}` {
		t.Errorf("request function = %#v", requestFunction)
	}
	if messages[2].(map[string]any)["tool_call_id"] != "request_call" {
		t.Errorf("tool message = %#v", messages[2])
	}
	function := gotBody["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "read_file" ||
		function["parameters"].(map[string]any)["type"] != "object" {
		t.Errorf("tool function = %#v", function)
	}

	wantCall := ToolCall{
		ID:            "response_call",
		Name:          "finish",
		ArgumentsJSON: `{"items":["summary"]}`,
	}
	if response.Content != "thinking" ||
		response.FinishReason != "tool_calls" ||
		len(response.ToolCalls) != 1 ||
		response.ToolCalls[0] != wantCall {
		t.Errorf("response = %#v", response)
	}
}

func TestDeepSeekClientUsesDefaultModelAndSixtySecondDeadline(t *testing.T) {
	t.Parallel()

	var gotModel string
	var remaining time.Duration
	client := newTestDeepSeekClient(t, "https://example.invalid", "key")
	client.httpClient = &http.Client{Transport: deepSeekRoundTripFunc(
		func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			if !ok {
				t.Error("request context has no deadline")
			} else {
				remaining = time.Until(deadline)
			}
			var body struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			gotModel = body.Model
			return deepSeekJSONResponse(
				http.StatusOK,
				`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`,
			), nil
		},
	)}

	if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if gotModel != "default-model" {
		t.Errorf("model = %q, want default-model", gotModel)
	}
	if remaining < 59*time.Second || remaining > 60*time.Second {
		t.Errorf("deadline remaining = %v, want about 60s", remaining)
	}
}

func TestDeepSeekClientRetriesThreeTimes(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "server error", status: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					if attempts.Add(1) < 3 {
						http.Error(w, "retry", tt.status)
						return
					}
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				},
			))
			defer server.Close()

			client := newTestDeepSeekClient(t, server.URL, "key")
			client.sleep = func(context.Context, time.Duration) error { return nil }
			if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
				t.Fatalf("Chat() error = %v", err)
			}
			if attempts.Load() != 3 {
				t.Errorf("attempts = %d, want 3", attempts.Load())
			}
		})
	}
}

func TestDeepSeekClientRetriesTemporaryNetworkErrors(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	client := newTestDeepSeekClient(t, "https://example.invalid", "key")
	client.httpClient = &http.Client{Transport: deepSeekRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			if attempts.Add(1) < 3 {
				return nil, deepSeekTemporaryError{}
			}
			return deepSeekJSONResponse(
				http.StatusOK,
				`{"choices":[{"message":{"content":"ok"}}]}`,
			), nil
		},
	)}
	client.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

func TestDeepSeekClientBackoffAndRetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		want       []time.Duration
	}{
		{
			name: "exponential defaults",
			want: []time.Duration{500 * time.Millisecond, time.Second},
		},
		{
			name:       "valid retry after",
			retryAfter: "2",
			want:       []time.Duration{2 * time.Second, 2 * time.Second},
		},
		{
			name:       "retry after over limit",
			retryAfter: "31",
			want:       []time.Duration{500 * time.Millisecond, time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					if tt.retryAfter != "" {
						w.Header().Set("Retry-After", tt.retryAfter)
					}
					http.Error(w, "retry", http.StatusTooManyRequests)
				},
			))
			defer server.Close()

			client := newTestDeepSeekClient(t, server.URL, "key")
			delays := []time.Duration{}
			client.sleep = func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil
			}
			if _, err := client.Chat(t.Context(), ChatRequest{}); err == nil {
				t.Fatal("Chat() error = nil, want error")
			}
			if len(delays) != len(tt.want) {
				t.Fatalf("delays = %v, want %v", delays, tt.want)
			}
			for i := range tt.want {
				if delays[i] != tt.want[i] {
					t.Errorf("delay %d = %v, want %v", i, delays[i], tt.want[i])
				}
			}
		})
	}
}

func TestDeepSeekClientDoesNotRetryPermanentFailures(t *testing.T) {
	tests := []struct {
		name      string
		transport http.RoundTripper
	}{
		{
			name: "ordinary client error",
			transport: deepSeekRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return deepSeekJSONResponse(http.StatusBadRequest, "bad request"), nil
			}),
		},
		{
			name: "permanent network error",
			transport: deepSeekRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("permanent failure")
			}),
		},
		{
			name: "invalid success response",
			transport: deepSeekRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return deepSeekJSONResponse(http.StatusOK, `{"choices":[]}`), nil
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			client := newTestDeepSeekClient(t, "https://example.invalid", "key")
			client.httpClient = &http.Client{Transport: deepSeekRoundTripFunc(
				func(r *http.Request) (*http.Response, error) {
					attempts.Add(1)
					return tt.transport.RoundTrip(r)
				},
			)}
			client.sleep = func(context.Context, time.Duration) error { return nil }

			if _, err := client.Chat(t.Context(), ChatRequest{}); err == nil {
				t.Fatal("Chat() error = nil, want error")
			}
			if attempts.Load() != 1 {
				t.Errorf("attempts = %d, want 1", attempts.Load())
			}
		})
	}
}

func TestDeepSeekClientLimitsAndRedactsErrors(t *testing.T) {
	t.Parallel()

	const apiKey = "network-secret"
	client := newTestDeepSeekClient(t, "https://example.invalid", apiKey)
	client.httpClient = &http.Client{Transport: deepSeekRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			body := strings.Repeat(apiKey+"x", 200)
			return deepSeekJSONResponse(http.StatusBadRequest, body), nil
		},
	)}

	_, err := client.Chat(t.Context(), ChatRequest{})
	if err == nil {
		t.Fatal("Chat() error = nil, want error")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Errorf("error leaked API key: %q", err)
	}
	if len(err.Error()) > 1150 {
		t.Errorf("len(error) = %d, response summary exceeds 1KiB", len(err.Error()))
	}

	client.httpClient = &http.Client{Transport: deepSeekRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection failed with " + apiKey)
		},
	)}
	_, err = client.Chat(t.Context(), ChatRequest{})
	if err == nil || strings.Contains(err.Error(), apiKey) {
		t.Errorf("network error = %v, want redacted error", err)
	}
}

func TestNewDeepSeekClientValidatesConfigAndImplementsClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		apiKey   string
		model    string
		wantText string
	}{
		{name: "missing base URL", apiKey: "key", model: "model", wantText: "base_url"},
		{name: "missing API key", baseURL: "https://example.com", model: "model", wantText: "api_key"},
		{name: "missing model", baseURL: "https://example.com", apiKey: "key", wantText: "model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newDeepSeekClient(tt.baseURL, tt.apiKey, tt.model)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error = %v, want containing %q", err, tt.wantText)
			}
		})
	}

	client, err := newDeepSeekClient("https://example.com", "key", "model")
	if err != nil {
		t.Fatalf("newDeepSeekClient() error = %v", err)
	}
	var _ Client = client
}

func newTestDeepSeekClient(t *testing.T, baseURL, apiKey string) *deepseekClient {
	t.Helper()

	client, err := newDeepSeekClient(baseURL, apiKey, "default-model")
	if err != nil {
		t.Fatalf("newDeepSeekClient() error = %v", err)
	}
	return client
}

type deepSeekRoundTripFunc func(*http.Request) (*http.Response, error)

func (f deepSeekRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type deepSeekTemporaryError struct{}

func (deepSeekTemporaryError) Error() string   { return "temporary failure" }
func (deepSeekTemporaryError) Timeout() bool   { return false }
func (deepSeekTemporaryError) Temporary() bool { return true }

func deepSeekJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
