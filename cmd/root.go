// Package cmd implements the data-analyzer CLI.
package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var version string

// CLI flags
var (
	flagConfigPath string
	flagDebug      bool
)

// Exit codes
const (
	exitOK           = 0
	exitGeneralError = 1
	exitInputError   = 2
	exitAPIError     = 3
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data-analyzer",
		Short: "Large-scale JSON/JSONL data analysis using local LLMs",
		Long: `data-analyzer analyzes large-scale log data (JSON/JSONL) using local LLMs
via OpenAI-compatible API. It uses a sliding window + progressive summarization
approach to overcome context window limitations.

Subcommands:
  prepare  — Interactively build analysis parameters
  analyze  — Run sliding window analysis on data
  compile  — Render analysis results to Markdown/HTML`,
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringVarP(&flagConfigPath, "config", "c", "", "config file path")
	cmd.PersistentFlags().BoolVar(&flagDebug, "debug", false, "enable debug output")

	cmd.AddCommand(newAnalyzeCmd())
	cmd.AddCommand(newPrepareCmd())
	cmd.AddCommand(newCompileCmd())
	cmd.AddCommand(newCleanCmd())

	return cmd
}

// Execute runs the root command.
func Execute(v string) {
	version = v
	cmd := newRootCmd()
	cmd.Version = version

	if err := cmd.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		os.Exit(exitGeneralError)
	}
}

type exitError struct {
	err  error
	code int
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func exitWithCode(err error, code int) error {
	return &exitError{err: err, code: code}
}
