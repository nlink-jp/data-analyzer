package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/nlink-jp/data-analyzer/internal/config"
	"github.com/nlink-jp/data-analyzer/internal/job"
	"github.com/spf13/cobra"
)

var (
	flagCleanAll    bool
	flagCleanMaxAge string
)

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove old job caches and checkpoints",
		Long: `Clean up completed job caches from the temp directory.

By default, removes completed jobs older than 7 days.
Use --all to remove all jobs, or --max-age to specify a custom age.`,
		RunE: runClean,
	}

	cmd.Flags().BoolVar(&flagCleanAll, "all", false, "remove all jobs (including incomplete)")
	cmd.Flags().StringVar(&flagCleanMaxAge, "max-age", "7d", "remove completed jobs older than this (e.g., 1d, 12h, 30d)")

	return cmd
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfigPath)
	if err != nil {
		return exitWithCode(fmt.Errorf("config: %w", err), exitInputError)
	}

	mgr, err := job.NewManager(cfg.Job.TempDir)
	if err != nil {
		return exitWithCode(fmt.Errorf("job manager: %w", err), exitGeneralError)
	}

	if flagCleanAll {
		jobs, err := mgr.ListJobs()
		if err != nil {
			return fmt.Errorf("listing jobs: %w", err)
		}
		removed := 0
		for _, j := range jobs {
			if err := mgr.RemoveJob(j.ID); err == nil {
				fmt.Fprintf(os.Stderr, "Removed %s (%.1f KB)\n", j.ID, float64(j.Size)/1024)
				removed++
			}
		}
		fmt.Fprintf(os.Stderr, "Removed %d job(s)\n", removed)
		return nil
	}

	maxAge, err := parseDuration(flagCleanMaxAge)
	if err != nil {
		return exitWithCode(fmt.Errorf("invalid --max-age: %w", err), exitInputError)
	}

	removed, err := mgr.CleanOldJobs(maxAge)
	if err != nil {
		return fmt.Errorf("cleaning jobs: %w", err)
	}

	if removed == 0 {
		fmt.Fprintln(os.Stderr, "No old jobs to clean")
	} else {
		fmt.Fprintf(os.Stderr, "Removed %d completed job(s) older than %s\n", removed, flagCleanMaxAge)
	}

	return nil
}

// parseDuration parses a human-friendly duration string (e.g., "7d", "12h", "30d").
func parseDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	last := s[len(s)-1]
	numStr := s[:len(s)-1]

	switch last {
	case 'd':
		var days int
		if _, err := fmt.Sscanf(numStr, "%d", &days); err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	default:
		// Fall back to Go's time.ParseDuration for h, m, s
		return time.ParseDuration(s)
	}
}
