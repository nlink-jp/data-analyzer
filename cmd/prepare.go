package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nlink-jp/data-analyzer/internal/config"
	"github.com/nlink-jp/data-analyzer/internal/llm"
	"github.com/nlink-jp/data-analyzer/internal/prepare"
	"github.com/spf13/cobra"
)

var (
	flagPrepareOutput string
	flagPrepareInput  string
)

func newPrepareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Interactively build analysis parameters",
		Long: `Interactively build analysis parameters through a conversation with the LLM.
The generated parameter file can be used with the 'analyze' subcommand.

Use --input to load initial requirements from a file, then refine interactively.`,
		RunE: runPrepare,
	}

	cmd.Flags().StringVarP(&flagPrepareOutput, "output", "o", "", "output file for parameter JSON (default: stdout)")
	cmd.Flags().StringVarP(&flagPrepareInput, "input", "i", "", "load initial requirements from file")

	return cmd
}

func runPrepare(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfigPath)
	if err != nil {
		return exitWithCode(fmt.Errorf("config: %w", err), exitInputError)
	}
	cfg.ApplyFlags(flagModel)

	client := llm.NewHTTPClient(cfg.API.Endpoint, cfg.API.Model, cfg.API.APIKey)

	session := prepare.NewSession(client, os.Stdin, os.Stdout, os.Stderr)

	// Load initial requirements from file if specified
	var initialInput string
	if flagPrepareInput != "" {
		data, err := os.ReadFile(flagPrepareInput)
		if err != nil {
			return exitWithCode(fmt.Errorf("reading input file: %w", err), exitInputError)
		}
		initialInput = string(data)
		fmt.Fprintf(os.Stderr, "Loaded requirements from %s (%d bytes)\n", flagPrepareInput, len(data))
	}

	ctx := cmd.Context()
	params, err := session.RunWithInput(ctx, initialInput)
	if err != nil {
		return exitWithCode(fmt.Errorf("prepare: %w", err), exitGeneralError)
	}

	data, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling params: %w", err)
	}

	if flagPrepareOutput != "" {
		if err := os.WriteFile(flagPrepareOutput, data, 0644); err != nil {
			return fmt.Errorf("writing params file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Parameters saved to %s\n", flagPrepareOutput)
	} else {
		fmt.Println(string(data))
	}

	return nil
}
