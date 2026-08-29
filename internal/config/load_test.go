package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/config"
)

func TestLoad_DeepSeekDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(config.LoadOptions{
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_API_KEY": "test-key",
		}),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "deepseek" {
		t.Errorf("Provider = %q, want deepseek", cfg.Provider)
	}
	if cfg.BaseURL != "https://api.deepseek.com" {
		t.Errorf("BaseURL = %q, want https://api.deepseek.com", cfg.BaseURL)
	}
	if cfg.Model != "deepseek-v4-pro" {
		t.Errorf("Model = %q, want deepseek-v4-pro", cfg.Model)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", cfg.APIKey)
	}
	if cfg.MaxSteps != 8 {
		t.Errorf("MaxSteps = %d, want 8", cfg.MaxSteps)
	}
	if cfg.MaxBytes != 524288 {
		t.Errorf("MaxBytes = %d, want 524288", cfg.MaxBytes)
	}
	if cfg.MaxParaChars != 8000 {
		t.Errorf("MaxParaChars = %d, want 8000", cfg.MaxParaChars)
	}
	if cfg.Verbose {
		t.Error("Verbose = true, want false")
	}
}

func TestLoad_YAMLSnakeCase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ai-file.yaml")
	writeFile(t, cfgPath, `
provider: openai
api_key: yaml-key
base_url: https://example.com/v1
model: gpt-4o-mini
max_steps: 10
max_bytes: 1024
max_para_chars: 500
`)

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: cfgPath,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", cfg.Provider)
	}
	if cfg.APIKey != "yaml-key" {
		t.Errorf("APIKey = %q, want yaml-key", cfg.APIKey)
	}
	if cfg.BaseURL != "https://example.com/v1" {
		t.Errorf("BaseURL = %q, want https://example.com/v1", cfg.BaseURL)
	}
	if cfg.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, want gpt-4o-mini", cfg.Model)
	}
	if cfg.MaxSteps != 10 {
		t.Errorf("MaxSteps = %d, want 10", cfg.MaxSteps)
	}
	if cfg.MaxBytes != 1024 {
		t.Errorf("MaxBytes = %d, want 1024", cfg.MaxBytes)
	}
	if cfg.MaxParaChars != 500 {
		t.Errorf("MaxParaChars = %d, want 500", cfg.MaxParaChars)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ai-file.yaml")
	writeFile(t, cfgPath, `
provider: deepseek
api_key: yaml-key
model: deepseek-v4-pro
max_steps: 8
`)

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: cfgPath,
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_PROVIDER":  "openai",
			"AI_FILE_API_KEY":   "env-key",
			"AI_FILE_BASE_URL":  "https://env.example/v1",
			"AI_FILE_MODEL":     "env-model",
			"AI_FILE_MAX_STEPS": "12",
		}),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", cfg.Provider)
	}
	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want env-key", cfg.APIKey)
	}
	if cfg.BaseURL != "https://env.example/v1" {
		t.Errorf("BaseURL = %q, want https://env.example/v1", cfg.BaseURL)
	}
	if cfg.Model != "env-model" {
		t.Errorf("Model = %q, want env-model", cfg.Model)
	}
	if cfg.MaxSteps != 12 {
		t.Errorf("MaxSteps = %d, want 12", cfg.MaxSteps)
	}
}

func TestLoad_FlagsHighestPriority(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ai-file.yaml")
	writeFile(t, cfgPath, `
provider: deepseek
api_key: yaml-key
model: deepseek-v4-pro
`)

	provider := "custom"
	baseURL := "https://flag.example/v1"
	model := "flag-model"
	maxSteps := 15
	maxBytes := int64(2048)
	maxParaChars := 900
	verbose := true

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: cfgPath,
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_API_KEY": "env-key",
		}),
		Provider:     &provider,
		BaseURL:      &baseURL,
		Model:        &model,
		MaxSteps:     &maxSteps,
		MaxBytes:     &maxBytes,
		MaxParaChars: &maxParaChars,
		Verbose:      &verbose,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "custom" {
		t.Errorf("Provider = %q, want custom", cfg.Provider)
	}
	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want env-key", cfg.APIKey)
	}
	if cfg.BaseURL != baseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, baseURL)
	}
	if cfg.Model != model {
		t.Errorf("Model = %q, want %q", cfg.Model, model)
	}
	if cfg.MaxSteps != maxSteps {
		t.Errorf("MaxSteps = %d, want %d", cfg.MaxSteps, maxSteps)
	}
	if cfg.MaxBytes != maxBytes {
		t.Errorf("MaxBytes = %d, want %d", cfg.MaxBytes, maxBytes)
	}
	if cfg.MaxParaChars != maxParaChars {
		t.Errorf("MaxParaChars = %d, want %d", cfg.MaxParaChars, maxParaChars)
	}
	if !cfg.Verbose {
		t.Error("Verbose = false, want true")
	}
}

