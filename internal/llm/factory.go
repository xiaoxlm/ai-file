package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xiaoxlm/ai-file/internal/config"
)

// New creates the configured LLM client.
func New(cfg config.Config) (Client, error) {
	if cfg.Provider != config.ProviderDeepSeek {
		return nil, fmt.Errorf(
			"unknown provider %q; supported: %s",
			cfg.Provider,
			config.ProviderDeepSeek,
		)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("api_key is required")
	}

	baseURL := cfg.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = config.DefaultDeepSeekBaseURL
	}

	model := cfg.Model
	if strings.TrimSpace(model) == "" {
		model = config.DefaultDeepSeekModel
	}

	return newDeepSeekClient(baseURL, cfg.APIKey, model)
}
