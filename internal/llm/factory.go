package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/xiaoxlm/ai-file/internal/config"
	"github.com/xiaoxlm/ai-file/internal/llm/openaicompat"
)

// New creates the configured LLM client.
func New(cfg config.Config) (Client, error) {
	switch cfg.Provider {
	case config.ProviderDeepSeek, config.ProviderOpenAI, config.ProviderCustom:
	default:
		return nil, fmt.Errorf(
			"unknown provider %q; supported: %s, %s, %s",
			cfg.Provider,
			config.ProviderDeepSeek,
			config.ProviderOpenAI,
			config.ProviderCustom,
		)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("api_key is required")
	}

	baseURL := cfg.BaseURL
	model := cfg.Model
	switch cfg.Provider {
	case config.ProviderDeepSeek:
		if strings.TrimSpace(baseURL) == "" {
			baseURL = config.DefaultDeepSeekBaseURL
		}
		if strings.TrimSpace(model) == "" {
			model = config.DefaultDeepSeekModel
		}
	case config.ProviderOpenAI:
		if strings.TrimSpace(baseURL) == "" {
			baseURL = config.DefaultOpenAIBaseURL
		}
		if strings.TrimSpace(model) == "" {
			return nil, errors.New("model is required for provider openai")
		}
	case config.ProviderCustom:
		if strings.TrimSpace(baseURL) == "" {
			return nil, errors.New("base_url is required for provider custom")
		}
		if strings.TrimSpace(model) == "" {
			return nil, errors.New("model is required for provider custom")
		}
	}

	adapter, err := openaicompat.New(openaicompat.Config{
		BaseURL:      baseURL,
		APIKey:       cfg.APIKey,
		DefaultModel: model,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai-compatible client: %w", err)
	}

	return &openAICompatibleClient{
		adapter:      adapter,
		baseURL:      baseURL,
		defaultModel: model,
	}, nil
}

type openAICompatibleClient struct {
	adapter      *openaicompat.Client
	baseURL      string
	defaultModel string
}

func (c *openAICompatibleClient) Chat(
	ctx context.Context,
	request ChatRequest,
) (ChatResponse, error) {
	response, err := c.adapter.Chat(ctx, toAdapterRequest(request))
	if err != nil {
		return ChatResponse{}, err
	}

	toolCalls := make([]ToolCall, 0, len(response.ToolCalls))
	for _, call := range response.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			ID:            call.ID,
			Name:          call.Name,
			ArgumentsJSON: call.ArgumentsJSON,
		})
	}

	return ChatResponse{
		Content:      response.Content,
		ToolCalls:    toolCalls,
		FinishReason: response.FinishReason,
	}, nil
}

func toAdapterRequest(request ChatRequest) openaicompat.ChatRequest {
	messages := make([]openaicompat.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		toolCalls := make([]openaicompat.ToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			toolCalls = append(toolCalls, openaicompat.ToolCall{
				ID:            call.ID,
				Name:          call.Name,
				ArgumentsJSON: call.ArgumentsJSON,
			})
		}
		messages = append(messages, openaicompat.Message{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  toolCalls,
		})
	}

	tools := make([]openaicompat.ToolSpec, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, openaicompat.ToolSpec{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	return openaicompat.ChatRequest{
		Messages: messages,
		Tools:    tools,
		Model:    request.Model,
	}
}
