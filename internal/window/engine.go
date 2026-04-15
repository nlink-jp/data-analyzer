package window

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/nlink-jp/data-analyzer/internal/job"
	"github.com/nlink-jp/data-analyzer/internal/llm"
	"github.com/nlink-jp/data-analyzer/internal/prompt"
	"github.com/nlink-jp/data-analyzer/internal/token"
	"github.com/nlink-jp/data-analyzer/internal/types"
	"github.com/nlink-jp/nlk/jsonfix"
	"github.com/nlink-jp/nlk/validate"
)

// EngineConfig holds configuration for the analysis engine.
type EngineConfig struct {
	ContextLimit int
	OverlapRatio float64
	MaxFindings  int
	JobID        string
	Params       *types.AnalysisParam
	Stderr       io.Writer
	Debug        bool
}

// Engine orchestrates the sliding window analysis.
type Engine struct {
	client  llm.Client
	builder *prompt.Builder
	mgr     *job.Manager
	cfg     *EngineConfig
}

// NewEngine creates a new analysis engine.
func NewEngine(client llm.Client, builder *prompt.Builder, mgr *job.Manager, cfg *EngineConfig) *Engine {
	return &Engine{
		client:  client,
		builder: builder,
		mgr:     mgr,
		cfg:     cfg,
	}
}

// buildRecordIndex builds a lookup from record index to source file path.
func buildRecordIndex(records []types.Record) map[int]string {
	m := make(map[int]string, len(records))
	for _, r := range records {
		m[r.Index] = r.Source
	}
	return m
}

// buildRecordMap builds a lookup from record index to the full record.
func buildRecordMap(records []types.Record) map[int]*types.Record {
	m := make(map[int]*types.Record, len(records))
	for i := range records {
		m[records[i].Index] = &records[i]
	}
	return m
}

