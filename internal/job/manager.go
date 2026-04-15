// Package job manages analysis job lifecycle: ID generation, checkpoints, and idempotency.
package job

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

// Manager handles job creation, checkpoint persistence, and resume.
type Manager struct {
	baseDir string
}

// NewManager creates a job manager with the given temp directory.
// If tempDir is empty, uses os.TempDir()/data-analyzer.
func NewManager(tempDir string) (*Manager, error) {
	if tempDir == "" {
		tempDir = filepath.Join(os.TempDir(), "data-analyzer")
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("creating job directory: %w", err)
	}

	return &Manager{baseDir: tempDir}, nil
}

// ResolveJob determines the job ID and returns any existing checkpoint state.
// If resumeID is provided, it loads that job. Otherwise, generates a new ID.
func (m *Manager) ResolveJob(resumeID string, params *types.AnalysisParam, inputs []string) (string, *types.WindowState, error) {
	if resumeID != "" {
		state, err := m.LoadLatestCheckpoint(resumeID)
		if err != nil {
			return "", nil, fmt.Errorf("loading checkpoint for %s: %w", resumeID, err)
		}
		return resumeID, state, nil
	}

	jobID := m.generateID(params, inputs)

	// Check for existing checkpoint
	state, err := m.LoadLatestCheckpoint(jobID)
	if err == nil && state != nil {
		return jobID, state, nil
	}

	// Create job directory
	jobDir := filepath.Join(m.baseDir, jobID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return "", nil, fmt.Errorf("creating job dir: %w", err)
	}

	// Save job metadata
	meta := map[string]any{
		"params":     params,
		"inputs":     inputs,
		"created_at": time.Now().UTC(),
	}
	metaPath := filepath.Join(jobDir, "job.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshaling job metadata: %w", err)
	}
	if err := writeAtomic(metaPath, data); err != nil {
		return "", nil, fmt.Errorf("writing job metadata: %w", err)
	}

	return jobID, nil, nil
}

// SaveCheckpoint persists the current window state for resume.
func (m *Manager) SaveCheckpoint(jobID string, state *types.WindowState) error {
	jobDir := filepath.Join(m.baseDir, jobID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("creating job dir: %w", err)
	}

	path := filepath.Join(jobDir, fmt.Sprintf("window_%03d.json", state.WindowIndex))
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling checkpoint: %w", err)
	}

	return writeAtomic(path, data)
}

// LoadLatestCheckpoint loads the most recent checkpoint for a job.
// Returns nil, nil if no checkpoints exist.
func (m *Manager) LoadLatestCheckpoint(jobID string) (*types.WindowState, error) {
	jobDir := filepath.Join(m.baseDir, jobID)

	entries, err := os.ReadDir(jobDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading job dir: %w", err)
	}

	// Find the highest-numbered window checkpoint
	var checkpoints []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "window_") && strings.HasSuffix(e.Name(), ".json") {
			checkpoints = append(checkpoints, e.Name())
		}
	}

	if len(checkpoints) == 0 {
		return nil, nil
	}

	sort.Strings(checkpoints)
	latest := checkpoints[len(checkpoints)-1]

	data, err := os.ReadFile(filepath.Join(jobDir, latest))
	if err != nil {
		return nil, fmt.Errorf("reading checkpoint: %w", err)
	}

	var state types.WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing checkpoint: %w", err)
	}

	return &state, nil
}

// SaveResult saves the final result, marking the job as complete.
func (m *Manager) SaveResult(jobID string, result *types.AnalysisResult) error {
	jobDir := filepath.Join(m.baseDir, jobID)
	path := filepath.Join(jobDir, "result.json")

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	return writeAtomic(path, data)
}

// GetResult checks if a job has a completed result.
func (m *Manager) GetResult(jobID string) (*types.AnalysisResult, bool) {
	jobDir := filepath.Join(m.baseDir, jobID)
	path := filepath.Join(jobDir, "result.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var result types.AnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}

	return &result, true
}

func (m *Manager) generateID(params *types.AnalysisParam, inputs []string) string {
	h := sha256.New()
	data, _ := json.Marshal(params)
	h.Write(data)
	for _, input := range inputs {
		h.Write([]byte(input))
	}
	hash := fmt.Sprintf("%x", h.Sum(nil))[:12]
	ts := time.Now().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s-%s", ts, hash)
}

// writeAtomic writes data to a temp file then renames for atomicity.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}
