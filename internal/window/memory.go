// Package window implements the sliding window analysis engine.
package window

import (
	"fmt"

	"github.com/nlink-jp/data-analyzer/internal/token"
	"github.com/nlink-jp/data-analyzer/internal/types"
)

// MemoryLimits holds configurable memory map parameters.
type MemoryLimits struct {
	SystemReserve   int
	ResponseReserve int
	MaxSummary      int
	MaxFindings     int
	MinRawData      int
}

// DefaultMemoryLimits returns the default memory map parameters.
func DefaultMemoryLimits() MemoryLimits {
	return MemoryLimits{
		SystemReserve:   2000,
		ResponseReserve: 5000,
		MaxSummary:      15000,
		MaxFindings:     20000,
		MinRawData:      10000,
	}
}

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
func ComputeMemoryMap(contextLimit int, summaryTokens int, findingsTokens int, limits MemoryLimits) MemoryMap {
	available := contextLimit - limits.SystemReserve - limits.ResponseReserve

	summary := min(summaryTokens, limits.MaxSummary)
	findings := min(findingsTokens, limits.MaxFindings)
	rawData := available - summary - findings

	// If RAW data budget is too small, reduce findings allocation
	if rawData < limits.MinRawData {
		findings = available - summary - limits.MinRawData
		if findings < 0 {
			findings = 0
		}
		rawData = available - summary - findings
	}

	return MemoryMap{
		SystemPrompt:    limits.SystemReserve,
		PreviousSummary: summary,
		Findings:        findings,
		RawData:         rawData,
		ResponseBuffer:  limits.ResponseReserve,
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
