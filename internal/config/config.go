// Package config manages data-analyzer configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Config holds all data-analyzer configuration.
type Config struct {
	API      APIConfig      `toml:"api"`
	Analysis AnalysisConfig `toml:"analysis"`
	Job      JobConfig      `toml:"job"`
}

// APIConfig holds LLM API settings.
type APIConfig struct {
	Endpoint string `toml:"endpoint"`
	Model    string `toml:"model"`
	APIKey   string `toml:"api_key"`
}

// AnalysisConfig holds analysis engine settings.
type AnalysisConfig struct {
	ContextLimit        int     `toml:"context_limit"`
	OverlapRatio        float64 `toml:"overlap_ratio"`
	MaxFindings         int     `toml:"max_findings"`
	MaxRecordsPerWindow int     `toml:"max_records_per_window"`
	Lang                string  `toml:"lang"`
}

// JobConfig holds job management settings.
type JobConfig struct {
	TempDir string `toml:"temp_dir"`
}

// Load reads config with defaults, TOML file, and env var overrides.
func Load(path string) (*Config, error) {
	cfg := &Config{
		API: APIConfig{
			Endpoint: "http://localhost:1234/v1",
			Model:    "google/gemma-4-26b-a4b",
		},
		Analysis: AnalysisConfig{
			ContextLimit:        131072,
			OverlapRatio:        0.1,
			MaxFindings:         100,
			MaxRecordsPerWindow: 200,
		},
	}

	// TOML file
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".config", "data-analyzer", "config.toml")
		}
	}

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, cfg); err != nil {
				return nil, fmt.Errorf("parse config %s: %w", path, err)
			}
		}
	}

	// Env var overrides
	if v := os.Getenv("DATA_ANALYZER_API_ENDPOINT"); v != "" {
		cfg.API.Endpoint = v
	}
	if v := os.Getenv("DATA_ANALYZER_API_MODEL"); v != "" {
		cfg.API.Model = v
	}
	if v := os.Getenv("DATA_ANALYZER_API_KEY"); v != "" {
		cfg.API.APIKey = v
	}
	if v := os.Getenv("DATA_ANALYZER_CONTEXT_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Analysis.ContextLimit = n
		}
	}
	if v := os.Getenv("DATA_ANALYZER_OVERLAP_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Analysis.OverlapRatio = f
		}
	}
	if v := os.Getenv("DATA_ANALYZER_MAX_FINDINGS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Analysis.MaxFindings = n
		}
	}
	if v := os.Getenv("DATA_ANALYZER_MAX_RECORDS_PER_WINDOW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Analysis.MaxRecordsPerWindow = n
		}
	}
	if v := os.Getenv("DATA_ANALYZER_LANG"); v != "" {
		cfg.Analysis.Lang = v
	}
	if v := os.Getenv("DATA_ANALYZER_TEMP_DIR"); v != "" {
		cfg.Job.TempDir = v
	}

	if cfg.API.Endpoint == "" {
		return nil, fmt.Errorf("API endpoint is required: set api.endpoint in config or DATA_ANALYZER_API_ENDPOINT")
	}
	if cfg.API.Model == "" {
		return nil, fmt.Errorf("API model is required: set api.model in config or DATA_ANALYZER_API_MODEL")
	}

	return cfg, nil
}

// ApplyFlags overrides config values with CLI flag values.
func (c *Config) ApplyFlags(model string) {
	if model != "" {
		c.API.Model = model
	}
}
