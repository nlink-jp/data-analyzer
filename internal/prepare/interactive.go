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

// Session manages an interactive parameter-building conversation.
type Session struct {
	client llm.Client
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
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

// Run executes the interactive parameter building loop.
// Returns the finalized AnalysisParam.
func (s *Session) Run(ctx context.Context) (*types.AnalysisParam, error) {
	scanner := bufio.NewScanner(s.stdin)

	fmt.Fprintln(s.stdout, "=== data-analyzer: Analysis Parameter Builder ===")
	fmt.Fprintln(s.stdout)
	fmt.Fprintln(s.stdout, "Describe what you want to analyze. Include:")
	fmt.Fprintln(s.stdout, "  - What kind of data you have (access logs, operation logs, etc.)")
	fmt.Fprintln(s.stdout, "  - What you're looking for (anomalies, patterns, threats, etc.)")
	fmt.Fprintln(s.stdout, "  - Any specific fields or indicators to focus on")
	fmt.Fprintln(s.stdout)
	fmt.Fprint(s.stdout, "> ")

	// Step 1: Get initial description
	if !scanner.Scan() {
		return nil, fmt.Errorf("no input received")
	}
	description := scanner.Text()
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("empty description")
	}

	// Step 2: Compile initial parameters
	fmt.Fprintln(s.stderr, "Compiling analysis parameters...")

	response, err := s.client.Chat(ctx, compileSystemPrompt, description)
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

		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

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
		sysPrompt := fmt.Sprintf(refineSystemPrompt, string(currentJSON))

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