// Run executes the sliding window analysis over the given records.
// If state is non-nil, resumes from the checkpoint.
func (e *Engine) Run(ctx context.Context, records []types.Record, state *types.WindowState) (*types.AnalysisResult, error) {
	startedAt := time.Now().UTC()

	// Initialize from checkpoint or fresh
	var (
		summary      string
		findings     []types.Finding
		windowIndex  int
		recordOffset int
	)

	if state != nil {
		summary = state.Summary
		findings = state.Findings
		windowIndex = state.WindowIndex + 1
		recordOffset = state.RecordOffset
		fmt.Fprintf(e.cfg.Stderr, "Resuming from window %d (offset %d)\n", windowIndex, recordOffset)
	}

	systemPrompt := e.builder.SystemPrompt()
	findingCounter := len(findings)
	sourceIndex := buildRecordIndex(records)
	recordMap := buildRecordMap(records)

	for recordOffset < len(records) {
		// Check for cancellation
		if err := ctx.Err(); err != nil {
			fmt.Fprintf(e.cfg.Stderr, "\nInterrupted at window %d. Use --resume %s to continue.\n",
				windowIndex, e.cfg.JobID)
			return nil, err
		}

		// Compute memory budget
		summaryTokens := token.Estimate(summary)
		findingsJSON, _ := json.Marshal(findings)
		findingsTokens := token.Estimate(string(findingsJSON))
		mm := ComputeMemoryMap(e.cfg.ContextLimit, summaryTokens, findingsTokens)

		// Determine records for this window
		remaining := records[recordOffset:]
		count := RecordsForBudget(remaining, mm.RawData)
		if count == 0 {
			count = 1 // always process at least one record
		}
		windowRecords := remaining[:count]

		// Build user prompt
		userPrompt, err := e.builder.WindowPrompt(summary, findings, windowRecords, windowIndex)
		if err != nil {
			return nil, fmt.Errorf("building prompt for window %d: %w", windowIndex, err)
		}

		if e.cfg.Debug {
			fmt.Fprintf(e.cfg.Stderr, "[debug] window=%d records=%d-%d budget=%d\n",
				windowIndex, recordOffset, recordOffset+count-1, mm.RawData)
		}

		// Progress display
		progress := float64(recordOffset+count) / float64(len(records)) * 100
		fmt.Fprintf(e.cfg.Stderr, "\rProcessing window %d (%d/%d records, %.0f%%)...",
			windowIndex, recordOffset+count, len(records), progress)

		// Call LLM
		response, err := e.client.Chat(ctx, systemPrompt, userPrompt)
		if err != nil {
			return nil, fmt.Errorf("LLM call for window %d: %w", windowIndex, err)
		}

		// Parse response
		windowResp, err := parseWindowResponse(response, windowIndex, &findingCounter, sourceIndex)
		if err != nil {
			fmt.Fprintf(e.cfg.Stderr, "\nWarning: failed to parse window %d response, skipping: %v\n", windowIndex, err)
			// Continue with next window rather than failing entirely
		} else {
			summary = windowResp.Summary

			// Verify citations against original records
			corrections := verifyCitations(windowResp.NewFindings, recordMap, e.cfg.Stderr)
			if corrections > 0 {
				fmt.Fprintf(e.cfg.Stderr, "  [citation-verify] Window %d: %d citation(s) corrected\n", windowIndex, corrections)
			}

			findings = append(findings, windowResp.NewFindings...)

			// Evict low-priority findings if exceeding max
			findings = e.evictFindings(findings)
		}

		// Calculate overlap
		overlapCount := int(float64(count) * e.cfg.OverlapRatio)
		nextOffset := recordOffset + count - overlapCount
		if nextOffset <= recordOffset {
			nextOffset = recordOffset + count // prevent infinite loop
		}

		// Save checkpoint
		checkpoint := &types.WindowState{
			WindowIndex:  windowIndex,
			RecordOffset: nextOffset,
			Summary:      summary,
			Findings:     findings,
			TotalRecords: len(records),
			ProcessedAt:  time.Now().UTC(),
		}
		if err := e.mgr.SaveCheckpoint(e.cfg.JobID, checkpoint); err != nil {
			fmt.Fprintf(e.cfg.Stderr, "\nWarning: failed to save checkpoint: %v\n", err)
		}

		recordOffset = nextOffset
		windowIndex++
	}

	fmt.Fprintf(e.cfg.Stderr, "\nGenerating final report...\n")

	// Final report generation
	finalSummary, err := e.generateFinalReport(ctx, summary, findings)
	if err != nil {
		// Use the last summary if final report generation fails
		fmt.Fprintf(e.cfg.Stderr, "Warning: final report generation failed, using last summary: %v\n", err)
		finalSummary = summary
	}

	result := &types.AnalysisResult{
		JobID:        e.cfg.JobID,
		Params:       *e.cfg.Params,
		Summary:      finalSummary,
		Findings:     findings,
		TotalRecords: len(records),
		WindowsUsed:  windowIndex,
		StartedAt:    startedAt,
		CompletedAt:  time.Now().UTC(),
	}

	fmt.Fprintf(e.cfg.Stderr, "Analysis complete: %d findings across %d windows\n",
		len(findings), windowIndex)

	return result, nil
}

func (e *Engine) generateFinalReport(ctx context.Context, summary string, findings []types.Finding) (string, error) {
	system, user, err := e.builder.FinalPrompt(summary, findings)
	if err != nil {
		return "", err
	}

	response, err := e.client.Chat(ctx, system, user)
	if err != nil {
		return "", err
	}

	var resp types.WindowResponse
	if err := jsonfix.ExtractTo(response, &resp); err != nil {
		// If JSON extraction fails, return the raw response as summary
		return response, nil
	}

	return resp.Summary, nil
}

func (e *Engine) evictFindings(findings []types.Finding) []types.Finding {
	if e.cfg.MaxFindings <= 0 || len(findings) <= e.cfg.MaxFindings {
		return findings
	}

	// Keep high/medium severity, evict info/low in FIFO order
	var high, other []types.Finding
	for _, f := range findings {
		if f.Severity == "high" || f.Severity == "medium" {
			high = append(high, f)
		} else {
			other = append(other, f)
		}
	}

	remaining := e.cfg.MaxFindings - len(high)
	if remaining < 0 {
		remaining = 0
	}
	if remaining < len(other) {
		other = other[len(other)-remaining:] // keep most recent
	}

	return append(high, other...)
}

