// Package prepare implements the interactive analysis parameter builder.
package prepare

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nlink-jp/data-analyzer/internal/llm"
	"github.com/nlink-jp/data-analyzer/internal/types"
	"github.com/nlink-jp/nlk/jsonfix"
	"github.com/nlink-jp/nlk/strip"
)

const compileSystemPrompt = `You are an analysis parameter compiler. The user will describe what they want to analyze in natural language.

Your job is to produce a structured JSON parameter file for a data analysis tool.

## Required Output Format
{
  "perspective": "A clear, specific analysis perspective statement",
  "target_fields": ["field1", "field2"],
  "attention_points": ["point1", "point2"],
  "user_findings": []
}

## Rules
1. "perspective" must be a clear, actionable analysis directive
2. "target_fields" should list JSON field names the analysis should focus on
3. "attention_points" should list specific patterns or behaviors to watch for
4. "user_findings" should be empty unless the user specifies findings to track
5. Output ONLY valid JSON — no other text`

const refineSystemPrompt = `You are an analysis parameter compiler. The user wants to refine the analysis parameters below.

## Current Parameters
%s

## Rules
1. Apply the user's requested changes to the parameters
2. Keep unchanged fields as-is
3. Output the complete updated parameter set as JSON
4. Output ONLY valid JSON — no other text`

const maxSampleRecords = 5

// Session manages an interactive parameter-building conversation.
type Session struct {
	client  llm.Client
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	samples []types.Record
}

