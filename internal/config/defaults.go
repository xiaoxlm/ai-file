package config

const (
	ProviderDeepSeek = "deepseek"
	ProviderOpenAI   = "openai"
	ProviderCustom   = "custom"

	DefaultDeepSeekBaseURL = "https://api.deepseek.com"
	DefaultDeepSeekModel   = "deepseek-v4-pro"
	DefaultOpenAIBaseURL   = "https://api.openai.com/v1"

	DefaultMaxSteps     = 8
	DefaultMaxBytes     = 524288
	DefaultMaxParaChars = 8000
)

var knownProviders = map[string]struct{}{
	ProviderDeepSeek: {},
	ProviderOpenAI:   {},
	ProviderCustom:   {},
}

func defaultConfig() Config {
	return Config{
		Provider:     ProviderDeepSeek,
		MaxSteps:     DefaultMaxSteps,
		MaxBytes:     DefaultMaxBytes,
		MaxParaChars: DefaultMaxParaChars,
	}
}

func applyProviderPreset(cfg *Config, explicit fieldFlags) {
	switch cfg.Provider {
	case ProviderDeepSeek:
		if !explicit.baseURL {
			cfg.BaseURL = DefaultDeepSeekBaseURL
		}
		if !explicit.model {
			cfg.Model = DefaultDeepSeekModel
		}
	case ProviderOpenAI:
		if !explicit.baseURL {
			cfg.BaseURL = DefaultOpenAIBaseURL
		}
	case ProviderCustom:
		// custom requires explicit base_url and model
	}
}
