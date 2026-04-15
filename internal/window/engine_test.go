package window

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/nlink-jp/data-analyzer/internal/job"
	"github.com/nlink-jp/data-analyzer/internal/prompt"
	"github.com/nlink-jp/data-analyzer/internal/types"
)

// mockClient is a test double for llm.Client.
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

func makeRecords(n int) []types.Record {
	records := make([]types.Record, n)
	for i := range n {
		records[i] = types.Record{
			Index:   i,
			Source:  "test.jsonl",
			RawJSON: json.RawMessage(fmt.Sprintf(`{"id":%d,"action":"test"}`, i)),
		}
	}
	return records
}

func TestEngineRunBasic(t *testing.T) {
	params := &types.AnalysisParam{Perspective: "test analysis"}
	builder := prompt.NewBuilder(params)

	dir := t.TempDir()
	mgr, _ := job.NewManager(dir)
	jobID, _, _ := mgr.ResolveJob("", params, []string{"test"})

	windowResp := types.WindowResponse{
		Summary: "Found some patterns",
		NewFindings: []types.Finding{
			{
				Description: "Test finding",
				Severity:    "medium",
				Citations: []types.Citation{
					{RecordIndex: 0, Source: "test.jsonl"},
				},
			},
		},
	}
	windowJSON, _ := json.Marshal(windowResp)

	finalResp := types.WindowResponse{
		Summary: "Final analysis complete",
	}
	finalJSON, _ := json.Marshal(finalResp)

	client := &mockClient{
		responses: []string{string(windowJSON), string(finalJSON)},
	}

	engine := NewEngine(client, builder, mgr, &EngineConfig{
		ContextLimit: 131072,
		OverlapRatio: 0.0,
		MaxFindings:  100,
		JobID:        jobID,
		Params:       params,
		Stderr:       io.Discard,
	})

	records := makeRecords(3)
	result, err := engine.Run(context.Background(), records, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.TotalRecords != 3 {
		t.Errorf("TotalRecords = %d, want 3", result.TotalRecords)
	}
	if len(result.Findings) == 0 {
		t.Error("expected at least one finding")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestEngineEvictFindings(t *testing.T) {
	engine := &Engine{
		cfg: &EngineConfig{MaxFindings: 3},
	}

	findings := []types.Finding{
		{ID: "F-001", Severity: "high"},
		{ID: "F-002", Severity: "info"},
		{ID: "F-003", Severity: "low"},
		{ID: "F-004", Severity: "medium"},
		{ID: "F-005", Severity: "info"},
	}

	result := engine.evictFindings(findings)

	if len(result) != 3 {
		t.Fatalf("got %d findings, want 3", len(result))
	}

	// Should keep high and medium
	hasHigh := false
	hasMedium := false
	for _, f := range result {
		if f.Severity == "high" {
			hasHigh = true
		}
		if f.Severity == "medium" {
			hasMedium = true
		}
	}
	if !hasHigh {
		t.Error("missing high severity finding")
	}
	if !hasMedium {
		t.Error("missing medium severity finding")
	}
}

func TestParseWindowResponse(t *testing.T) {
	input := `{"summary":"test summary","new_findings":[{"description":"found something","severity":"high","citations":[{"record_index":5,"source":"test.jsonl"}]}]}`

	counter := 0
	sourceIdx := map[int]string{5: "test.jsonl"}
	resp, err := parseWindowResponse(input, 0, &counter, sourceIdx)
	if err != nil {
		t.Fatalf("parseWindowResponse: %v", err)
	}

	if resp.Summary != "test summary" {
		t.Errorf("Summary = %q, want %q", resp.Summary, "test summary")
	}
	if len(resp.NewFindings) != 1 {
		t.Fatalf("NewFindings = %d, want 1", len(resp.NewFindings))
	}
	if resp.NewFindings[0].ID != "F-001" {
		t.Errorf("ID = %q, want F-001", resp.NewFindings[0].ID)
	}
	if resp.NewFindings[0].WindowIndex != 0 {
		t.Errorf("WindowIndex = %d, want 0", resp.NewFindings[0].WindowIndex)
	}
}

func TestParseWindowResponseInvalidSeverity(t *testing.T) {
	input := `{"summary":"s","new_findings":[{"description":"f","severity":"critical"}]}`

	counter := 0
	sourceIdx := map[int]string{5: "test.jsonl"}
	resp, err := parseWindowResponse(input, 0, &counter, sourceIdx)
	if err != nil {
		t.Fatalf("parseWindowResponse: %v", err)
	}

	if resp.NewFindings[0].Severity != "info" {
		t.Errorf("Severity = %q, want info (fallback)", resp.NewFindings[0].Severity)
	}
}

func TestVerifyCitationsCorrectExcerpt(t *testing.T) {
	recordMap := map[int]*types.Record{
		5: {Index: 5, Source: "test.jsonl", RawJSON: json.RawMessage(`{"user":"alice","action":"login"}`)},
	}

	findings := []types.Finding{
		{
			ID: "F-001",
			Citations: []types.Citation{
				{RecordIndex: 5, Source: "test.jsonl", Excerpt: json.RawMessage(`{"user":"alice"}`)},
			},
		},
	}

	corrections := verifyCitations(findings, recordMap, io.Discard)
	if corrections != 0 {
		t.Errorf("expected 0 corrections, got %d", corrections)
	}
	if len(findings[0].Citations) != 1 {
		t.Errorf("expected 1 citation, got %d", len(findings[0].Citations))
	}
}

func TestVerifyCitationsMismatchedExcerpt(t *testing.T) {
	recordMap := map[int]*types.Record{
		5: {Index: 5, Source: "test.jsonl", RawJSON: json.RawMessage(`{"user":"alice","action":"login"}`)},
	}

	findings := []types.Finding{
		{
			ID: "F-001",
			Citations: []types.Citation{
				{RecordIndex: 5, Source: "test.jsonl", Excerpt: json.RawMessage(`{"user":"bob"}`)},
			},
		},
	}

	corrections := verifyCitations(findings, recordMap, io.Discard)
	if corrections != 1 {
		t.Errorf("expected 1 correction, got %d", corrections)
	}

	// Excerpt should be corrected to actual value
	var excerpt map[string]any
	json.Unmarshal(findings[0].Citations[0].Excerpt, &excerpt)
	if excerpt["user"] != "alice" {
		t.Errorf("corrected user = %v, want alice", excerpt["user"])
	}
}

func TestVerifyCitationsOutOfRange(t *testing.T) {
	recordMap := map[int]*types.Record{
		5: {Index: 5, Source: "test.jsonl", RawJSON: json.RawMessage(`{"user":"alice"}`)},
	}

	findings := []types.Finding{
		{
			ID: "F-001",
			Citations: []types.Citation{
				{RecordIndex: 999, Source: "test.jsonl", Excerpt: json.RawMessage(`{"user":"alice"}`)},
			},
		},
	}

	corrections := verifyCitations(findings, recordMap, io.Discard)
	if corrections != 1 {
		t.Errorf("expected 1 correction, got %d", corrections)
	}
	if len(findings[0].Citations) != 0 {
		t.Errorf("expected 0 citations after removing out-of-range, got %d", len(findings[0].Citations))
	}
}

func TestVerifyCitationsMultipleMixed(t *testing.T) {
	recordMap := map[int]*types.Record{
		1: {Index: 1, Source: "a.jsonl", RawJSON: json.RawMessage(`{"x":1}`)},
		2: {Index: 2, Source: "a.jsonl", RawJSON: json.RawMessage(`{"x":2}`)},
	}

	findings := []types.Finding{
		{
			ID: "F-001",
			Citations: []types.Citation{
				{RecordIndex: 1, Excerpt: json.RawMessage(`{"x":1}`)},    // correct
				{RecordIndex: 2, Excerpt: json.RawMessage(`{"x":99}`)},   // mismatch
				{RecordIndex: 100, Excerpt: json.RawMessage(`{"x":1}`)},  // out of range
			},
		},
	}

	corrections := verifyCitations(findings, recordMap, io.Discard)
	if corrections != 2 {
		t.Errorf("expected 2 corrections, got %d", corrections)
	}
	if len(findings[0].Citations) != 2 {
		t.Errorf("expected 2 valid citations, got %d", len(findings[0].Citations))
	}

	// Second citation should be corrected
	var excerpt map[string]any
	json.Unmarshal(findings[0].Citations[1].Excerpt, &excerpt)
	if excerpt["x"] != float64(2) {
		t.Errorf("corrected x = %v, want 2", excerpt["x"])
	}
}
