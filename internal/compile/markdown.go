// Package compile renders analysis results into report formats.
package compile

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

// Markdown renders an AnalysisResult as a Markdown report.
func Markdown(w io.Writer, result *types.AnalysisResult) error {
	fmt.Fprintf(w, "# Analysis Report\n\n")

	// Metadata
	fmt.Fprintf(w, "- **Job ID:** %s\n", result.JobID)
	fmt.Fprintf(w, "- **Total Records:** %d\n", result.TotalRecords)
	fmt.Fprintf(w, "- **Windows Used:** %d\n", result.WindowsUsed)
	fmt.Fprintf(w, "- **Started:** %s\n", result.StartedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(w, "- **Completed:** %s\n", result.CompletedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintln(w)

	// Analysis perspective
	if result.Params.Perspective != "" {
		fmt.Fprintf(w, "## Analysis Perspective\n\n%s\n\n", result.Params.Perspective)
	}

	// Executive summary
	fmt.Fprintf(w, "## Executive Summary\n\n%s\n\n", result.Summary)

	// Findings summary table
	if len(result.Findings) > 0 {
		fmt.Fprintf(w, "## Findings Overview\n\n")
		fmt.Fprintf(w, "| ID | Severity | Description |\n")
		fmt.Fprintf(w, "|---|---|---|\n")
		for _, f := range result.Findings {
			desc := truncate(f.Description, 80)
			fmt.Fprintf(w, "| %s | %s | %s |\n", f.ID, severityBadge(f.Severity), desc)
		}
		fmt.Fprintln(w)

		// Severity counts
		counts := countSeverities(result.Findings)
		fmt.Fprintf(w, "**Summary:** %d High, %d Medium, %d Low, %d Info\n\n",
			counts["high"], counts["medium"], counts["low"], counts["info"])

		// Detailed findings
		fmt.Fprintf(w, "## Detailed Findings\n\n")
		for _, f := range result.Findings {
			fmt.Fprintf(w, "### %s: %s\n\n", f.ID, f.Description)
			fmt.Fprintf(w, "- **Severity:** %s\n", severityBadge(f.Severity))
			fmt.Fprintf(w, "- **Window:** %d\n\n", f.WindowIndex)

			if len(f.Citations) > 0 {
				fmt.Fprintf(w, "**Evidence:**\n\n")
				for _, c := range f.Citations {
					fmt.Fprintf(w, "- Record #%d (`%s`)\n", c.RecordIndex, c.Source)
					if len(c.Excerpt) > 0 && string(c.Excerpt) != "null" {
						pretty, err := prettyJSON(c.Excerpt)
						if err == nil {
							fmt.Fprintf(w, "  ```json\n  %s\n  ```\n", indent(pretty, "  "))
						} else {
							// Fallback: output raw excerpt
							fmt.Fprintf(w, "  ```\n  %s\n  ```\n", string(c.Excerpt))
						}
					}
				}
				fmt.Fprintln(w)
			}
		}
	} else {
		fmt.Fprintf(w, "## Findings\n\nNo findings detected.\n\n")
	}

	return nil
}

func severityBadge(severity string) string {
	switch severity {
	case "high":
		return "**HIGH**"
	case "medium":
		return "MEDIUM"
	case "low":
		return "low"
	case "info":
		return "info"
	default:
		return severity
	}
}

func countSeverities(findings []types.Finding) map[string]int {
	counts := map[string]int{"high": 0, "medium": 0, "low": 0, "info": 0}
	for _, f := range findings {
		counts[f.Severity]++
	}
	return counts
}

func truncate(s string, maxRunes int) string {
	// Replace newlines for table display
	s = strings.ReplaceAll(s, "\n", " ")
	// Escape pipe characters for Markdown table cells
	s = strings.ReplaceAll(s, "|", "\\|")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "…"
}

func prettyJSON(raw json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
