package config

// Config holds merged runtime settings for ai-file.
type Config struct {
	Provider     string
	APIKey       string
	BaseURL      string
	Model        string
	MaxSteps     int
	MaxBytes     int64
	MaxParaChars int
	Verbose      bool
}

// LoadOptions controls config loading and optional test injection.
type LoadOptions struct {
	// ConfigPath is the cwd-level ai-file.yaml path; empty uses ./ai-file.yaml.
	ConfigPath string
	// HomeDir is the home directory for ~/.ai-file.yaml fallback; empty uses os.UserHomeDir.
	HomeDir string
	// LookupEnv resolves environment variables; nil uses os.LookupEnv.
	LookupEnv func(key string) (string, bool)

	Provider     *string
	BaseURL      *string
	Model        *string
	MaxSteps     *int
	MaxBytes     *int64
	MaxParaChars *int
	Verbose      *bool
}
