package config

const (
	ProviderDeepSeek = "deepseek"

	DefaultDeepSeekBaseURL = "https://api.deepseek.com"
	DefaultDeepSeekModel   = "deepseek-v4-pro"

	DefaultMaxSteps     = 8
	DefaultMaxBytes     = 524288
	DefaultMaxParaChars = 8000
)

var knownProviders = map[string]struct{}{
	ProviderDeepSeek: {},
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
	if cfg.Provider != ProviderDeepSeek {
		return
	}
	if !explicit.baseURL {
		cfg.BaseURL = DefaultDeepSeekBaseURL
	}
	if !explicit.model {
		cfg.Model = DefaultDeepSeekModel
	}
}
