# data-analyzer

Large-scale JSON/JSONL data analysis CLI using local LLMs.

Uses a **sliding window + progressive summarization** approach to overcome
context window limitations — no Map-Reduce information loss.

## Features

- Analyze up to 100K+ JSON/JSONL records with local LLMs
- Sliding window engine with overlap for boundary context preservation
- Citation verification — every finding cites source records, verified against originals
- Checkpoint-based resume for long-running analysis
- Idempotent job execution
- Interactive parameter builder (with file input support)
- Markdown and HTML report output
- Output language control (`--lang Japanese`)

## Requirements

- Go 1.23+
- Local LLM with OpenAI-compatible API (e.g., [LM Studio](https://lmstudio.ai/))
- Recommended model: `google/gemma-4-26b-a4b` (Think OFF)

## Installation

```bash
make build    # → dist/data-analyzer
```

## Setup

```bash
# Option 1: Environment variables
export DATA_ANALYZER_API_ENDPOINT="http://localhost:1234/v1"
export DATA_ANALYZER_API_MODEL="google/gemma-4-26b-a4b"

# Option 2: Config file (~/.config/data-analyzer/config.toml)
mkdir -p ~/.config/data-analyzer
cp config.example.toml ~/.config/data-analyzer/config.toml
```

## Usage

### 1. Prepare analysis parameters

Build parameters interactively with LLM assistance:

```bash
# Interactive mode (supports multi-line input, end with empty line)
data-analyzer prepare --output params.json

# With sample data — LLM sees actual field names and values
data-analyzer prepare --sample logs.jsonl --output params.json

# Load requirements from file + sample data, then refine interactively
data-analyzer prepare --input requirements.txt --sample logs.jsonl --output params.json
```

Or create `params.json` manually:

```json
{
  "perspective": "Detect insider threats and unauthorized access",
  "target_fields": ["user", "action", "source_ip", "timestamp"],
  "attention_points": [
    "Multiple failed login attempts",
    "Privilege escalation",
    "Large data transfers to external services"
  ],
  "user_findings": [],
  "lang": "Japanese"
}
```

### 2. Run analysis

```bash
# Single file
data-analyzer analyze --params params.json logs.jsonl

# Directory (all .json/.jsonl files)
data-analyzer analyze --params params.json ./log_dir/

# With output file and language
data-analyzer analyze --params params.json --lang Japanese --output result.json logs.jsonl

# Resume interrupted analysis
data-analyzer analyze --params params.json --resume <job-id> logs.jsonl
```

### 3. Generate report

```bash
# Markdown to stdout
data-analyzer compile result.json

# HTML report
data-analyzer compile --format html --output report.html result.json

# Both Markdown and HTML
data-analyzer compile --format both --output report result.json

# From stdin
cat result.json | data-analyzer compile -
```

## Configuration

Settings are loaded in order: defaults → config file → env vars → CLI flags.

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_ANALYZER_API_ENDPOINT` | `http://localhost:1234/v1` | OpenAI-compatible API endpoint |
| `DATA_ANALYZER_API_MODEL` | `google/gemma-4-26b-a4b` | Model name |
| `DATA_ANALYZER_API_KEY` | — | API key (optional) |
| `DATA_ANALYZER_CONTEXT_LIMIT` | `131072` | Context window budget (tokens) |
| `DATA_ANALYZER_OVERLAP_RATIO` | `0.1` | Window overlap ratio (0.0–1.0) |
| `DATA_ANALYZER_MAX_FINDINGS` | `100` | Max findings to accumulate |
| `DATA_ANALYZER_MAX_RECORDS_PER_WINDOW` | `200` | Max records per window (quality guard) |
| `DATA_ANALYZER_LANG` | — | Output language (e.g., `Japanese`) |
| `DATA_ANALYZER_TEMP_DIR` | `$TMPDIR/data-analyzer` | Checkpoint directory |

## How It Works

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│   prepare    │───▶│   analyze    │───▶│   compile    │
│ (interactive)│    │(sliding win) │    │(md/html/both)│
└─────────────┘    └──────────────┘    └──────────────┘
   params.json        result.json       report.md/.html
```

**Sliding Window Algorithm:**

1. Divide records into overlapping windows (max 200 records per window)
2. For each window: `[Previous Summary] + [Findings] + [New Data]` → LLM
3. LLM returns updated summary + new findings with record citations
4. Citation verification: check excerpt relevance, replace with full original record
5. Checkpoint saved after each window (resume on interruption)
6. Final report generated from accumulated findings

**Citation Verification:**

Every citation from the LLM is verified against the original data:
- Excerpt values checked for relevance against the original record
- Excerpts always replaced with the full original record (no field omission)
- Non-matching excerpts flagged as possible hallucination
- Missing citations recovered from `Record #N` references in description text

**Memory Map (128K token budget):**

| Section | Allocation |
|---------|-----------|
| System prompt | ~2K (fixed) |
| Previous summary | 0→15K (grows, then stabilizes) |
| Accumulated findings | 0→20K (grows, priority eviction) |
| New RAW data | Remainder (~86K–106K) |
| Response buffer | ~5K (fixed) |

### 4. Clean up old jobs

```bash
# Remove completed jobs older than 7 days (default)
data-analyzer clean

# Remove completed jobs older than 1 day
data-analyzer clean --max-age 1d

# Remove all jobs (including incomplete)
data-analyzer clean --all
```

## License

MIT