func TestLoad_HomeYAMLFallback(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	homeCfg := filepath.Join(home, ".ai-file.yaml")
	writeFile(t, homeCfg, `
provider: deepseek
api_key: home-key
model: deepseek-v4-pro
`)

	missingCwdCfg := filepath.Join(t.TempDir(), "ai-file.yaml")

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: missingCwdCfg,
		HomeDir:    home,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "home-key" {
		t.Errorf("APIKey = %q, want home-key", cfg.APIKey)
	}
}

func TestLoad_CwdYAMLPreferredOverHome(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".ai-file.yaml"), `
provider: deepseek
api_key: home-key
model: deepseek-v4-pro
`)

	dir := t.TempDir()
	cwdCfg := filepath.Join(dir, "ai-file.yaml")
	writeFile(t, cwdCfg, `
provider: deepseek
api_key: cwd-key
model: deepseek-v4-pro
`)

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: cwdCfg,
		HomeDir:    home,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "cwd-key" {
		t.Errorf("APIKey = %q, want cwd-key", cfg.APIKey)
	}
}

func TestLoad_UnknownProvider(t *testing.T) {
	t.Parallel()

	provider := "unknown-vendor"
	_, err := config.Load(config.LoadOptions{
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_API_KEY": "test-key",
		}),
		Provider: &provider,
	})
	if err == nil {
		t.Fatal("Load() error = nil, want unknown provider error")
	}
}

func TestLoad_MissingAPIKey(t *testing.T) {
	t.Parallel()

	_, err := config.Load(config.LoadOptions{
		LookupEnv: envLookup(t, map[string]string{}),
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing api key error")
	}
}

func TestLoad_InvalidNumericValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts config.LoadOptions
	}{
		{
			name: "zero max_steps flag",
			opts: config.LoadOptions{
				LookupEnv: envLookup(t, map[string]string{"AI_FILE_API_KEY": "k"}),
				MaxSteps:  intPtr(0),
			},
		},
		{
			name: "negative max_bytes env",
			opts: config.LoadOptions{
				LookupEnv: envLookup(t, map[string]string{
					"AI_FILE_API_KEY":   "k",
					"AI_FILE_MAX_BYTES": "-1",
				}),
			},
		},
		{
			name: "zero max_para_chars yaml",
			opts: func() config.LoadOptions {
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, "ai-file.yaml")
				writeFile(t, cfgPath, `
provider: deepseek
api_key: k
model: deepseek-v4-pro
max_para_chars: 0
`)
				return config.LoadOptions{ConfigPath: cfgPath}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(tt.opts)
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
		})
	}
}

func TestLoad_OpenAIRequiresModel(t *testing.T) {
	t.Parallel()

	provider := "openai"
	_, err := config.Load(config.LoadOptions{
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_API_KEY": "test-key",
		}),
		Provider: &provider,
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing model error")
	}
}

func TestLoad_OpenAIPresetBaseURL(t *testing.T) {
	t.Parallel()

	provider := "openai"
	model := "gpt-4o-mini"
	cfg, err := config.Load(config.LoadOptions{
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_API_KEY": "test-key",
		}),
		Provider: &provider,
		Model:    &model,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL = %q, want https://api.openai.com/v1", cfg.BaseURL)
	}
}

func TestLoad_CustomRequiresBaseURLAndModel(t *testing.T) {
	t.Parallel()

	provider := "custom"
	apiKey := "test-key"

	t.Run("missing base_url", func(t *testing.T) {
		t.Parallel()
		model := "m"
		_, err := config.Load(config.LoadOptions{
			LookupEnv: envLookup(t, map[string]string{"AI_FILE_API_KEY": apiKey}),
			Provider:  &provider,
			Model:     &model,
		})
		if err == nil {
			t.Fatal("Load() error = nil, want missing base_url error")
		}
	})

	t.Run("missing model", func(t *testing.T) {
		t.Parallel()
		baseURL := "https://custom.example/v1"
		_, err := config.Load(config.LoadOptions{
			LookupEnv: envLookup(t, map[string]string{"AI_FILE_API_KEY": apiKey}),
			Provider:  &provider,
			BaseURL:   &baseURL,
		})
		if err == nil {
			t.Fatal("Load() error = nil, want missing model error")
		}
	})
}

func TestLoad_MissingYAMLNotError(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"),
		HomeDir:    t.TempDir(),
		LookupEnv: envLookup(t, map[string]string{
			"AI_FILE_API_KEY": "test-key",
		}),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", cfg.APIKey)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ai-file.yaml")
	writeFile(t, cfgPath, "provider: [unclosed")

	_, err := config.Load(config.LoadOptions{ConfigPath: cfgPath})
	if err == nil {
		t.Fatal("Load() error = nil, want yaml parse error")
	}
}

func envLookup(t *testing.T, values map[string]string) func(string) (string, bool) {
	t.Helper()
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func intPtr(v int) *int {
	return &v
}
