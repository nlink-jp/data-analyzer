package prepare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

type mockClient struct {
	responses []string
	callCount int
}

func (m *mockClient) Chat(_ context.Context, _, _ string) (string, error) {
	if m.callCount >= len(m.responses) {
		return "", fmt.Errorf("unexpected call #%d", m.callCount)
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func TestSessionRunAcceptFirst(t *testing.T) {
	params := types.AnalysisParam{
		Perspective:     "Detect suspicious login patterns",
		TargetFields:    []string{"user", "action", "source_ip"},
		AttentionPoints: []string{"Failed logins", "Brute force"},
	}
	paramsJSON, _ := json.Marshal(params)

	client := &mockClient{
		responses: []string{string(paramsJSON)},
	}

	// User types description, then Enter to accept
	stdin := strings.NewReader("Analyze login logs for suspicious patterns\n\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Perspective != "Detect suspicious login patterns" {
		t.Errorf("Perspective = %q, want %q", result.Perspective, "Detect suspicious login patterns")
	}
	if len(result.TargetFields) != 3 {
		t.Errorf("TargetFields = %d, want 3", len(result.TargetFields))
	}
}

func TestSessionRunWithRefinement(t *testing.T) {
	initial := types.AnalysisParam{
		Perspective:  "Detect suspicious activity",
		TargetFields: []string{"user", "action"},
	}
	refined := types.AnalysisParam{
		Perspective:  "Detect suspicious activity",
		TargetFields: []string{"user", "action", "user_agent"},
	}
	initialJSON, _ := json.Marshal(initial)
	refinedJSON, _ := json.Marshal(refined)

	client := &mockClient{
		responses: []string{string(initialJSON), string(refinedJSON)},
	}

	// User types description, then refinement, then Enter to accept
	stdin := strings.NewReader("Analyze logs\nadd field: user_agent\n\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(result.TargetFields) != 3 {
		t.Errorf("TargetFields = %v, want 3 fields", result.TargetFields)
	}
}

func TestSessionRunQuit(t *testing.T) {
	params := types.AnalysisParam{Perspective: "test"}
	paramsJSON, _ := json.Marshal(params)

	client := &mockClient{
		responses: []string{string(paramsJSON)},
	}

	stdin := strings.NewReader("Analyze logs\nquit\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	_, err := session.Run(context.Background())
	if err == nil {
		t.Fatal("expected error on quit")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error = %q, want 'cancelled'", err.Error())
	}
}

func TestParseParams(t *testing.T) {
	input := `{"perspective":"test","target_fields":["a","b"],"attention_points":["c"],"user_findings":[]}`
	params, err := parseParams(input)
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if params.Perspective != "test" {
		t.Errorf("Perspective = %q, want test", params.Perspective)
	}
}

func TestParseParamsWithThinkTags(t *testing.T) {
	input := `<think>reasoning</think>{"perspective":"cleaned","target_fields":[],"attention_points":[],"user_findings":[]}`
	params, err := parseParams(input)
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if params.Perspective != "cleaned" {
		t.Errorf("Perspective = %q, want cleaned", params.Perspective)
	}
}

func TestParseParamsEmptyPerspective(t *testing.T) {
	input := `{"perspective":"","target_fields":[]}`
	_, err := parseParams(input)
	if err == nil {
		t.Fatal("expected error for empty perspective")
	}
}
