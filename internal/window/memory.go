// Package window implements the sliding window analysis engine.
package window

import (
	"fmt"

	"github.com/nlink-jp/data-analyzer/internal/token"
	"github.com/nlink-jp/data-analyzer/internal/types"
)

const (
	systemReserve   = 2000
	responseReserve = 5000
	maxSummary      = 15000
	maxFindingsBudget = 20000
	minRawData      = 10000
)

// MemoryMap describes how the context budget is allocated.
type MemoryMap struct {
	SystemPrompt    int
	PreviousSummary int
	Findings        int
	RawData         int
	ResponseBuffer  int
}

// ComputeMemoryMap calculates the context budget allocation based on
// current summary and findings token usage.
func ComputeMemoryMap(contextLimit int, summaryTokens int, findingsTokens int) MemoryMap {
	available := contextLimit - systemReserve - responseReserve

	summary := min(summaryTokens, maxSummary)
	findings := min(findingsTokens, maxFindingsBudget)
	rawData := available - summary - findings

	// If RAW data budget is too small, reduce findings allocation
	if rawData < minRawData {
		findings = available - summary - minRawData
		if findings < 0 {
			findings = 0
		}
		rawData = available - summary - findings
	}

	return MemoryMap{
		SystemPrompt:    systemReserve,
		PreviousSummary: summary,
		Findings:        findings,
		RawData:         rawData,
		ResponseBuffer:  responseReserve,
	}
}

// RecordsForBudget returns how many records from the slice fit within
// the given token budget.
func RecordsForBudget(records []types.Record, budget int) int {
	used := 0
	for i, r := range records {
		// Estimate: "[Record #N] " prefix + JSON content
		prefix := token.Estimate(fmt.Sprintf("[Record #%d] ", r.Index))
		content := token.Estimate(string(r.RawJSON))
		total := prefix + content

		if used+total > budget && i > 0 {
			return i
		}
		used += total
	}
	return len(records)
}
