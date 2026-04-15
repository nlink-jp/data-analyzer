package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nlink-jp/data-analyzer/internal/compile"
	"github.com/nlink-jp/data-analyzer/internal/types"
	"github.com/spf13/cobra"
)

var (
	flagCompileFormat string
	flagCompileOutput string
)

func newCompileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile [flags] <result.json>",
		Short: "Render analysis results to Markdown or HTML",
		Long: `Render a JSON analysis result (from 'analyze') into a human-readable
report format.

Formats:
  md    — Markdown (default, can output to stdout)
  html  — Self-contained HTML with embedded CSS
  both  — Output both .md and .html (requires --output base name)

Input can be a file path or stdin (use - for stdin).`,
		Args: cobra.ExactArgs(1),
		RunE: runCompile,
	}

	cmd.Flags().StringVarP(&flagCompileFormat, "format", "f", "md", "output format: md, html, both")
	cmd.Flags().StringVarP(&flagCompileOutput, "output", "o", "", "output file (default: stdout)")

	return cmd
}

func runCompile(cmd *cobra.Command, args []string) error {
	// Read input
	var data []byte
	var err error

	if args[0] == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(args[0])
	}
	if err != nil {
		return exitWithCode(fmt.Errorf("reading input: %w", err), exitInputError)
	}

	var result types.AnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return exitWithCode(fmt.Errorf("parsing result JSON: %w", err), exitInputError)
	}

	switch flagCompileFormat {
	case "md", "markdown":
		return renderToOutput(&result, flagCompileOutput, compile.Markdown)

	case "html":
		return renderToOutput(&result, flagCompileOutput, compile.HTML)

	case "both":
		if flagCompileOutput == "" {
			return exitWithCode(fmt.Errorf("--output is required for 'both' format (used as base name)"), exitInputError)
		}
		base := strings.TrimSuffix(strings.TrimSuffix(flagCompileOutput, ".md"), ".html")
		if err := renderToFile(&result, base+".md", compile.Markdown); err != nil {
			return err
		}
		if err := renderToFile(&result, base+".html", compile.HTML); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Generated %s.md and %s.html\n", base, base)
		return nil

	default:
		return exitWithCode(fmt.Errorf("unknown format: %s (supported: md, html, both)", flagCompileFormat), exitInputError)
	}
}

type renderFunc func(io.Writer, *types.AnalysisResult) error

func renderToOutput(result *types.AnalysisResult, output string, fn renderFunc) error {
	if output != "" {
		return renderToFile(result, output, fn)
	}
	return fn(os.Stdout, result)
}

func renderToFile(result *types.AnalysisResult, path string, fn renderFunc) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if err := fn(f, result); err != nil {
		return fmt.Errorf("rendering %s: %w", path, err)
	}
	return nil
}
