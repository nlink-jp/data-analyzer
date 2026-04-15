// Package types defines shared data structures for data-analyzer.
package types

import (
	"encoding/json"
	"time"
)

// AnalysisParam is the output of `prepare` and input to `analyze`.
type AnalysisParam struct {
	Perspective     string   `json:"perspective"`      // What to look for
	TargetFields    []string `json:"target_fields"`    // JSON fields to focus on
	AttentionPoints []string `json:"attention_points"` // Specific things to watch
	UserFindings    []string `json:"user_findings"`    // User-specified findings to track
	Lang            string   `json:"lang,omitempty"`   // Output language (e.g., "Japanese", "English")
}

// Record is a single data record from the input.
type Record struct {
	Index   int             `json:"index"`    // 0-based position in input
	Source  string          `json:"source"`   // Source file path
	RawJSON json.RawMessage `json:"raw_json"` // Original JSON bytes
}

// Finding represents a discovered insight with RAW data citations.
type Finding struct {
	ID          string     `json:"id"`           // F-001, F-002, etc.
	Description string     `json:"description"`  // What was found
	Severity    string     `json:"severity"`     // high, medium, low, info
	Citations   []Citation `json:"citations"`    // RAW data references
	WindowIndex int        `json:"window_index"` // Which window discovered this
}

// Citation references a specific record in the source data.
type Citation struct {
	RecordIndex int             `json:"record_index"` // Index in source
	Source      string          `json:"source"`       // Source file
	Excerpt     json.RawMessage `json:"excerpt"`      // Relevant portion of the record
}

// WindowState is saved as a checkpoint after each window.
type WindowState struct {
	WindowIndex  int       `json:"window_index"`
	RecordOffset int       `json:"record_offset"` // Next record to process
	Summary      string    `json:"summary"`       // Running summary
	Findings     []Finding `json:"findings"`       // Accumulated findings
	TotalRecords int       `json:"total_records"`
	ProcessedAt  time.Time `json:"processed_at"`
}

// AnalysisResult is the final output of `analyze`.
type AnalysisResult struct {
	JobID        string        `json:"job_id"`
	Params       AnalysisParam `json:"params"`
	Summary      string        `json:"summary"`
	Findings     []Finding     `json:"findings"`
	TotalRecords int           `json:"total_records"`
	WindowsUsed  int           `json:"windows_used"`
	StartedAt    time.Time     `json:"started_at"`
	CompletedAt  time.Time     `json:"completed_at"`
}

// WindowResponse is the expected LLM response for each window.
type WindowResponse struct {
	Summary     string    `json:"summary"`      // Updated running summary
	NewFindings []Finding `json:"new_findings"` // Newly discovered findings
}

// ValidSeverities is the set of valid severity values.
var ValidSeverities = []string{"high", "medium", "low", "info"}
