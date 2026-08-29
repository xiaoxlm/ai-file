package openaicompat

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

func TestClientChatMapsRequestAndResponse(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAuthorization string
	var gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices": [{
				"message": {
					"content": "thinking",
					"tool_calls": [{
						"id": "call_response",
						"type": "function",
						"function": {
							"name": "finish",
							"arguments": "{\"items\":[\"summary\"]}"
						}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`)
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL:      server.URL + "///",
		APIKey:       "secret-key",
		DefaultModel: "default-model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response, err := client.Chat(t.Context(), ChatRequest{
		Model: "request-model",
		Messages: []Message{
			{Role: "system", Content: "system prompt"},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{{
					ID:            "call_request",
					Name:          "read_file",
					ArgumentsJSON: `{"path":"/tmp/input"}`,
				}},
			},
			{Role: "tool", Content: "observation", ToolCallID: "call_request"},
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
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["model"] != "request-model" {
		t.Errorf("model = %v, want request-model", gotBody["model"])
	}
	if gotBody["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, want auto", gotBody["tool_choice"])
	}
	if gotBody["stream"] != false {
		t.Errorf("stream = %v, want false", gotBody["stream"])
	}

	messages := gotBody["messages"].([]any)
	assistant := messages[1].(map[string]any)
	requestCalls := assistant["tool_calls"].([]any)
	requestFunction := requestCalls[0].(map[string]any)["function"].(map[string]any)
	if requestFunction["name"] != "read_file" {
		t.Errorf("request tool name = %v, want read_file", requestFunction["name"])
	}
	if requestFunction["arguments"] != `{"path":"/tmp/input"}` {
		t.Errorf("request tool arguments = %v", requestFunction["arguments"])
	}
	toolMessage := messages[2].(map[string]any)
	if toolMessage["tool_call_id"] != "call_request" {
		t.Errorf("tool_call_id = %v, want call_request", toolMessage["tool_call_id"])
	}

	tools := gotBody["tools"].([]any)
	function := tools[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "read_file" {
		t.Errorf("tool name = %v, want read_file", function["name"])
	}
	parameters := function["parameters"].(map[string]any)
	if parameters["type"] != "object" {
		t.Errorf("tool parameters type = %v, want object", parameters["type"])
	}

	if response.Content != "thinking" {
		t.Errorf("Content = %q, want thinking", response.Content)
	}
	if response.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", response.FinishReason)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(response.ToolCalls))
	}
	if response.ToolCalls[0] != (ToolCall{
		ID:            "call_response",
		Name:          "finish",
		ArgumentsJSON: `{"items":["summary"]}`,
	}) {
		t.Errorf("ToolCalls[0] = %#v", response.ToolCalls[0])
	}
}

func TestClientChatUsesDefaultModelAndSixtySecondDeadline(t *testing.T) {
	t.Parallel()

	var gotModel string
	var remaining time.Duration
	client, err := New(Config{
		BaseURL:      "https://example.invalid",
		APIKey:       "key",
		DefaultModel: "default-model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		return jsonResponse(
			http.StatusOK,
			`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`,
		), nil
	})}
	if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if gotModel != "default-model" {
		t.Errorf("model = %q, want default-model", gotModel)
	}
	if remaining < 59*time.Second || remaining > 60*time.Second {
		t.Errorf("request deadline remaining = %v, want about 60s", remaining)
	}
}

func TestClientChatRetriesRetryableStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "internal server error", status: http.StatusInternalServerError},
		{name: "service unavailable", status: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if attempts.Add(1) < 3 {
					http.Error(w, "retry", tt.status)
					return
				}
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, "key")
			client.sleep = func(context.Context, time.Duration) error { return nil }
			if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
				t.Fatalf("Chat() error = %v", err)
			}
			if got := attempts.Load(); got != 3 {
				t.Errorf("attempts = %d, want 3", got)
			}
		})
	}
}

func TestClientChatStopsAfterThreeAttemptsWithExponentialBackoff(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "key")
	delays := make([]time.Duration, 0, 2)
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	if _, err := client.Chat(t.Context(), ChatRequest{}); err == nil {
		t.Fatal("Chat() error = nil, want error")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	expectedDelays := []time.Duration{500 * time.Millisecond, time.Second}
	if len(delays) != len(expectedDelays) {
		t.Fatalf("retry delays = %v, want %v", delays, expectedDelays)
	}
	for i := range expectedDelays {
		if delays[i] != expectedDelays[i] {
			t.Errorf("retry delay %d = %v, want %v", i, delays[i], expectedDelays[i])
		}
	}
}

func TestClientChatRetriesTemporaryNetworkErrors(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	client := newTestClient(t, "https://example.invalid", "key")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if attempts.Add(1) < 3 {
			return nil, temporaryError{}
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})}
	client.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestClientChatDoesNotRetryOrdinaryClientOrPermanentNetworkErrors(t *testing.T) {
	tests := []struct {
		name      string
		transport http.RoundTripper
	}{
		{
			name: "bad request",
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusBadRequest, "bad request"), nil
			}),
		},
		{
			name: "permanent network error",
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("permanent failure")
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			client := newTestClient(t, "https://example.invalid", "key")
			client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts.Add(1)
				return tt.transport.RoundTrip(r)
			})}
			client.sleep = func(context.Context, time.Duration) error { return nil }

			if _, err := client.Chat(t.Context(), ChatRequest{}); err == nil {
				t.Fatal("Chat() error = nil, want error")
			}
			if got := attempts.Load(); got != 1 {
				t.Errorf("attempts = %d, want 1", got)
			}
		})
	}
}

func TestClientChatUsesRetryAfterUpToThirtySeconds(t *testing.T) {
	tests := []struct {
		name          string
		retryAfter    string
		expectedDelay time.Duration
	}{
		{name: "valid retry after", retryAfter: "2", expectedDelay: 2 * time.Second},
		{name: "retry after over limit", retryAfter: "31", expectedDelay: 500 * time.Millisecond},
		{name: "invalid retry after", retryAfter: "invalid", expectedDelay: 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if attempts.Add(1) == 1 {
					w.Header().Set("Retry-After", tt.retryAfter)
					http.Error(w, "retry", http.StatusTooManyRequests)
					return
				}
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, "key")
			delays := make([]time.Duration, 0, 1)
			client.sleep = func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil
			}

			if _, err := client.Chat(t.Context(), ChatRequest{}); err != nil {
				t.Fatalf("Chat() error = %v", err)
			}
			if len(delays) != 1 || delays[0] != tt.expectedDelay {
				t.Errorf("retry delays = %v, want [%v]", delays, tt.expectedDelay)
			}
		})
	}
}

func TestClientChatLimitsAndRedactsErrorResponse(t *testing.T) {
	t.Parallel()

	const apiKey = "z"
	responseBody := strings.Repeat(apiKey, 256)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, responseBody, http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, apiKey)
	_, err := client.Chat(t.Context(), ChatRequest{})
	if err == nil {
		t.Fatal("Chat() error = nil, want error")
	}
	errorText := err.Error()
	if !strings.Contains(errorText, "status 400") {
		t.Errorf("error = %q, want status 400", errorText)
	}
	if strings.Contains(errorText, apiKey) {
		t.Errorf("error leaked API key: %q", errorText)
	}
	if len(errorText) > 1150 {
		t.Errorf("len(error) = %d, response summary exceeds 1KiB", len(errorText))
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestClientChatRedactsAPIKeyFromNetworkErrors(t *testing.T) {
	t.Parallel()

	const apiKey = "network-secret"
	client := newTestClient(t, "https://example.invalid", apiKey)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection failed with " + apiKey)
	})}

	_, err := client.Chat(t.Context(), ChatRequest{})
	if err == nil {
		t.Fatal("Chat() error = nil, want error")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Errorf("error leaked API key: %q", err)
	}
}

func TestClientChatRejectsInvalidSuccessfulResponsesWithoutRetry(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{`},
		{name: "missing choices", body: `{"choices":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, "key")
			client.sleep = func(context.Context, time.Duration) error { return nil }
			if _, err := client.Chat(t.Context(), ChatRequest{}); err == nil {
				t.Fatal("Chat() error = nil, want error")
			}
			if got := attempts.Load(); got != 1 {
				t.Errorf("attempts = %d, want 1", got)
			}
		})
	}
}

func newTestClient(t *testing.T, baseURL, apiKey string) *Client {
	t.Helper()

	client, err := New(Config{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		DefaultModel: "model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary failure" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
