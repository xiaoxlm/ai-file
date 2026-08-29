// Package openaicompat implements the OpenAI-compatible Chat Completions wire protocol.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	chatTimeout     = 60 * time.Second
	maxAttempts     = 3
	maxErrorBody    = 1024
	maxRetryAfter   = 30 * time.Second
	redactedValue   = "[redacted]"
	toolType        = "function"
	defaultToolMode = "auto"
)

var retryBackoffs = [...]time.Duration{
	500 * time.Millisecond,
	time.Second,
}

// Config configures an OpenAI-compatible client.
type Config struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
}

// Message is a chat message in the adapter's transport-neutral input.
type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall describes a function call sent to or returned by the API.
type ToolCall struct {
	ID            string
	Name          string
	ArgumentsJSON string
}

// ToolSpec describes a function tool exposed to the model.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ChatRequest contains one Chat Completions request.
type ChatRequest struct {
	Messages []Message
	Tools    []ToolSpec
	Model    string
}

// ChatResponse contains the first choice returned by Chat Completions.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// Client calls an OpenAI-compatible Chat Completions endpoint.
type Client struct {
	endpoint     string
	apiKey       string
	defaultModel string
	httpClient   *http.Client
	sleep        func(context.Context, time.Duration) error
}

// New creates an OpenAI-compatible client.
func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("api_key is required")
	}
	if strings.TrimSpace(cfg.DefaultModel) == "" {
		return nil, errors.New("default model is required")
	}

	return &Client{
		endpoint:     baseURL + "/chat/completions",
		apiKey:       cfg.APIKey,
		defaultModel: cfg.DefaultModel,
		httpClient:   http.DefaultClient,
		sleep:        sleepWithContext,
	}, nil
}

// Chat performs one non-streaming Chat Completions call.
func (c *Client) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	body, err := c.marshalRequest(request)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode chat request: %w", err)
	}

	for attempt := range maxAttempts {
		response, requestErr := c.do(ctx, body)
		if requestErr != nil {
			if attempt == maxAttempts-1 || !isTemporary(requestErr) {
				return ChatResponse{}, fmt.Errorf(
					"send chat request: %s",
					c.redact(requestErr.Error()),
				)
			}
			if err := c.sleep(ctx, retryBackoffs[attempt]); err != nil {
				return ChatResponse{}, fmt.Errorf("wait to retry chat request: %w", err)
			}
			continue
		}

		result, shouldRetry, delay, responseErr := c.handleResponse(response, attempt)
		if responseErr == nil {
			return result, nil
		}
		if !shouldRetry {
			return ChatResponse{}, errors.New(c.redact(responseErr.Error()))
		}
		if err := c.sleep(ctx, delay); err != nil {
			return ChatResponse{}, fmt.Errorf("wait to retry chat request: %w", err)
		}
	}

	return ChatResponse{}, errors.New("chat request failed")
}

func (c *Client) marshalRequest(request ChatRequest) ([]byte, error) {
	model := request.Model
	if model == "" {
		model = c.defaultModel
	}

	messages := make([]wireMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		toolCalls := make([]wireToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			toolCalls = append(toolCalls, toWireToolCall(call))
		}
		messages = append(messages, wireMessage{
			Role:       message.Role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  toolCalls,
		})
	}

	tools := make([]wireTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, wireTool{
			Type: toolType,
			Function: wireFunctionSpec{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}

	return json.Marshal(wireRequest{
		Model:      model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: defaultToolMode,
		Stream:     false,
	})
}

func (c *Client) do(ctx context.Context, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(request)
}

func (c *Client) handleResponse(
	response *http.Response,
	attempt int,
) (ChatResponse, bool, time.Duration, error) {
	defer response.Body.Close()

	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result, err := decodeResponse(response.Body)
		return result, false, 0, err
	}

	summary, err := c.readErrorSummary(response.Body)
	if err != nil {
		return ChatResponse{}, false, 0, fmt.Errorf(
			"read chat response with status %d: %w",
			response.StatusCode,
			err,
		)
	}
	responseErr := fmt.Errorf(
		"chat response status %d: %s",
		response.StatusCode,
		summary,
	)

	isRetryable := response.StatusCode == http.StatusTooManyRequests ||
		response.StatusCode >= http.StatusInternalServerError
	if !isRetryable || attempt == maxAttempts-1 {
		return ChatResponse{}, false, 0, responseErr
	}

	delay := retryBackoffs[attempt]
	if retryAfter, ok := parseRetryAfter(response.Header.Get("Retry-After")); ok {
		delay = retryAfter
	}
	return ChatResponse{}, true, delay, responseErr
}

func (c *Client) readErrorSummary(reader io.Reader) (string, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxErrorBody))
	if err != nil {
		return "", err
	}

	summary := strings.TrimSpace(string(body))
	summary = c.redact(summary)
	if len(summary) > maxErrorBody {
		summary = summary[:maxErrorBody]
	}
	if summary == "" {
		return http.StatusText(http.StatusInternalServerError), nil
	}
	return summary, nil
}

func (c *Client) redact(text string) string {
	if c.apiKey == "" {
		return text
	}
	return strings.ReplaceAll(text, c.apiKey, redactedValue)
}

func decodeResponse(reader io.Reader) (ChatResponse, error) {
	var response wireResponse
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return ChatResponse{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(response.Choices) == 0 {
		return ChatResponse{}, errors.New("decode chat response: no choices")
	}

	choice := response.Choices[0]
	toolCalls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			ID:            call.ID,
			Name:          call.Function.Name,
			ArgumentsJSON: call.Function.Arguments,
		})
	}

	return ChatResponse{
		Content:      choice.Message.Content,
		ToolCalls:    toolCalls,
		FinishReason: choice.FinishReason,
	}, nil
}

func toWireToolCall(call ToolCall) wireToolCall {
	return wireToolCall{
		ID:   call.ID,
		Type: toolType,
		Function: wireFunctionCall{
			Name:      call.Name,
			Arguments: call.ArgumentsJSON,
		},
	}
}

func isTemporary(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func parseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		delay := time.Duration(seconds) * time.Second
		return delay, delay >= 0 && delay <= maxRetryAfter
	}

	date, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(date)
	return delay, delay >= 0 && delay <= maxRetryAfter
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type wireRequest struct {
	Model      string        `json:"model"`
	Messages   []wireMessage `json:"messages"`
	Tools      []wireTool    `json:"tools"`
	ToolChoice string        `json:"tool_choice"`
	Stream     bool          `json:"stream"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
}

type wireTool struct {
	Type     string           `json:"type"`
	Function wireFunctionSpec `json:"function"`
}

type wireFunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireResponse struct {
	Choices []wireChoice `json:"choices"`
}

type wireChoice struct {
	Message      wireResponseMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type wireResponseMessage struct {
	Content   string         `json:"content"`
	ToolCalls []wireToolCall `json:"tool_calls"`
}
