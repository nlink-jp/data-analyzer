package prepare

import (
	"bufio"
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

	// Single-line description, then empty line to end input, then empty line to accept
	stdin := strings.NewReader("Analyze login logs for suspicious patterns\n\n\n")
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

func TestSessionRunMultiLineInput(t *testing.T) {
	params := types.AnalysisParam{
		Perspective:     "Detect insider threats",
		TargetFields:    []string{"user", "action"},
		AttentionPoints: []string{"Privilege escalation"},
	}
	paramsJSON, _ := json.Marshal(params)

	client := &mockClient{
		responses: []string{string(paramsJSON)},
	}

	// Multi-line description with empty line as terminator, then accept
	stdin := strings.NewReader("Analyze user activity logs.\nLook for privilege escalation.\nFocus on admin accounts.\n\n\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Perspective != "Detect insider threats" {
		t.Errorf("Perspective = %q, want %q", result.Perspective, "Detect insider threats")
	}

	// Verify the LLM received the multi-line input
	if client.callCount != 1 {
		t.Errorf("LLM call count = %d, want 1", client.callCount)
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

	// Description (end with empty line), refinement (end with empty line), accept (empty line)
	stdin := strings.NewReader("Analyze logs\n\nadd field: user_agent\n\n\n")
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

	// Description, then quit
	stdin := strings.NewReader("Analyze logs\n\nquit\n\n")
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

func TestSessionRunLargeInput(t *testing.T) {
	params := types.AnalysisParam{
		Perspective:  "Detect anomalies",
		TargetFields: []string{"user"},
	}
	paramsJSON, _ := json.Marshal(params)

	client := &mockClient{
		responses: []string{string(paramsJSON)},
	}

	// Large single line + empty line terminator + accept
	largeInput := strings.Repeat("a", 100*1024) + "\n\n\n"
	stdin := strings.NewReader(largeInput)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run with large input: %v", err)
	}
	if result.Perspective != "Detect anomalies" {
		t.Errorf("Perspective = %q, want %q", result.Perspective, "Detect anomalies")
	}
}

func TestSessionRunLargeMultiLine(t *testing.T) {
	params := types.AnalysisParam{
		Perspective:  "Detect threats",
		TargetFields: []string{"user"},
	}
	paramsJSON, _ := json.Marshal(params)

	client := &mockClient{
		responses: []string{string(paramsJSON)},
	}

	// Simulate pasting a large multi-line prompt (like the user's actual use case)
	var sb strings.Builder
	sb.WriteString("You are an expert security analyst.\n")
	sb.WriteString("Analyze the following logs.\n")
	for i := range 50 {
		sb.WriteString(fmt.Sprintf("- Check domain pattern %d for anomalies\n", i))
	}
	sb.WriteString("\n") // empty line to end input
	sb.WriteString("\n") // accept
	stdin := strings.NewReader(sb.String())
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Perspective != "Detect threats" {
		t.Errorf("Perspective = %q, want %q", result.Perspective, "Detect threats")
	}
}

func TestSessionRunWithFileInput(t *testing.T) {
	params := types.AnalysisParam{
		Perspective:     "Detect insider threats in user activity logs",
		TargetFields:    []string{"user", "action", "source_ip"},
		AttentionPoints: []string{"Privilege escalation", "Data exfiltration"},
	}
	paramsJSON, _ := json.Marshal(params)

	client := &mockClient{
		responses: []string{string(paramsJSON)},
	}

	fileContent := "You are an expert security analyst.\nAnalyze user activity logs.\nLook for privilege escalation and data exfiltration."

	// Stdin only has the accept (empty line) since description comes from file
	stdin := strings.NewReader("\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.RunWithInput(context.Background(), fileContent)
	if err != nil {
		t.Fatalf("RunWithInput: %v", err)
	}

	if result.Perspective != "Detect insider threats in user activity logs" {
		t.Errorf("Perspective = %q", result.Perspective)
	}

	// Verify stdout shows loaded message
	if !strings.Contains(stdout.String(), "Loaded initial requirements") {
		t.Error("stdout should show loaded message")
	}
}

func TestSessionRunWithFileInputThenRefine(t *testing.T) {
	initial := types.AnalysisParam{
		Perspective:  "Detect threats",
		TargetFields: []string{"user"},
	}
	refined := types.AnalysisParam{
		Perspective:  "Detect threats",
		TargetFields: []string{"user", "device"},
	}
	initialJSON, _ := json.Marshal(initial)
	refinedJSON, _ := json.Marshal(refined)

	client := &mockClient{
		responses: []string{string(initialJSON), string(refinedJSON)},
	}

	// After file input: refine, then accept
	stdin := strings.NewReader("add field: device\n\n\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	session := NewSession(client, stdin, stdout, stderr)
	result, err := session.RunWithInput(context.Background(), "Analyze logs")
	if err != nil {
		t.Fatalf("RunWithInput: %v", err)
	}

	if len(result.TargetFields) != 2 {
		t.Errorf("TargetFields = %v, want 2 fields", result.TargetFields)
	}
}

func TestReadMultiLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single line",
			input: "hello\n\n",
			want:  "hello",
		},
		{
			name:  "multi line",
			input: "line1\nline2\nline3\n\n",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "empty input",
			input: "\n",
			want:  "",
		},
		{
			name:  "EOF without empty line",
			input: "hello",
			want:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			got, err := readMultiLine(scanner)
			if err != nil {
				t.Fatalf("readMultiLine: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
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