// NewSession creates a new interactive session.
func NewSession(client llm.Client, stdin io.Reader, stdout, stderr io.Writer) *Session {
	return &Session{
		client: client,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
}

// SetSampleRecords sets sample data records for field discovery.
// Only the first few records are used to keep the prompt compact.
func (s *Session) SetSampleRecords(records []types.Record) {
	if len(records) > maxSampleRecords {
		records = records[:maxSampleRecords]
	}
	s.samples = records
}

// Run executes the interactive parameter building loop.
// Returns the finalized AnalysisParam.
func (s *Session) Run(ctx context.Context) (*types.AnalysisParam, error) {
	return s.RunWithInput(ctx, "")
}

// RunWithInput executes the parameter building loop.
// If initialInput is non-empty, it is used as the initial description
// (skipping the interactive prompt), then enters the refine loop.
func (s *Session) RunWithInput(ctx context.Context, initialInput string) (*types.AnalysisParam, error) {
	scanner := bufio.NewScanner(s.stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB per line

	var description string

	if initialInput != "" {
		// Use file input as description
		description = strings.TrimSpace(initialInput)
		fmt.Fprintln(s.stdout, "=== data-analyzer: Analysis Parameter Builder ===")
		fmt.Fprintln(s.stdout)
		fmt.Fprintf(s.stdout, "Loaded initial requirements (%d chars)\n", len(description))
	} else {
		// Interactive input
		fmt.Fprintln(s.stdout, "=== data-analyzer: Analysis Parameter Builder ===")
		fmt.Fprintln(s.stdout)
		fmt.Fprintln(s.stdout, "Describe what you want to analyze.")
		fmt.Fprintln(s.stdout, "You can paste multiple lines. End with an empty line.")
		fmt.Fprintln(s.stdout)
		fmt.Fprint(s.stdout, "> ")

		var err error
		description, err = readMultiLine(scanner)
		if err != nil {
			return nil, err
		}
	}

	if description == "" {
		return nil, fmt.Errorf("empty description")
	}

	// Compile initial parameters
	fmt.Fprintln(s.stderr, "Compiling analysis parameters...")

	sysPrompt := s.buildCompilePrompt()
	response, err := s.client.Chat(ctx, sysPrompt, description)
	if err != nil {
		return nil, fmt.Errorf("LLM compilation: %w", err)
	}

	params, err := parseParams(response)
	if err != nil {
		return nil, fmt.Errorf("parsing compiled parameters: %w", err)
	}

	// Step 3: Review and refine loop
	for {
		s.displayParams(params)
		fmt.Fprintln(s.stdout)
		fmt.Fprintln(s.stdout, "Options:")
		fmt.Fprintln(s.stdout, "  [Enter] Accept and save")
		fmt.Fprintln(s.stdout, "  Type changes to refine (e.g., 'add field: user_agent')")
		fmt.Fprintln(s.stdout, "  'quit' to cancel")
		fmt.Fprint(s.stdout, "> ")

		input, err := readMultiLine(scanner)
		if err != nil {
			return nil, err
		}
		input = strings.TrimSpace(input)

		if input == "" {
			// Accept
			break
		}
		if input == "quit" || input == "q" {
			return nil, fmt.Errorf("cancelled by user")
		}

		// Refine
		fmt.Fprintln(s.stderr, "Refining parameters...")

		currentJSON, _ := json.MarshalIndent(params, "", "  ")
		sysPrompt := s.buildRefinePrompt(string(currentJSON))

		response, err := s.client.Chat(ctx, sysPrompt, input)
		if err != nil {
			fmt.Fprintf(s.stderr, "Warning: refinement failed: %v\n", err)
			continue
		}

		refined, err := parseParams(response)
		if err != nil {
			fmt.Fprintf(s.stderr, "Warning: failed to parse refined parameters: %v\n", err)
			continue
		}

		params = refined
	}

	return params, nil
}

// readMultiLine reads lines until an empty line or EOF.
// Returns all lines joined with newlines.
func readMultiLine(scanner *bufio.Scanner) (string, error) {
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && len(lines) > 0 {
			// Empty line after content → end of input
			break
		}
		if line == "" && len(lines) == 0 {
			// Leading empty line with no content → end (accept/enter)
			return "", nil
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Session) displayParams(params *types.AnalysisParam) {
	fmt.Fprintln(s.stdout)
	fmt.Fprintln(s.stdout, "--- Generated Parameters ---")
	fmt.Fprintf(s.stdout, "Perspective: %s\n", params.Perspective)

	if len(params.TargetFields) > 0 {
		fmt.Fprintf(s.stdout, "Target Fields: %s\n", strings.Join(params.TargetFields, ", "))
	}

	if len(params.AttentionPoints) > 0 {
		fmt.Fprintln(s.stdout, "Attention Points:")
		for _, p := range params.AttentionPoints {
			fmt.Fprintf(s.stdout, "  - %s\n", p)
		}
	}

	if len(params.UserFindings) > 0 {
		fmt.Fprintln(s.stdout, "User Findings:")
		for _, f := range params.UserFindings {
			fmt.Fprintf(s.stdout, "  - %s\n", f)
		}
	}
}

func (s *Session) buildRefinePrompt(currentParamsJSON string) string {
	base := fmt.Sprintf(refineSystemPrompt, currentParamsJSON)
	if len(s.samples) == 0 {
		return base
	}
	return base + "\n\n" + s.sampleDataSection()
}

// buildCompilePrompt constructs the system prompt, including sample data if available.
func (s *Session) buildCompilePrompt() string {
	if len(s.samples) == 0 {
		return compileSystemPrompt
	}
	return compileSystemPrompt + "\n\n" + s.sampleDataSection()
}

func (s *Session) sampleDataSection() string {
	var sb strings.Builder
	sb.WriteString("## Sample Data Records\n")
	sb.WriteString("The following are actual records from the dataset. Use these to identify\n")
	sb.WriteString("relevant field names for target_fields and realistic attention_points.\n\n")

	for i, r := range s.samples {
		sb.WriteString(fmt.Sprintf("Record %d:\n```json\n%s\n```\n\n", i+1, string(r.RawJSON)))
	}

	return sb.String()
}

func parseParams(text string) (*types.AnalysisParam, error) {
	cleaned := strip.ThinkTags(text)

	var params types.AnalysisParam
	if err := jsonfix.ExtractTo(cleaned, &params); err != nil {
		return nil, fmt.Errorf("JSON extraction: %w (raw: %.200s)", err, text)
	}

	if params.Perspective == "" {
		return nil, fmt.Errorf("perspective is empty")
	}

	return &params, nil
}
