package llm

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
	deepSeekChatTimeout     = 60 * time.Second
	deepSeekMaxAttempts     = 3
	deepSeekMaxErrorBody    = 1024
	deepSeekMaxRetryAfter   = 30 * time.Second
	deepSeekRedactedValue   = "[redacted]"
	deepSeekToolType        = "function"
	deepSeekDefaultToolMode = "auto"
)

var deepSeekRetryBackoffs = [...]time.Duration{
	500 * time.Millisecond,
	time.Second,
}

type deepseekClient struct {
	endpoint     string
	apiKey       string
	defaultModel string
	httpClient   *http.Client
	sleep        func(context.Context, time.Duration) error
}

func newDeepSeekClient(baseURL, apiKey, defaultModel string) (*deepseekClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api_key is required")
	}
	if strings.TrimSpace(defaultModel) == "" {
		return nil, errors.New("default model is required")
	}

	return &deepseekClient{
		endpoint:     baseURL + "/chat/completions",
		apiKey:       apiKey,
		defaultModel: defaultModel,
		httpClient:   http.DefaultClient,
		sleep:        deepSeekSleepWithContext,
	}, nil
}

func (c *deepseekClient) Chat(
	ctx context.Context,
	request ChatRequest,
) (ChatResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, deepSeekChatTimeout)
	defer cancel()

	body, err := c.marshalRequest(request)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode chat request: %w", err)
	}

	for attempt := range deepSeekMaxAttempts {
		response, requestErr := c.do(ctx, body)
		if requestErr != nil {
			if attempt == deepSeekMaxAttempts-1 || !deepSeekIsTemporary(requestErr) {
				return ChatResponse{}, fmt.Errorf(
					"send chat request: %s",
					c.redact(requestErr.Error()),
				)
			}
			if err := c.sleep(ctx, deepSeekRetryBackoffs[attempt]); err != nil {
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

func (c *deepseekClient) marshalRequest(request ChatRequest) ([]byte, error) {
	model := request.Model
	if model == "" {
		model = c.defaultModel
	}

	messages := make([]deepSeekWireMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		toolCalls := make([]deepSeekWireToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			toolCalls = append(toolCalls, deepSeekToWireToolCall(call))
		}
		messages = append(messages, deepSeekWireMessage{
			Role:       message.Role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  toolCalls,
		})
	}

	tools := make([]deepSeekWireTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, deepSeekWireTool{
			Type: deepSeekToolType,
			Function: deepSeekWireFunctionSpec{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}

	return json.Marshal(deepSeekWireRequest{
		Model:      model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: deepSeekDefaultToolMode,
		Stream:     false,
	})
}

func (c *deepseekClient) do(ctx context.Context, body []byte) (*http.Response, error) {
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

func (c *deepseekClient) handleResponse(
	response *http.Response,
	attempt int,
) (ChatResponse, bool, time.Duration, error) {
	defer response.Body.Close()

	if response.StatusCode >= http.StatusOK &&
		response.StatusCode < http.StatusMultipleChoices {
		result, err := deepSeekDecodeResponse(response.Body)
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
	if !isRetryable || attempt == deepSeekMaxAttempts-1 {
		return ChatResponse{}, false, 0, responseErr
	}

	delay := deepSeekRetryBackoffs[attempt]
	if retryAfter, ok := deepSeekParseRetryAfter(
		response.Header.Get("Retry-After"),
	); ok {
		delay = retryAfter
	}
	return ChatResponse{}, true, delay, responseErr
}

func (c *deepseekClient) readErrorSummary(reader io.Reader) (string, error) {
	body, err := io.ReadAll(io.LimitReader(reader, deepSeekMaxErrorBody))
	if err != nil {
		return "", err
	}

	summary := c.redact(strings.TrimSpace(string(body)))
	if len(summary) > deepSeekMaxErrorBody {
		summary = summary[:deepSeekMaxErrorBody]
	}
	if summary == "" {
		return http.StatusText(http.StatusInternalServerError), nil
	}
	return summary, nil
}

func (c *deepseekClient) redact(text string) string {
	if c.apiKey == "" {
		return text
	}
	return strings.ReplaceAll(text, c.apiKey, deepSeekRedactedValue)
}

func deepSeekDecodeResponse(reader io.Reader) (ChatResponse, error) {
	var response deepSeekWireResponse
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

func deepSeekToWireToolCall(call ToolCall) deepSeekWireToolCall {
	return deepSeekWireToolCall{
		ID:   call.ID,
		Type: deepSeekToolType,
		Function: deepSeekWireFunctionCall{
			Name:      call.Name,
			Arguments: call.ArgumentsJSON,
		},
	}
}

func deepSeekIsTemporary(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func deepSeekParseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		delay := time.Duration(seconds) * time.Second
		return delay, delay >= 0 && delay <= deepSeekMaxRetryAfter
	}

	date, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(date)
	return delay, delay >= 0 && delay <= deepSeekMaxRetryAfter
}

func deepSeekSleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type deepSeekWireRequest struct {
	Model      string                `json:"model"`
	Messages   []deepSeekWireMessage `json:"messages"`
	Tools      []deepSeekWireTool    `json:"tools"`
	ToolChoice string                `json:"tool_choice"`
	Stream     bool                  `json:"stream"`
}

type deepSeekWireMessage struct {
	Role       Role                   `json:"role"`
	Content    string                 `json:"content"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolCalls  []deepSeekWireToolCall `json:"tool_calls,omitempty"`
}

type deepSeekWireTool struct {
	Type     string                   `json:"type"`
	Function deepSeekWireFunctionSpec `json:"function"`
}

type deepSeekWireFunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type deepSeekWireToolCall struct {
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function deepSeekWireFunctionCall `json:"function"`
}

type deepSeekWireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type deepSeekWireResponse struct {
	Choices []deepSeekWireChoice `json:"choices"`
}

type deepSeekWireChoice struct {
	Message      deepSeekWireResponseMessage `json:"message"`
	FinishReason string                      `json:"finish_reason"`
}

type deepSeekWireResponseMessage struct {
	Content   string                 `json:"content"`
	ToolCalls []deepSeekWireToolCall `json:"tool_calls"`
}
