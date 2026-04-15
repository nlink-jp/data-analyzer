package window

import "testing"

func TestComputeMemoryMapDefaults(t *testing.T) {
	limits := DefaultMemoryLimits()
	mm := ComputeMemoryMap(131072, 0, 0, limits)

	if mm.SystemPrompt != limits.SystemReserve {
		t.Errorf("SystemPrompt = %d, want %d", mm.SystemPrompt, limits.SystemReserve)
	}
	if mm.ResponseBuffer != limits.ResponseReserve {
		t.Errorf("ResponseBuffer = %d, want %d", mm.ResponseBuffer, limits.ResponseReserve)
	}
	if mm.PreviousSummary != 0 {
		t.Errorf("PreviousSummary = %d, want 0", mm.PreviousSummary)
	}
	if mm.Findings != 0 {
		t.Errorf("Findings = %d, want 0", mm.Findings)
	}

	expectedRaw := 131072 - limits.SystemReserve - limits.ResponseReserve
	if mm.RawData != expectedRaw {
		t.Errorf("RawData = %d, want %d", mm.RawData, expectedRaw)
	}
}

func TestComputeMemoryMapWithSummaryAndFindings(t *testing.T) {
	limits := DefaultMemoryLimits()
	mm := ComputeMemoryMap(131072, 10000, 15000, limits)

	if mm.PreviousSummary != 10000 {
		t.Errorf("PreviousSummary = %d, want 10000", mm.PreviousSummary)
	}
	if mm.Findings != 15000 {
		t.Errorf("Findings = %d, want 15000", mm.Findings)
	}

	expectedRaw := 131072 - limits.SystemReserve - limits.ResponseReserve - 10000 - 15000
	if mm.RawData != expectedRaw {
		t.Errorf("RawData = %d, want %d", mm.RawData, expectedRaw)
	}
}

func TestComputeMemoryMapCapsSummary(t *testing.T) {
	limits := DefaultMemoryLimits()
	mm := ComputeMemoryMap(131072, 50000, 0, limits)

	if mm.PreviousSummary != limits.MaxSummary {
		t.Errorf("PreviousSummary = %d, want %d (capped)", mm.PreviousSummary, limits.MaxSummary)
	}
}

func TestComputeMemoryMapCapsFindings(t *testing.T) {
	limits := DefaultMemoryLimits()
	mm := ComputeMemoryMap(131072, 0, 50000, limits)

	if mm.Findings != limits.MaxFindings {
		t.Errorf("Findings = %d, want %d (capped)", mm.Findings, limits.MaxFindings)
	}
}

func TestComputeMemoryMapReducesFindingsWhenTight(t *testing.T) {
	limits := DefaultMemoryLimits()
	mm := ComputeMemoryMap(50000, 15000, 20000, limits)

	if mm.RawData < limits.MinRawData {
		t.Errorf("RawData = %d, should be >= %d", mm.RawData, limits.MinRawData)
	}
	if mm.PreviousSummary != limits.MaxSummary {
		t.Errorf("PreviousSummary = %d, want %d", mm.PreviousSummary, limits.MaxSummary)
	}
	if mm.Findings >= 20000 {
		t.Errorf("Findings = %d, should be reduced", mm.Findings)
	}
}

func TestComputeMemoryMapCustomLimits(t *testing.T) {
	limits := MemoryLimits{
		SystemReserve:   1000,
		ResponseReserve: 3000,
		MaxSummary:      5000,
		MaxFindings:     8000,
		MinRawData:      5000,
	}
	mm := ComputeMemoryMap(30000, 3000, 5000, limits)

	if mm.SystemPrompt != 1000 {
		t.Errorf("SystemPrompt = %d, want 1000", mm.SystemPrompt)
	}
	if mm.ResponseBuffer != 3000 {
		t.Errorf("ResponseBuffer = %d, want 3000", mm.ResponseBuffer)
	}
	expectedRaw := 30000 - 1000 - 3000 - 3000 - 5000
	if mm.RawData != expectedRaw {
		t.Errorf("RawData = %d, want %d", mm.RawData, expectedRaw)
	}
}
