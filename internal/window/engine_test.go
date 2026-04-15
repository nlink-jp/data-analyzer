package window

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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

func (m *mockClient) WaitForModel(_ context.Context) error { return nil }

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
		MemoryLimits: DefaultMemoryLimits(),
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

func TestVerifyCitationsRelevantExcerptReplacedWithOriginal(t *testing.T) {
	original := `{"user":"alice","action":"login","ip":"10.0.0.1"}`
	recordMap := map[int]*types.Record{
		5: {Index: 5, Source: "test.jsonl", RawJSON: json.RawMessage(original)},
	}

	// Partial excerpt with values that exist in original → relevant, replaced with full record
	findings := []types.Finding{
		{
			ID: "F-001",
			Citations: []types.Citation{
				{RecordIndex: 5, Source: "test.jsonl", Excerpt: json.RawMessage(`{"user":"alice"}`)},
			},
		},
	}

	verifyCitations(findings, recordMap, io.Discard)

	if string(findings[0].Citations[0].Excerpt) != original {
		t.Errorf("excerpt = %s, want full original %s", findings[0].Citations[0].Excerpt, original)
	}
}

func TestVerifyCitationsHallucinatedExcerptWarns(t *testing.T) {
	original := `{"user":"alice","action":"login"}`
	recordMap := map[int]*types.Record{
		5: {Index: 5, Source: "test.jsonl", RawJSON: json.RawMessage(original)},
	}

	// Excerpt with values NOT in original → hallucination warning but still replaced
	findings := []types.Finding{
		{
			ID: "F-001",
			Citations: []types.Citation{
				{RecordIndex: 5, Source: "test.jsonl", Excerpt: json.RawMessage(`{"user":"evil_hacker","action":"delete_all"}`)},
			},
		},
	}

	var stderr bytes.Buffer
	verifyCitations(findings, recordMap, &stderr)

	if !strings.Contains(stderr.String(), "possible hallucination") {
		t.Error("expected hallucination warning")
	}
	// Should still include with full original for user to judge
	if string(findings[0].Citations[0].Excerpt) != original {
		t.Errorf("excerpt should be replaced with original even on hallucination")
	}
}

func TestIsExcerptRelevant(t *testing.T) {
	original := `{"user":"alice","action":"login","ts":"2026-04-15"}`
	tests := []struct {
		name    string
		excerpt string
		want    bool
	}{
		{"matching value", `{"user":"alice"}`, true},
		{"no match", `{"user":"evil"}`, false},
		{"null", `null`, false},
		{"empty", ``, false},
		{"partial match", `{"user":"alice","role":"admin"}`, true}, // alice matches
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcerptRelevant(json.RawMessage(tt.excerpt), original)
			if got != tt.want {
				t.Errorf("isExcerptRelevant(%s) = %v, want %v", tt.excerpt, got, tt.want)
			}
		})
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

func TestVerifyCitationsRecoverFromDescription(t *testing.T) {
	recordMap := map[int]*types.Record{
		5: {Index: 5, Source: "test.jsonl", RawJSON: json.RawMessage(`{"user":"alice","action":"login"}`)},
		8: {Index: 8, Source: "test.jsonl", RawJSON: json.RawMessage(`{"user":"bob","action":"logout"}`)},
	}

	// Finding has no citations but description mentions records
	findings := []types.Finding{
		{
			ID:          "F-001",
			Description: "Suspicious activity observed in Record #5 and Record #8",
			Citations:   nil,
		},
	}

	corrections := verifyCitations(findings, recordMap, io.Discard)
	if corrections != 1 {
		t.Errorf("expected 1 correction (recovery), got %d", corrections)
	}
	if len(findings[0].Citations) != 2 {
		t.Fatalf("expected 2 recovered citations, got %d", len(findings[0].Citations))
	}
	if findings[0].Citations[0].RecordIndex != 5 {
		t.Errorf("first citation index = %d, want 5", findings[0].Citations[0].RecordIndex)
	}
	if findings[0].Citations[1].RecordIndex != 8 {
		t.Errorf("second citation index = %d, want 8", findings[0].Citations[1].RecordIndex)
	}
}

func TestRecoverCitationsFromText(t *testing.T) {
	recordMap := map[int]*types.Record{
		1: {Index: 1, Source: "a.jsonl", RawJSON: json.RawMessage(`{"x":1}`)},
		3: {Index: 3, Source: "a.jsonl", RawJSON: json.RawMessage(`{"x":3}`)},
	}

	tests := []struct {
		name string
		text string
		want int
	}{
		{"standard", "See Record #1 and Record #3", 2},
		{"no hash", "See Record 1", 1},
		{"case insensitive", "see record #1", 1},
		{"duplicate", "Record #1 again Record #1", 1},
		{"no match", "no records here", 0},
		{"out of range", "Record #99", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recoverCitations(tt.text, recordMap)
			if len(got) != tt.want {
				t.Errorf("recoverCitations(%q) = %d citations, want %d", tt.text, len(got), tt.want)
			}
		})
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
	// 3 corrections: record 1 replaced with original, record 2 hallucination+replaced, record 100 removed
	if corrections != 3 {
		t.Errorf("expected 3 corrections, got %d", corrections)
	}
	if len(findings[0].Citations) != 2 {
		t.Errorf("expected 2 valid citations, got %d", len(findings[0].Citations))
	}

	// Second citation should be replaced with full original
	var excerpt map[string]any
	json.Unmarshal(findings[0].Citations[1].Excerpt, &excerpt)
	if excerpt["x"] != float64(2) {
		t.Errorf("corrected x = %v, want 2", excerpt["x"])
	}
	if findings[0].Citations[1].Source != "a.jsonl" {
		t.Errorf("source = %q, want a.jsonl", findings[0].Citations[1].Source)
	}
}
