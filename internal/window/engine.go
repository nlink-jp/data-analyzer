package window

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
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
	ContextLimit        int
	OverlapRatio        float64
	MaxFindings         int
	MaxRecordsPerWindow int
	MemoryLimits        MemoryLimits
	JobID               string
	Params              *types.AnalysisParam
	Stderr              io.Writer
	Debug               bool
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
		mm := ComputeMemoryMap(e.cfg.ContextLimit, summaryTokens, findingsTokens, e.cfg.MemoryLimits)

		// Determine records for this window
		remaining := records[recordOffset:]
		count := RecordsForBudget(remaining, mm.RawData)
		if count == 0 {
			count = 1 // always process at least one record
		}
		// Enforce max records per window to maintain LLM output quality
		if e.cfg.MaxRecordsPerWindow > 0 && count > e.cfg.MaxRecordsPerWindow {
			count = e.cfg.MaxRecordsPerWindow
		}
		windowRecords := remaining[:count]

		// Trim findings to fit token budget before building prompt
		promptFindings := trimFindingsForBudget(findings, mm.Findings)

		// Build user prompt
		userPrompt, err := e.builder.WindowPrompt(summary, promptFindings, windowRecords, windowIndex)
		if err != nil {
			return nil, fmt.Errorf("building prompt for window %d: %w", windowIndex, err)
		}

		// Progress display
		endRecord := recordOffset + count
		if endRecord > len(records) {
			endRecord = len(records)
		}
		progress := float64(endRecord) / float64(len(records)) * 100
		fmt.Fprintf(e.cfg.Stderr, "\rProcessing window %d (records %d–%d of %d, %.0f%%, %d in window)...",
			windowIndex, recordOffset, endRecord-1, len(records), progress, count)

		if e.cfg.Debug {
			promptFindingsJSON, _ := json.Marshal(promptFindings)
			promptFindingsTokens := token.Estimate(string(promptFindingsJSON))
			fmt.Fprintf(e.cfg.Stderr, "\n[debug] window=%d offset=%d count=%d budget=%d summary=%d findings=%d (prompt=%d, budget=%d)\n",
				windowIndex, recordOffset, count, mm.RawData, summaryTokens, findingsTokens, promptFindingsTokens, mm.Findings)
		}

		// Pre-flight: confirm model is loaded before calling LLM
		if err := e.client.WaitForModel(ctx); err != nil {
			return nil, fmt.Errorf("model health-check before window %d: %w", windowIndex, err)
		}

		// Call LLM (retry once on parse failure)
		var windowResp *types.WindowResponse
		for attempt := range 2 {
			response, err := e.client.Chat(ctx, systemPrompt, userPrompt)
			if err != nil {
				return nil, fmt.Errorf("LLM call for window %d: %w", windowIndex, err)
			}

			windowResp, err = parseWindowResponse(response, windowIndex, &findingCounter, sourceIndex)
			if err == nil {
				break
			}
			if attempt == 0 {
				fmt.Fprintf(e.cfg.Stderr, "\nWarning: failed to parse window %d response, retrying: %v\n", windowIndex, err)
			} else {
				fmt.Fprintf(e.cfg.Stderr, "\nWarning: failed to parse window %d response after retry, skipping: %v\n", windowIndex, err)
			}
			if e.cfg.Debug {
				fmt.Fprintf(e.cfg.Stderr, "[debug] failed response (attempt %d, %d chars):\n%s\n---\n",
					attempt+1, len(response), response)
			}
		}
		if windowResp == nil {
			// Both attempts failed — skip this window
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

	if err := e.client.WaitForModel(ctx); err != nil {
		return "", fmt.Errorf("model health-check before final report: %w", err)
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

// trimFindingsForBudget returns a copy of findings that fits within the given
// token budget. When the full findings exceed the budget, older findings have
// their citation excerpts replaced with a compact placeholder to reduce size.
// The original findings slice is never modified.
func trimFindingsForBudget(findings []types.Finding, budget int) []types.Finding {
	if len(findings) == 0 || budget <= 0 {
		return findings
	}

	fullJSON, _ := json.Marshal(findings)
	if token.Estimate(string(fullJSON)) <= budget {
		return findings
	}

	// Deep-copy and strip excerpts from oldest findings first
	trimmed := make([]types.Finding, len(findings))
	for i := range findings {
		trimmed[i] = findings[i]
		// Deep-copy citations so we don't modify the originals
		trimmed[i].Citations = make([]types.Citation, len(findings[i].Citations))
		copy(trimmed[i].Citations, findings[i].Citations)
	}

	placeholder := json.RawMessage(`"[see original]"`)

	// Strip excerpts from oldest findings until we fit the budget
	for i := 0; i < len(trimmed); i++ {
		for j := range trimmed[i].Citations {
			trimmed[i].Citations[j].Excerpt = placeholder
		}
		trimmedJSON, _ := json.Marshal(trimmed)
		if token.Estimate(string(trimmedJSON)) <= budget {
			return trimmed
		}
	}

	// Still over budget after stripping all excerpts — also truncate descriptions
	for i := 0; i < len(trimmed); i++ {
		if len(trimmed[i].Description) > 100 {
			trimmed[i].Description = trimmed[i].Description[:100] + "..."
		}
	}

	return trimmed
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

			// Check if LLM excerpt values exist in the original record
			origStr := string(rec.RawJSON)
			excerptRelevant := isExcerptRelevant(c.Excerpt, origStr)

			if excerptRelevant {
				// Excerpt references real data — replace with full original
				c.Excerpt = rec.RawJSON
				c.Source = rec.Source
				valid = append(valid, c)
				corrections++ // always replace to get full record
			} else {
				// Excerpt values not found in original — likely hallucinated
				fmt.Fprintf(w, "  [citation-verify] WARNING: %s Record #%d — excerpt does not match original, possible hallucination\n", f.ID, c.RecordIndex)
				// Still include with full original so the user can judge
				c.Excerpt = rec.RawJSON
				c.Source = rec.Source
				valid = append(valid, c)
				corrections++
			}
		}

		f.Citations = valid

		// If no citations remain, try to recover from description text
		if len(f.Citations) == 0 {
			recovered := recoverCitations(f.Description, recordMap)
			if len(recovered) > 0 {
				f.Citations = recovered
				corrections++
				fmt.Fprintf(w, "  [citation-verify] %s: recovered %d citation(s) from description\n", f.ID, len(recovered))
			} else {
				fmt.Fprintf(w, "  [citation-verify] WARNING: %s has no valid citations — finding may be hallucinated\n", f.ID)
			}
		}
	}

	return corrections
}

