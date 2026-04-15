package compile

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/nlink-jp/data-analyzer/internal/types"
)

// HTML renders an AnalysisResult as a self-contained HTML report.
func HTML(w io.Writer, result *types.AnalysisResult) error {
	fmt.Fprint(w, htmlHead)

	fmt.Fprint(w, `<div class="container">`)

	// Header
	fmt.Fprint(w, `<h1>Analysis Report</h1>`)

	// Metadata
	fmt.Fprint(w, `<div class="meta">`)
	fmt.Fprintf(w, `<span>Job ID: %s</span>`, esc(result.JobID))
	fmt.Fprintf(w, `<span>Records: %d</span>`, result.TotalRecords)
	fmt.Fprintf(w, `<span>Windows: %d</span>`, result.WindowsUsed)
	fmt.Fprintf(w, `<span>%s — %s</span>`,
		result.StartedAt.Format("2006-01-02 15:04"),
		result.CompletedAt.Format("15:04 UTC"))
	fmt.Fprint(w, `</div>`)

	// Perspective
	if result.Params.Perspective != "" {
		fmt.Fprint(w, `<h2>Analysis Perspective</h2>`)
		fmt.Fprintf(w, `<p>%s</p>`, esc(result.Params.Perspective))
	}

	// Summary
	fmt.Fprint(w, `<h2>Executive Summary</h2>`)
	writeHTMLParagraphs(w, result.Summary)

	// Findings
	if len(result.Findings) > 0 {
		counts := countSeverities(result.Findings)

		// Severity badges
		fmt.Fprint(w, `<h2>Findings Overview</h2>`)
		fmt.Fprint(w, `<div class="badge-row">`)
		fmt.Fprintf(w, `<span class="badge high">%d High</span>`, counts["high"])
		fmt.Fprintf(w, `<span class="badge medium">%d Medium</span>`, counts["medium"])
		fmt.Fprintf(w, `<span class="badge low">%d Low</span>`, counts["low"])
		fmt.Fprintf(w, `<span class="badge info">%d Info</span>`, counts["info"])
		fmt.Fprint(w, `</div>`)

		// Table
		fmt.Fprint(w, `<table><thead><tr><th>ID</th><th>Severity</th><th>Description</th></tr></thead><tbody>`)
		for _, f := range result.Findings {
			fmt.Fprintf(w, `<tr><td><a href="#%s">%s</a></td><td><span class="badge %s">%s</span></td><td>%s</td></tr>`,
				f.ID, f.ID, f.Severity, strings.ToUpper(f.Severity), esc(truncate(f.Description, 100)))
		}
		fmt.Fprint(w, `</tbody></table>`)

		// Detailed findings
		fmt.Fprint(w, `<h2>Detailed Findings</h2>`)
		for _, f := range result.Findings {
			fmt.Fprintf(w, `<div class="finding" id="%s">`, f.ID)
			fmt.Fprintf(w, `<h3>%s: %s</h3>`, f.ID, esc(f.Description))
			fmt.Fprintf(w, `<div class="finding-meta">`)
			fmt.Fprintf(w, `<span class="badge %s">%s</span>`, f.Severity, strings.ToUpper(f.Severity))
			fmt.Fprintf(w, `<span>Window %d</span>`, f.WindowIndex)
			fmt.Fprint(w, `</div>`)

			if len(f.Citations) > 0 {
				fmt.Fprint(w, `<h4>Evidence</h4>`)
				for _, c := range f.Citations {
					fmt.Fprintf(w, `<div class="citation">`)
					fmt.Fprintf(w, `<div class="citation-header">Record #%d <code>%s</code></div>`,
						c.RecordIndex, esc(c.Source))
					if len(c.Excerpt) > 0 {
						pretty, err := prettyJSON(c.Excerpt)
						if err == nil {
							fmt.Fprintf(w, `<pre><code>%s</code></pre>`, esc(pretty))
						}
					}
					fmt.Fprint(w, `</div>`)
				}
			}
			fmt.Fprint(w, `</div>`)
		}
	} else {
		fmt.Fprint(w, `<h2>Findings</h2><p>No findings detected.</p>`)
	}

	fmt.Fprint(w, `</div>`) // container
	fmt.Fprint(w, htmlFoot)

	return nil
}

func esc(s string) string {
	return html.EscapeString(s)
}

func writeHTMLParagraphs(w io.Writer, text string) {
	paragraphs := strings.Split(text, "\n\n")
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Handle **bold** in text
		p = convertBold(esc(p))
		fmt.Fprintf(w, `<p>%s</p>`, p)
	}
}

func convertBold(s string) string {
	for {
		start := strings.Index(s, "**")
		if start == -1 {
			break
		}
		end := strings.Index(s[start+2:], "**")
		if end == -1 {
			break
		}
		end += start + 2
		s = s[:start] + "<strong>" + s[start+2:end] + "</strong>" + s[end+2:]
	}
	return s
}

const htmlHead = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Analysis Report</title>
<style>
  :root { --high: #dc2626; --medium: #f59e0b; --low: #3b82f6; --info: #6b7280; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         line-height: 1.6; color: #1f2937; background: #f9fafb; }
  .container { max-width: 960px; margin: 0 auto; padding: 2rem 1.5rem; }
  h1 { font-size: 1.75rem; margin-bottom: 1rem; border-bottom: 2px solid #e5e7eb; padding-bottom: 0.5rem; }
  h2 { font-size: 1.35rem; margin: 2rem 0 0.75rem; color: #374151; }
  h3 { font-size: 1.1rem; margin-bottom: 0.5rem; }
  h4 { font-size: 0.95rem; margin: 0.75rem 0 0.5rem; color: #4b5563; }
  p { margin-bottom: 0.75rem; }
  .meta { display: flex; flex-wrap: wrap; gap: 1rem; color: #6b7280; font-size: 0.875rem;
          margin-bottom: 1.5rem; }
  .badge-row { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
  .badge { display: inline-block; padding: 0.15rem 0.6rem; border-radius: 4px;
           font-size: 0.8rem; font-weight: 600; color: #fff; }
  .badge.high { background: var(--high); }
  .badge.medium { background: var(--medium); color: #1f2937; }
  .badge.low { background: var(--low); }
  .badge.info { background: var(--info); }
  table { width: 100%; border-collapse: collapse; margin-bottom: 1.5rem; font-size: 0.9rem; }
  th, td { text-align: left; padding: 0.5rem 0.75rem; border-bottom: 1px solid #e5e7eb; }
  th { background: #f3f4f6; font-weight: 600; }
  tr:hover { background: #f9fafb; }
  a { color: #2563eb; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .finding { background: #fff; border: 1px solid #e5e7eb; border-radius: 8px;
             padding: 1.25rem; margin-bottom: 1rem; }
  .finding-meta { display: flex; gap: 0.75rem; align-items: center; margin-bottom: 0.75rem; }
  .citation { background: #f3f4f6; border-radius: 6px; padding: 0.75rem; margin-bottom: 0.5rem; }
  .citation-header { font-size: 0.85rem; font-weight: 600; margin-bottom: 0.35rem; }
  .citation-header code { background: #e5e7eb; padding: 0.1rem 0.3rem; border-radius: 3px; font-size: 0.8rem; }
  pre { background: #1f2937; color: #e5e7eb; padding: 0.75rem; border-radius: 6px;
        overflow-x: auto; font-size: 0.8rem; margin-top: 0.35rem; }
  pre code { background: none; padding: 0; color: inherit; }
</style>
</head>
<body>
`

const htmlFoot = `</body>
</html>
`
