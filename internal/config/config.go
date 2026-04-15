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
	Tuning   TuningConfig   `toml:"tuning"`
}

// TuningConfig holds advanced tuning parameters for token estimation and memory layout.
// Most users should not need to change these.
type TuningConfig struct {
	// Token estimation coefficients
	CJKTokenRatio   float64 `toml:"cjk_token_ratio"`   // tokens per CJK character (default: 2.0)
	ASCIITokenRatio float64 `toml:"ascii_token_ratio"`  // tokens per ASCII word (default: 1.3)
	CharsPerToken   int     `toml:"chars_per_token"`    // chars per token for char-based estimate (default: 4)

	// Memory map reserves (in tokens)
	SystemReserve   int `toml:"system_reserve"`    // tokens reserved for system prompt (default: 2000)
	ResponseReserve int `toml:"response_reserve"`  // tokens reserved for LLM response (default: 5000)
	MaxSummary      int `toml:"max_summary"`       // max tokens for running summary (default: 15000)
	MaxFindings     int `toml:"max_findings_budget"` // max tokens for findings in context (default: 20000)
	MinRawData      int `toml:"min_raw_data"`      // minimum tokens for RAW data per window (default: 10000)
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
		Tuning: TuningConfig{
			CJKTokenRatio:   2.0,
			ASCIITokenRatio: 1.3,
			CharsPerToken:   4,
			SystemReserve:   2000,
			ResponseReserve: 5000,
			MaxSummary:      15000,
			MaxFindings:     20000,
			MinRawData:      10000,
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
