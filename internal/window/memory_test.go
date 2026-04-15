package window

import "testing"

func TestComputeMemoryMapDefaults(t *testing.T) {
	mm := ComputeMemoryMap(131072, 0, 0)

	if mm.SystemPrompt != systemReserve {
		t.Errorf("SystemPrompt = %d, want %d", mm.SystemPrompt, systemReserve)
	}
	if mm.ResponseBuffer != responseReserve {
		t.Errorf("ResponseBuffer = %d, want %d", mm.ResponseBuffer, responseReserve)
	}
	if mm.PreviousSummary != 0 {
		t.Errorf("PreviousSummary = %d, want 0", mm.PreviousSummary)
	}
	if mm.Findings != 0 {
		t.Errorf("Findings = %d, want 0", mm.Findings)
	}

	expectedRaw := 131072 - systemReserve - responseReserve
	if mm.RawData != expectedRaw {
		t.Errorf("RawData = %d, want %d", mm.RawData, expectedRaw)
	}
}

func TestComputeMemoryMapWithSummaryAndFindings(t *testing.T) {
	mm := ComputeMemoryMap(131072, 10000, 15000)

	if mm.PreviousSummary != 10000 {
		t.Errorf("PreviousSummary = %d, want 10000", mm.PreviousSummary)
	}
	if mm.Findings != 15000 {
		t.Errorf("Findings = %d, want 15000", mm.Findings)
	}

	expectedRaw := 131072 - systemReserve - responseReserve - 10000 - 15000
	if mm.RawData != expectedRaw {
		t.Errorf("RawData = %d, want %d", mm.RawData, expectedRaw)
	}
}

func TestComputeMemoryMapCapsSummary(t *testing.T) {
	mm := ComputeMemoryMap(131072, 50000, 0)

	if mm.PreviousSummary != maxSummary {
		t.Errorf("PreviousSummary = %d, want %d (capped)", mm.PreviousSummary, maxSummary)
	}
}

func TestComputeMemoryMapCapsFindings(t *testing.T) {
	mm := ComputeMemoryMap(131072, 0, 50000)

	if mm.Findings != maxFindingsBudget {
		t.Errorf("Findings = %d, want %d (capped)", mm.Findings, maxFindingsBudget)
	}
}

func TestComputeMemoryMapReducesFindingsWhenTight(t *testing.T) {
	// contextLimit=50000, available=50000-2000-5000=43000
	// summary=15000 (capped at 15000), findings=20000
	// rawData = 43000-15000-20000 = 8000 < 10000
	// → findings reduced to 43000-15000-10000 = 18000, rawData = 10000
	mm := ComputeMemoryMap(50000, 15000, 20000)

	if mm.RawData < minRawData {
		t.Errorf("RawData = %d, should be >= %d", mm.RawData, minRawData)
	}
	if mm.PreviousSummary != maxSummary {
		t.Errorf("PreviousSummary = %d, want %d", mm.PreviousSummary, maxSummary)
	}
	// Findings should be reduced from 20000
	if mm.Findings >= 20000 {
		t.Errorf("Findings = %d, should be reduced", mm.Findings)
	}
}
