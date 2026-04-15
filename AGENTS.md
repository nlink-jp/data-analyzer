# AGENTS.md — data-analyzer

## Summary

Large-scale JSON/JSONL data analysis CLI using local LLMs (OpenAI-compatible API).
Uses sliding window + progressive summarization to overcome context window limitations.
Part of util-series.

## Build & test

```bash
make build        # → dist/data-analyzer
make test         # go test ./...
make build-all    # cross-compile 5 platforms + zip
make clean        # rm -rf dist/
```

## Module path

`github.com/nlink-jp/data-analyzer`

## Key structure

```
cmd/
  root.go          — Cobra root command, global flags
  analyze.go       — analyze subcommand
  prepare.go       — prepare subcommand (interactive, --sample for field discovery)
  compile.go       — compile subcommand (Markdown/HTML)
internal/
  config/          — TOML + env var + flag 3-layer configuration
  llm/             — OpenAI-compatible API client (LLMClient interface)
  reader/          — JSON/JSONL reader (file, directory, stdin)
  token/           — Token estimation (CJK-aware)
  window/
    engine.go      — Sliding window analysis engine (core algorithm)
    memory.go      — Context budget calculator (memory map)
  job/             — Job ID, checkpoint, resume, idempotency
  prompt/          — System/user prompt builders (nlk/guard)
  prepare/         — Interactive parameter builder session
  compile/         — Report renderers (Markdown, HTML)
  types/           — Shared type definitions
testdata/          — Sample JSON/JSONL and parameter files
docs/              — RFP, architecture documents (ja/en)
```

## Configuration

Settings loaded: defaults → `~/.config/data-analyzer/config.toml` → env vars → CLI flags.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATA_ANALYZER_API_ENDPOINT` | No | `http://localhost:1234/v1` | OpenAI-compatible endpoint |
| `DATA_ANALYZER_API_MODEL` | No | `google/gemma-4-26b-a4b` | Model name |
| `DATA_ANALYZER_API_KEY` | No | — | API key (optional) |
| `DATA_ANALYZER_CONTEXT_LIMIT` | No | `131072` | Context window budget (tokens) |
| `DATA_ANALYZER_OVERLAP_RATIO` | No | `0.1` | Window overlap ratio |
| `DATA_ANALYZER_MAX_FINDINGS` | No | `100` | Max findings to accumulate |
| `DATA_ANALYZER_TEMP_DIR` | No | `$TMPDIR/data-analyzer` | Checkpoint directory |

## Core algorithm

1. Read JSON/JSONL records
2. For each sliding window: [Previous Summary] + [Findings] + [New RAW Data] → LLM → [Updated Summary] + [New Findings]
3. Findings accumulate with RAW data citations
4. Checkpoint saved after each window (resume with `--resume`)
5. Final report generation from accumulated findings

## Key dependencies

- `github.com/spf13/cobra` — CLI framework
- `github.com/BurntSushi/toml` — TOML config parsing
- `github.com/nlink-jp/nlk` — guard, backoff, strip, jsonfix, validate

## Resilience

- `[resilience]` config controls retry and health-check for model crash/unload
- On model crash: client polls `/v1/models` until model reloads, then retries
- Pre-flight health check before each window and final report LLM call
- Default: 10 retries, 120s max backoff, 10s poll interval, 300s timeout
- Crash patterns (`crashed`, `model not found`, `unloaded`) are retryable
  regardless of HTTP status code (e.g., 400 with crash message)

## Gotchas

- Findings sent to LLM are token-budget-trimmed (oldest excerpts stripped first); full findings preserved in checkpoints/output
- LLM client uses net/http directly (no OpenAI SDK)
- Local models emit thinking tags — nlk/strip removes them
- Local models may emit malformed JSON — nlk/jsonfix repairs
- Token estimation is approximate (CJK: 1 char ≈ 2 tokens, ASCII: 1 word ≈ 1.3 tokens)
- Checkpoints use atomic file rename for durability
- 5-minute HTTP timeout for local LLM inference
- Model crash returns HTTP 400 (not 5xx) in some backends — handled by error message pattern matching