func parseWindowResponse(text string, windowIndex int, counter *int, sourceIndex map[int]string) (*types.WindowResponse, error) {
	var resp types.WindowResponse
	if err := jsonfix.ExtractTo(text, &resp); err != nil {
		return nil, fmt.Errorf("JSON extraction: %w (raw: %.200s)", err, text)
	}

	// Validate and assign IDs to new findings
	for i := range resp.NewFindings {
		f := &resp.NewFindings[i]
		*counter++
		f.ID = fmt.Sprintf("F-%03d", *counter)
		f.WindowIndex = windowIndex

		// Validate severity
		if err := validate.Run(
			validate.OneOf("severity", f.Severity, types.ValidSeverities...),
		); err != nil {
			f.Severity = "info" // fallback
		}

		// Fix citation sources: replace LLM-generated source with actual file path
		for j := range f.Citations {
			if actual, ok := sourceIndex[f.Citations[j].RecordIndex]; ok {
				f.Citations[j].Source = actual
			}
		}
	}

	return &resp, nil
}

// verifyCitations checks each citation's excerpt against the original record.
// - Out-of-range record_index: citation removed, warning emitted.
// - Excerpt field value mismatch: excerpt replaced with actual fields, warning emitted.
// Returns the number of corrections made.
func verifyCitations(findings []types.Finding, recordMap map[int]*types.Record, w io.Writer) int {
	corrections := 0

	for i := range findings {
		f := &findings[i]
		var valid []types.Citation

		for _, c := range f.Citations {
			rec, ok := recordMap[c.RecordIndex]
			if !ok {
				fmt.Fprintf(w, "  [citation-verify] %s: Record #%d out of range — citation removed\n", f.ID, c.RecordIndex)
				corrections++
				continue
			}

			// Parse original record and excerpt
			var original map[string]any
			if err := json.Unmarshal(rec.RawJSON, &original); err != nil {
				valid = append(valid, c)
				continue
			}

			var excerpt map[string]any
			if err := json.Unmarshal(c.Excerpt, &excerpt); err != nil {
				// Excerpt is not a JSON object — replace with relevant fields from original
				corrected, _ := json.Marshal(original)
				c.Excerpt = corrected
				c.Source = rec.Source
				valid = append(valid, c)
				corrections++
				fmt.Fprintf(w, "  [citation-verify] %s: Record #%d excerpt malformed — replaced with original\n", f.ID, c.RecordIndex)
				continue
			}

			// Verify each field in excerpt exists and matches original
			mismatched := false
			for key, val := range excerpt {
				origVal, exists := original[key]
				if !exists {
					mismatched = true
					break
				}
				// Compare JSON representations for type-safe comparison
				excerptJSON, _ := json.Marshal(val)
				origJSON, _ := json.Marshal(origVal)
				if string(excerptJSON) != string(origJSON) {
					mismatched = true
					break
				}
			}

			if mismatched {
				// Replace excerpt with actual fields that the LLM tried to cite
				corrected := make(map[string]any)
				for key := range excerpt {
					if origVal, exists := original[key]; exists {
						corrected[key] = origVal
					}
				}
				correctedJSON, _ := json.Marshal(corrected)
				fmt.Fprintf(w, "  [citation-verify] %s: Record #%d excerpt mismatch — corrected from original\n", f.ID, c.RecordIndex)
				c.Excerpt = correctedJSON
				corrections++
			}

			c.Source = rec.Source
			valid = append(valid, c)
		}

		f.Citations = valid

		// Alert if all citations were removed
		if len(f.Citations) == 0 && len(findings[i].Citations) > 0 {
			fmt.Fprintf(w, "  [citation-verify] WARNING: %s has no valid citations — finding may be hallucinated\n", f.ID)
		}
	}

	return corrections
}
