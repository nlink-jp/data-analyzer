package reader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAllJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"a":1}
{"a":2}
{"a":3}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	records, err := ReadAll([]string{path})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}

	for i, r := range records {
		if r.Index != i {
			t.Errorf("records[%d].Index = %d, want %d", i, r.Index, i)
		}
		if r.Source != path {
			t.Errorf("records[%d].Source = %q, want %q", i, r.Source, path)
		}
	}
}

func TestReadAllJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `[{"a":1},{"a":2}]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	records, err := ReadAll([]string{path})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
}

func TestReadAllDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create 2 JSONL files
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"x":1}`+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Non-JSON file should be skipped
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("skip me"), 0644); err != nil {
		t.Fatal(err)
	}

	records, err := ReadAll([]string{dir})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	// Indices should be sequential across files
	if records[0].Index != 0 || records[1].Index != 1 {
		t.Errorf("indices = [%d, %d], want [0, 1]", records[0].Index, records[1].Index)
	}
}

func TestReadAllEmptyInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	records, err := ReadAll([]string{path})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
}

func TestReadAllInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadAll([]string{path})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadAllMultipleSources(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.jsonl")
	p2 := filepath.Join(dir, "b.json")

	if err := os.WriteFile(p1, []byte(`{"x":1}`+"\n"+`{"x":2}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte(`[{"y":3}]`), 0644); err != nil {
		t.Fatal(err)
	}

	records, err := ReadAll([]string{p1, p2})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}

	// Check sequential indices across sources
	for i, r := range records {
		if r.Index != i {
			t.Errorf("records[%d].Index = %d, want %d", i, r.Index, i)
		}
	}
}