// isExcerptRelevant checks whether the LLM-generated excerpt contains values
// that actually exist in the original record string. If at least one non-trivial
// value from the excerpt is found in the original, the citation is considered valid.
func isExcerptRelevant(excerpt json.RawMessage, original string) bool {
	if len(excerpt) == 0 || string(excerpt) == "null" {
		return false
	}

	// Try to parse excerpt as JSON object
	var excerptMap map[string]any
	if err := json.Unmarshal(excerpt, &excerptMap); err != nil {
		// Not a JSON object — check if raw text appears in original
		return strings.Contains(original, strings.TrimSpace(string(excerpt)))
	}

	// Check if any value from the excerpt appears in the original
	matchCount := 0
	for _, val := range excerptMap {
		valStr := fmt.Sprintf("%v", val)
		// Skip trivial values
		if valStr == "" || valStr == "0" || valStr == "true" || valStr == "false" {
			continue
		}
		if strings.Contains(original, valStr) {
			matchCount++
		}
	}

	// At least one non-trivial value must match
	return matchCount > 0
}

var reRecordRef = regexp.MustCompile(`(?i)Record\s*#?(\d+)`)

// recoverCitations extracts Record #N references from text and builds
// citations from the original records.
func recoverCitations(text string, recordMap map[int]*types.Record) []types.Citation {
	matches := reRecordRef.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[int]bool)
	var citations []types.Citation
	for _, m := range matches {
		idx, err := strconv.Atoi(m[1])
		if err != nil || seen[idx] {
			continue
		}
		seen[idx] = true

		rec, ok := recordMap[idx]
		if !ok {
			continue
		}

		citations = append(citations, types.Citation{
			RecordIndex: idx,
			Source:      rec.Source,
			Excerpt:     rec.RawJSON,
		})
	}

	return citations
}
