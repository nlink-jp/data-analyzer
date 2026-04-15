package compile

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

func TestMarkdown(t *testing.T) {
	result := &types.AnalysisResult{
		JobID: "test-job-123",
		Params: types.AnalysisParam{
			Perspective: "Detect insider threats",
		},
		Summary:      "Found 2 suspicious activities.",
		TotalRecords: 100,
		WindowsUsed:  3,
		StartedAt:    time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 4, 15, 10, 5, 0, 0, time.UTC),
		Findings: []types.Finding{
			{
				ID:          "F-001",
				Description: "Brute-force login attempt",
				Severity:    "high",
				WindowIndex: 0,
				Citations: []types.Citation{
					{
						RecordIndex: 7,
						Source:      "logs.jsonl",
						Excerpt:     json.RawMessage(`{"user":"admin","action":"login","result":"failure"}`),
					},
				},
			},
			{
				ID:          "F-002",
				Description: "Normal user logout",
				Severity:    "info",
				WindowIndex: 1,
				Citations:   nil,
			},
		},
	}

	var buf bytes.Buffer
	if err := Markdown(&buf, result); err != nil {
		t.Fatalf("Markdown: %v", err)
	}

	output := buf.String()

	// Check sections exist
	checks := []string{
		"# Analysis Report",
		"test-job-123",
		"Detect insider threats",
		"Found 2 suspicious activities",
		"## Findings Overview",
		"| F-001 |",
		"| F-002 |",
		"**HIGH**",
		"1 High, 0 Medium, 0 Low, 1 Info",
		"## Detailed Findings",
		"### F-001:",
		"Record #7",
		"logs.jsonl",
		"```json",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestMarkdownNoFindings(t *testing.T) {
	result := &types.AnalysisResult{
		JobID:        "empty-job",
		Summary:      "Nothing found.",
		TotalRecords: 10,
		WindowsUsed:  1,
		StartedAt:    time.Now(),
		CompletedAt:  time.Now(),
	}

	var buf bytes.Buffer
	if err := Markdown(&buf, result); err != nil {
		t.Fatalf("Markdown: %v", err)
	}

	if !strings.Contains(buf.String(), "No findings detected") {
		t.Error("expected 'No findings detected' message")
	}
}

func TestHTML(t *testing.T) {
	result := &types.AnalysisResult{
		JobID: "html-test-123",
		Params: types.AnalysisParam{
			Perspective: "Detect insider threats",
		},
		Summary:      "Found **critical** issues.\n\nSecond paragraph.",
		TotalRecords: 50,
		WindowsUsed:  2,
		StartedAt:    time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 4, 15, 10, 5, 0, 0, time.UTC),
		Findings: []types.Finding{
			{
				ID:          "F-001",
				Description: "Brute-force login",
				Severity:    "high",
				WindowIndex: 0,
				Citations: []types.Citation{
					{
						RecordIndex: 7,
						Source:      "logs.jsonl",
						Excerpt:     json.RawMessage(`{"user":"admin","result":"failure"}`),
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := HTML(&buf, result); err != nil {
		t.Fatalf("HTML: %v", err)
	}

	output := buf.String()

	checks := []string{
		"<!DOCTYPE html>",
		"html-test-123",
		"Detect insider threats",
		"<strong>critical</strong>",
		"Second paragraph",
		`class="badge high"`,
		"F-001",
		"Brute-force login",
		"Record #7",
		"logs.jsonl",
		`&#34;user&#34;`,
		"</html>",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("HTML output missing %q", check)
		}
	}
}

func TestHTMLNoFindings(t *testing.T) {
	result := &types.AnalysisResult{
		JobID:       "empty-html",
		Summary:     "Nothing found.",
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
	}

	var buf bytes.Buffer
	if err := HTML(&buf, result); err != nil {
		t.Fatalf("HTML: %v", err)
	}

	if !strings.Contains(buf.String(), "No findings detected") {
		t.Error("expected 'No findings detected'")
	}
}

func TestHTMLEscaping(t *testing.T) {
	result := &types.AnalysisResult{
		JobID:       "esc-test",
		Summary:     "Test <script>alert('xss')</script>",
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
	}

	var buf bytes.Buffer
	if err := HTML(&buf, result); err != nil {
		t.Fatalf("HTML: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "<script>") {
		t.Error("HTML output contains unescaped script tag — XSS vulnerability")
	}
	if !strings.Contains(output, "&lt;script&gt;") {
		t.Error("script tag not properly escaped")
	}
}

func TestConvertBold(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"no bold", "no bold"},
		{"**bold**", "<strong>bold</strong>"},
		{"a **b** c **d** e", "a <strong>b</strong> c <strong>d</strong> e"},
		{"**unclosed", "**unclosed"},
	}

	for _, tt := range tests {
		got := convertBold(tt.input)
		if got != tt.want {
			t.Errorf("convertBold(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is..."},
		{"newline\nin text", 20, "newline in text"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestSeverityBadge(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"high", "**HIGH**"},
		{"medium", "MEDIUM"},
		{"low", "low"},
		{"info", "info"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := severityBadge(tt.input)
		if got != tt.want {
			t.Errorf("severityBadge(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
