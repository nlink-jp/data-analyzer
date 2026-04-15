package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nlink-jp/data-analyzer/internal/config"
	"github.com/nlink-jp/data-analyzer/internal/job"
	"github.com/nlink-jp/data-analyzer/internal/llm"
	"github.com/nlink-jp/data-analyzer/internal/prompt"
	"github.com/nlink-jp/data-analyzer/internal/reader"
	"github.com/nlink-jp/data-analyzer/internal/types"
	"github.com/nlink-jp/data-analyzer/internal/window"
	"github.com/spf13/cobra"
)

var (
	flagParams  string
	flagResume  string
	flagOutput  string
	flagModel   string
	flagLang    string
)

func newAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze [flags] <input...>",
		Short: "Run sliding window analysis on JSON/JSONL data",
		Long: `Analyze large-scale JSON/JSONL data using sliding window + progressive
summarization. Requires a parameter file from 'prepare' or manual creation.

Input can be files, directories, or stdin (use - for stdin).`,
		Args: cobra.MinimumNArgs(1),
		RunE: runAnalyze,
	}

	cmd.Flags().StringVarP(&flagParams, "params", "p", "", "parameter file path or base64-encoded JSON")
	cmd.Flags().StringVar(&flagResume, "resume", "", "resume job by ID")
	cmd.Flags().StringVarP(&flagOutput, "output", "o", "", "output file (default: stdout)")
	cmd.Flags().StringVarP(&flagModel, "model", "m", "", "model name override")
	cmd.Flags().StringVar(&flagLang, "lang", "", "output language (e.g., Japanese, English)")

	_ = cmd.MarkFlagRequired("params")

	return cmd
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	// 1. Load config
	cfg, err := config.Load(flagConfigPath)
	if err != nil {
		return exitWithCode(fmt.Errorf("config: %w", err), exitInputError)
	}
	cfg.ApplyFlags(flagModel)

	// 2. Load analysis parameters
	params, err := loadParams(flagParams)
	if err != nil {
		return exitWithCode(fmt.Errorf("params: %w", err), exitInputError)
	}

	// Apply lang: CLI flag > params file > config
	if flagLang != "" {
		params.Lang = flagLang
	} else if params.Lang == "" && cfg.Analysis.Lang != "" {
		params.Lang = cfg.Analysis.Lang
	}

	// 3. Read input records
	records, err := reader.ReadAll(args)
	if err != nil {
		return exitWithCode(fmt.Errorf("reading input: %w", err), exitInputError)
	}

	if len(records) == 0 {
		return exitWithCode(fmt.Errorf("no records found in input"), exitInputError)
	}

	fmt.Fprintf(os.Stderr, "Loaded %d records from %d source(s)\n", len(records), countSources(records))

	// 4. Create job manager
	mgr, err := job.NewManager(cfg.Job.TempDir)
	if err != nil {
		return exitWithCode(fmt.Errorf("job manager: %w", err), exitGeneralError)
	}

	// 5. Check idempotency or resume
	jobID, state, err := mgr.ResolveJob(flagResume, params, args)
	if err != nil {
		return exitWithCode(fmt.Errorf("job: %w", err), exitGeneralError)
	}

	if result, ok := mgr.GetResult(jobID); ok {
		fmt.Fprintf(os.Stderr, "Job %s already completed, returning cached result\n", jobID)
		return writeResult(result, flagOutput)
	}

	// 6. Create LLM client
	client := llm.NewHTTPClient(cfg.API.Endpoint, cfg.API.Model, cfg.API.APIKey)

	// 7. Create prompt builder
	builder := prompt.NewBuilder(params)

	// 8. Run analysis engine with cancellation support
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	engine := window.NewEngine(client, builder, mgr, &window.EngineConfig{
		ContextLimit: cfg.Analysis.ContextLimit,
		OverlapRatio: cfg.Analysis.OverlapRatio,
		MaxFindings:  cfg.Analysis.MaxFindings,
		JobID:        jobID,
		Params:       params,
		Stderr:       os.Stderr,
		Debug:        flagDebug,
	})

	result, err := engine.Run(ctx, records, state)
	if err != nil {
		return exitWithCode(fmt.Errorf("analysis: %w", err), exitAPIError)
	}

	// 9. Save result for idempotency
	if err := mgr.SaveResult(jobID, result); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save result: %v\n", err)
	}

	return writeResult(result, flagOutput)
}

func loadParams(path string) (*types.AnalysisParam, error) {
	var data []byte
	var err error

	// Try base64 decode first
	if decoded, decErr := base64.StdEncoding.DecodeString(path); decErr == nil && json.Valid(decoded) {
		data = decoded
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading params file: %w", err)
		}
	}

	var params types.AnalysisParam
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}

	if params.Perspective == "" {
		return nil, fmt.Errorf("perspective is required in params")
	}

	return &params, nil
}

func writeResult(result *types.AnalysisResult, output string) error {
	var f *os.File
	if output != "" {
		var err error
		f, err = os.Create(output)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
	} else {
		f = os.Stdout
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func countSources(records []types.Record) int {
	seen := make(map[string]struct{})
	for _, r := range records {
		seen[r.Source] = struct{}{}
	}
	return len(seen)
}
