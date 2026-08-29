package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type fieldFlags struct {
	provider     bool
	apiKey       bool
	baseURL      bool
	model        bool
	maxSteps     bool
	maxBytes     bool
	maxParaChars bool
	verbose      bool
}

type yamlConfig struct {
	Provider     string `yaml:"provider"`
	APIKey       string `yaml:"api_key"`
	BaseURL      string `yaml:"base_url"`
	Model        string `yaml:"model"`
	MaxSteps     *int   `yaml:"max_steps"`
	MaxBytes     *int64 `yaml:"max_bytes"`
	MaxParaChars *int   `yaml:"max_para_chars"`
}

// Load merges configuration from defaults, provider presets, YAML, env, and flags.
func Load(opts LoadOptions) (Config, error) {
	cfg := defaultConfig()
	explicit := fieldFlags{}

	applyProviderPreset(&cfg, explicit)

	yamlCfg, err := readYAML(opts)
	if err != nil {
		return Config{}, err
	}
	mergeYAML(&cfg, yamlCfg, &explicit)

	applyProviderPreset(&cfg, explicit)

	lookup := resolveLookupEnv(opts)
	if err := mergeEnv(&cfg, lookup, &explicit); err != nil {
		return Config{}, err
	}

	mergeFlags(&cfg, opts, &explicit)
	applyProviderPreset(&cfg, explicit)

	if err := validate(cfg, explicit); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func resolveLookupEnv(opts LoadOptions) func(string) (string, bool) {
	if opts.LookupEnv != nil {
		return opts.LookupEnv
	}
	return os.LookupEnv
}

func resolveConfigPaths(opts LoadOptions) (cwdPath, homePath string) {
	if opts.ConfigPath != "" {
		cwdPath = opts.ConfigPath
	} else if wd, err := os.Getwd(); err == nil {
		cwdPath = filepath.Join(wd, "ai-file.yaml")
	}

	homeDir := opts.HomeDir
	if homeDir == "" {
		if h, err := os.UserHomeDir(); err == nil {
			homeDir = h
		}
	}
	if homeDir != "" {
		homePath = filepath.Join(homeDir, ".ai-file.yaml")
	}

	return cwdPath, homePath
}

func readYAML(opts LoadOptions) (yamlConfig, error) {
	cwdPath, homePath := resolveConfigPaths(opts)

	if cwdPath != "" {
		if _, err := os.Stat(cwdPath); err == nil {
			return parseYAMLFile(cwdPath)
		} else if !os.IsNotExist(err) {
			return yamlConfig{}, fmt.Errorf("read config %s: %w", cwdPath, err)
		}
	}

	if homePath != "" {
		if _, err := os.Stat(homePath); err == nil {
			return parseYAMLFile(homePath)
		} else if !os.IsNotExist(err) {
			return yamlConfig{}, fmt.Errorf("read config %s: %w", homePath, err)
		}
	}

	return yamlConfig{}, nil
}

func parseYAMLFile(path string) (yamlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return yamlConfig{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return yamlConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	return yc, nil
}

func mergeYAML(cfg *Config, yc yamlConfig, explicit *fieldFlags) {
	if yc.Provider != "" {
		cfg.Provider = yc.Provider
		explicit.provider = true
	}
	if yc.APIKey != "" {
		cfg.APIKey = yc.APIKey
		explicit.apiKey = true
	}
	if yc.BaseURL != "" {
		cfg.BaseURL = yc.BaseURL
		explicit.baseURL = true
	}
	if yc.Model != "" {
		cfg.Model = yc.Model
		explicit.model = true
	}
	if yc.MaxSteps != nil {
		cfg.MaxSteps = *yc.MaxSteps
		explicit.maxSteps = true
	}
	if yc.MaxBytes != nil {
		cfg.MaxBytes = *yc.MaxBytes
		explicit.maxBytes = true
	}
	if yc.MaxParaChars != nil {
		cfg.MaxParaChars = *yc.MaxParaChars
		explicit.maxParaChars = true
	}
}

func mergeEnv(cfg *Config, lookup func(string) (string, bool), explicit *fieldFlags) error {
	if v, ok := lookup("AI_FILE_PROVIDER"); ok && v != "" {
		cfg.Provider = v
		explicit.provider = true
	}
	if v, ok := lookup("AI_FILE_API_KEY"); ok && v != "" {
		cfg.APIKey = v
		explicit.apiKey = true
	}
	if v, ok := lookup("AI_FILE_BASE_URL"); ok && v != "" {
		cfg.BaseURL = v
		explicit.baseURL = true
	}
	if v, ok := lookup("AI_FILE_MODEL"); ok && v != "" {
		cfg.Model = v
		explicit.model = true
	}
	if v, ok := lookup("AI_FILE_MAX_STEPS"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid AI_FILE_MAX_STEPS: %w", err)
		}
		cfg.MaxSteps = n
		explicit.maxSteps = true
	}
	if v, ok := lookup("AI_FILE_MAX_BYTES"); ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid AI_FILE_MAX_BYTES: %w", err)
		}
		cfg.MaxBytes = n
		explicit.maxBytes = true
	}
	if v, ok := lookup("AI_FILE_MAX_PARA_CHARS"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid AI_FILE_MAX_PARA_CHARS: %w", err)
		}
		cfg.MaxParaChars = n
		explicit.maxParaChars = true
	}

	return nil
}

func mergeFlags(cfg *Config, opts LoadOptions, explicit *fieldFlags) {
	if opts.Provider != nil {
		cfg.Provider = *opts.Provider
		explicit.provider = true
	}
	if opts.BaseURL != nil {
		cfg.BaseURL = *opts.BaseURL
		explicit.baseURL = true
	}
	if opts.Model != nil {
		cfg.Model = *opts.Model
		explicit.model = true
	}
	if opts.MaxSteps != nil {
		cfg.MaxSteps = *opts.MaxSteps
		explicit.maxSteps = true
	}
	if opts.MaxBytes != nil {
		cfg.MaxBytes = *opts.MaxBytes
		explicit.maxBytes = true
	}
	if opts.MaxParaChars != nil {
		cfg.MaxParaChars = *opts.MaxParaChars
		explicit.maxParaChars = true
	}
	if opts.Verbose != nil {
		cfg.Verbose = *opts.Verbose
		explicit.verbose = true
	}
}

func validate(cfg Config, explicit fieldFlags) error {
	if _, ok := knownProviders[cfg.Provider]; !ok {
		names := []string{ProviderDeepSeek, ProviderOpenAI, ProviderCustom}
		return fmt.Errorf(
			"unknown provider %q; supported: %s",
			cfg.Provider,
			strings.Join(names, ", "),
		)
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("api_key is required")
	}

	switch cfg.Provider {
	case ProviderOpenAI:
		if !explicit.model {
			return fmt.Errorf("model is required for provider openai")
		}
	case ProviderCustom:
		if !explicit.baseURL {
			return fmt.Errorf("base_url is required for provider custom")
		}
		if !explicit.model {
			return fmt.Errorf("model is required for provider custom")
		}
	}

	if cfg.MaxSteps <= 0 {
		return fmt.Errorf("max_steps must be positive")
	}
	if cfg.MaxBytes <= 0 {
		return fmt.Errorf("max_bytes must be positive")
	}
	if cfg.MaxParaChars <= 0 {
		return fmt.Errorf("max_para_chars must be positive")
	}

	return nil
}
