package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.API.Endpoint != "http://localhost:1234/v1" {
		t.Errorf("endpoint = %q, want %q", cfg.API.Endpoint, "http://localhost:1234/v1")
	}
	if cfg.API.Model != "google/gemma-4-26b-a4b" {
		t.Errorf("model = %q, want %q", cfg.API.Model, "google/gemma-4-26b-a4b")
	}
	if cfg.Analysis.ContextLimit != 131072 {
		t.Errorf("context_limit = %d, want %d", cfg.Analysis.ContextLimit, 131072)
	}
	if cfg.Analysis.OverlapRatio != 0.1 {
		t.Errorf("overlap_ratio = %f, want %f", cfg.Analysis.OverlapRatio, 0.1)
	}
	if cfg.Analysis.MaxFindings != 100 {
		t.Errorf("max_findings = %d, want %d", cfg.Analysis.MaxFindings, 100)
	}
}

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[api]
endpoint = "http://example.com/v1"
model    = "test-model"
api_key  = "test-key"

[analysis]
context_limit = 65536
overlap_ratio = 0.2
max_findings  = 50

[job]
temp_dir = "/tmp/test"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.API.Endpoint != "http://example.com/v1" {
		t.Errorf("endpoint = %q, want %q", cfg.API.Endpoint, "http://example.com/v1")
	}
	if cfg.API.Model != "test-model" {
		t.Errorf("model = %q, want %q", cfg.API.Model, "test-model")
	}
	if cfg.API.APIKey != "test-key" {
		t.Errorf("api_key = %q, want %q", cfg.API.APIKey, "test-key")
	}
	if cfg.Analysis.ContextLimit != 65536 {
		t.Errorf("context_limit = %d, want %d", cfg.Analysis.ContextLimit, 65536)
	}
	if cfg.Analysis.OverlapRatio != 0.2 {
		t.Errorf("overlap_ratio = %f, want %f", cfg.Analysis.OverlapRatio, 0.2)
	}
	if cfg.Job.TempDir != "/tmp/test" {
		t.Errorf("temp_dir = %q, want %q", cfg.Job.TempDir, "/tmp/test")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("DATA_ANALYZER_API_ENDPOINT", "http://env.example.com/v1")
	t.Setenv("DATA_ANALYZER_API_MODEL", "env-model")
	t.Setenv("DATA_ANALYZER_API_KEY", "env-key")
	t.Setenv("DATA_ANALYZER_CONTEXT_LIMIT", "32768")

	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.API.Endpoint != "http://env.example.com/v1" {
		t.Errorf("endpoint = %q, want %q", cfg.API.Endpoint, "http://env.example.com/v1")
	}
	if cfg.API.Model != "env-model" {
		t.Errorf("model = %q, want %q", cfg.API.Model, "env-model")
	}
	if cfg.API.APIKey != "env-key" {
		t.Errorf("api_key = %q, want %q", cfg.API.APIKey, "env-key")
	}
	if cfg.Analysis.ContextLimit != 32768 {
		t.Errorf("context_limit = %d, want %d", cfg.Analysis.ContextLimit, 32768)
	}
}

func TestApplyFlags(t *testing.T) {
	cfg, _ := Load("/nonexistent/path/config.toml")
	cfg.ApplyFlags("flag-model")

	if cfg.API.Model != "flag-model" {
		t.Errorf("model = %q, want %q", cfg.API.Model, "flag-model")
	}
}
