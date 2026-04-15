package prompt

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

func TestSystemPromptContainsPerspective(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{
		Perspective: "Detect unusual login patterns",
	})

	sys := b.SystemPrompt()
	if !strings.Contains(sys, "Detect unusual login patterns") {
		t.Error("system prompt missing perspective")
	}
}

func TestSystemPromptContainsTargetFields(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{
		Perspective:  "test",
		TargetFields: []string{"timestamp", "user_id", "action"},
	})

	sys := b.SystemPrompt()
	if !strings.Contains(sys, "timestamp") {
		t.Error("system prompt missing target fields")
	}
}

func TestSystemPromptContainsAttentionPoints(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{
		Perspective:     "test",
		AttentionPoints: []string{"failed logins", "privilege escalation"},
	})

	sys := b.SystemPrompt()
	if !strings.Contains(sys, "failed logins") {
		t.Error("system prompt missing attention points")
	}
}

func TestWindowPromptFirstWindow(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{Perspective: "test"})

	records := []types.Record{
		{Index: 0, Source: "test.jsonl", RawJSON: json.RawMessage(`{"a":1}`)},
		{Index: 1, Source: "test.jsonl", RawJSON: json.RawMessage(`{"a":2}`)},
	}

	prompt, err := b.WindowPrompt("", nil, records, 0)
	if err != nil {
		t.Fatalf("WindowPrompt: %v", err)
	}

	if !strings.Contains(prompt, "N/A (first window)") {
		t.Error("first window should show N/A for previous summary")
	}
	if !strings.Contains(prompt, "[Record #0]") {
		t.Error("prompt missing record index")
	}
	if !strings.Contains(prompt, "[Record #1]") {
		t.Error("prompt missing record index")
	}
}

func TestWindowPromptWithSummaryAndFindings(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{Perspective: "test"})

	findings := []types.Finding{
		{ID: "F-001", Description: "test finding", Severity: "high"},
	}

	prompt, err := b.WindowPrompt("Previous analysis summary", findings, nil, 1)
	if err != nil {
		t.Fatalf("WindowPrompt: %v", err)
	}

	if !strings.Contains(prompt, "Previous analysis summary") {
		t.Error("prompt missing previous summary")
	}
	if !strings.Contains(prompt, "F-001") {
		t.Error("prompt missing finding")
	}
}

func TestWindowPromptWithUserFindings(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{
		Perspective:  "test",
		UserFindings: []string{"track admin actions"},
	})

	prompt, err := b.WindowPrompt("", nil, nil, 0)
	if err != nil {
		t.Fatalf("WindowPrompt: %v", err)
	}

	if !strings.Contains(prompt, "track admin actions") {
		t.Error("prompt missing user findings")
	}
}

func TestFinalPrompt(t *testing.T) {
	b := NewBuilder(&types.AnalysisParam{Perspective: "test"})

	findings := []types.Finding{
		{ID: "F-001", Description: "finding 1"},
	}

	sys, user, err := b.FinalPrompt("summary text", findings)
	if err != nil {
		t.Fatalf("FinalPrompt: %v", err)
	}

	if !strings.Contains(sys, "final analysis report") {
		t.Error("system prompt missing report instruction")
	}
	if !strings.Contains(user, "summary text") {
		t.Error("user prompt missing summary")
	}
	if !strings.Contains(user, "F-001") {
		t.Error("user prompt missing finding")
	}
}
