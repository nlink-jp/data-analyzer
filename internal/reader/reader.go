// Package reader provides JSON/JSONL input reading from files, directories, and stdin.
package reader

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

// ReadAll reads records from the given paths.
// Paths can be files, directories, or "-" for stdin.
func ReadAll(paths []string) ([]types.Record, error) {
	var records []types.Record
	index := 0

	for _, p := range paths {
		if p == "-" {
			recs, err := readStream(os.Stdin, "stdin", &index)
			if err != nil {
				return nil, fmt.Errorf("reading stdin: %w", err)
			}
			records = append(records, recs...)
			continue
		}

		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if info.IsDir() {
			recs, err := readDir(p, &index)
			if err != nil {
				return nil, err
			}
			records = append(records, recs...)
		} else {
			recs, err := readFile(p, &index)
			if err != nil {
				return nil, err
			}
			records = append(records, recs...)
		}
	}

	return records, nil
}

func readDir(dir string, index *int) ([]types.Record, error) {
	var records []types.Record

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isJSONFile(name) {
			continue
		}
		recs, err := readFile(filepath.Join(dir, name), index)
		if err != nil {
			return nil, err
		}
		records = append(records, recs...)
	}

	return records, nil
}

func readFile(path string, index *int) ([]types.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	return readStream(f, path, index)
}

func readStream(r io.Reader, source string, index *int) ([]types.Record, error) {
	// Peek at the first non-whitespace byte to determine format
	br := bufio.NewReader(r)

	// Skip leading whitespace
	for {
		b, err := br.ReadByte()
		if err != nil {
			return nil, nil // empty input
		}
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if err := br.UnreadByte(); err != nil {
			return nil, fmt.Errorf("unread: %w", err)
		}

		if b == '[' {
			return readJSONArray(br, source, index)
		}
		return readJSONL(br, source, index)
	}
}

func readJSONArray(r io.Reader, source string, index *int) ([]types.Record, error) {
	var raw []json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parsing JSON array from %s: %w", source, err)
	}

	records := make([]types.Record, 0, len(raw))
	for _, item := range raw {
		records = append(records, types.Record{
			Index:   *index,
			Source:  source,
			RawJSON: item,
		})
		*index++
	}

	return records, nil
}

func readJSONL(r io.Reader, source string, index *int) ([]types.Record, error) {
	var records []types.Record
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB per line

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		raw := json.RawMessage(line)
		if !json.Valid(raw) {
			return nil, fmt.Errorf("invalid JSON at line in %s: %.100s", source, line)
		}

		records = append(records, types.Record{
			Index:   *index,
			Source:  source,
			RawJSON: raw,
		})
		*index++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning %s: %w", source, err)
	}

	return records, nil
}

func isJSONFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".jsonl")
}
