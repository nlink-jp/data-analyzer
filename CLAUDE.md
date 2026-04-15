# CLAUDE.md — data-analyzer

**Organization rules (mandatory): https://github.com/nlink-jp/.github/blob/main/CONVENTIONS.md**

## Project overview

Large-scale JSON/JSONL data analysis CLI using local LLMs via OpenAI-compatible API.
Uses sliding window + progressive summarization to overcome context window limitations.
Part of util-series.

## Non-negotiable rules

- **Tests are mandatory** — write them with the implementation.
- **Never `go build` directly** — always `make build` (outputs to `dist/`).
- **Docs in sync** — update `README.md` and `README.ja.md` together.
- **Small, typed commits** — `feat:`, `fix:`, `test:`, `chore:`, etc.
- **Security first** — prompt injection defense (nlk/guard), no secrets in code.
- **LLM client is modular** — all LLM calls go through the `llm.Client` interface.

## Build & test

```bash
make build          # → dist/data-analyzer
make test           # or: go test ./...
make build-all      # cross-compile 5 platforms
```

## Architecture

- `cmd/` — Cobra CLI with subcommands (analyze, prepare, compile)
- `internal/config/` — TOML + env var + flag configuration
- `internal/llm/` — OpenAI-compatible API client (interface-based)
- `internal/reader/` — JSON/JSONL input reader
- `internal/token/` — Token estimation (CJK-aware)
- `internal/window/` — Sliding window engine + memory map
- `internal/job/` — Job management, checkpoints, idempotency
- `internal/prompt/` — Prompt templates with nlk/guard
- `internal/types/` — Shared type definitions
