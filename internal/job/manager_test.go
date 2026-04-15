package job

import (
	"testing"
	"time"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.baseDir != dir {
		t.Errorf("baseDir = %q, want %q", mgr.baseDir, dir)
	}
}

func TestResolveJobCreatesNew(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewManager(dir)

	params := &types.AnalysisParam{Perspective: "test"}
	jobID, state, err := mgr.ResolveJob("", params, []string{"input.jsonl"})
	if err != nil {
		t.Fatalf("ResolveJob: %v", err)
	}
	if jobID == "" {
		t.Error("jobID is empty")
	}
	if state != nil {
		t.Error("state should be nil for new job")
	}
}

func TestCheckpointSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewManager(dir)

	params := &types.AnalysisParam{Perspective: "test"}
	jobID, _, _ := mgr.ResolveJob("", params, []string{"input.jsonl"})

	// Save checkpoints
	state0 := &types.WindowState{
		WindowIndex:  0,
		RecordOffset: 100,
		Summary:      "summary after window 0",
		ProcessedAt:  time.Now().UTC(),
	}
	if err := mgr.SaveCheckpoint(jobID, state0); err != nil {
		t.Fatalf("SaveCheckpoint(0): %v", err)
	}

	state1 := &types.WindowState{
		WindowIndex:  1,
		RecordOffset: 200,
		Summary:      "summary after window 1",
		ProcessedAt:  time.Now().UTC(),
	}
	if err := mgr.SaveCheckpoint(jobID, state1); err != nil {
		t.Fatalf("SaveCheckpoint(1): %v", err)
	}

	// Load latest checkpoint — should be window 1
	loaded, err := mgr.LoadLatestCheckpoint(jobID)
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}
	if loaded.WindowIndex != 1 {
		t.Errorf("WindowIndex = %d, want 1", loaded.WindowIndex)
	}
	if loaded.RecordOffset != 200 {
		t.Errorf("RecordOffset = %d, want 200", loaded.RecordOffset)
	}
}

func TestResumeFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewManager(dir)

	params := &types.AnalysisParam{Perspective: "test"}
	jobID, _, _ := mgr.ResolveJob("", params, []string{"input.jsonl"})

	state := &types.WindowState{
		WindowIndex:  2,
		RecordOffset: 300,
		Summary:      "partial summary",
		ProcessedAt:  time.Now().UTC(),
	}
	mgr.SaveCheckpoint(jobID, state)

	// Resume should load checkpoint
	_, resumeState, err := mgr.ResolveJob(jobID, params, []string{"input.jsonl"})
	if err != nil {
		t.Fatalf("ResolveJob(resume): %v", err)
	}
	if resumeState == nil {
		t.Fatal("expected checkpoint state on resume")
	}
	if resumeState.WindowIndex != 2 {
		t.Errorf("resumed WindowIndex = %d, want 2", resumeState.WindowIndex)
	}
}

func TestSaveAndGetResult(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewManager(dir)

	params := &types.AnalysisParam{Perspective: "test"}
	jobID, _, _ := mgr.ResolveJob("", params, []string{"input.jsonl"})

	result := &types.AnalysisResult{
		JobID:        jobID,
		Summary:      "final summary",
		TotalRecords: 1000,
		WindowsUsed:  5,
	}

	if err := mgr.SaveResult(jobID, result); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	loaded, ok := mgr.GetResult(jobID)
	if !ok {
		t.Fatal("GetResult returned false")
	}
	if loaded.Summary != "final summary" {
		t.Errorf("Summary = %q, want %q", loaded.Summary, "final summary")
	}
	if loaded.TotalRecords != 1000 {
		t.Errorf("TotalRecords = %d, want 1000", loaded.TotalRecords)
	}
}

func TestIdempotency(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewManager(dir)

	params := &types.AnalysisParam{Perspective: "test"}
	jobID, _, _ := mgr.ResolveJob("", params, []string{"input.jsonl"})

	result := &types.AnalysisResult{JobID: jobID, Summary: "done"}
	mgr.SaveResult(jobID, result)

	// Check that result exists
	_, ok := mgr.GetResult(jobID)
	if !ok {
		t.Error("completed job should have result")
	}
}

func TestNoCheckpointReturnsNil(t *testing.T) {
	dir := t.TempDir()
	mgr, _ := NewManager(dir)

	state, err := mgr.LoadLatestCheckpoint("nonexistent-job")
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for nonexistent job")
	}
}
