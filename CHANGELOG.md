# Changelog

## [0.1.3] - 2026-04-15

### Added
- `--lang` flag and `lang` field in params for output language control
  (e.g., `--lang Japanese`). Also configurable via `DATA_ANALYZER_LANG` env var
  or `analysis.lang` in config.toml.
- Citation recovery: when LLM returns findings with empty citations, the engine
  extracts Record #N references from the description text and builds citations
  from the original data.

## [0.1.2] - 2026-04-15

### Fixed
- Multi-line paste in `prepare` caused remaining lines to leak to the shell as
  commands. Input now reads until an empty line instead of single-line mode.

### Added
- `prepare --input` flag to load initial requirements from a file, then enter
  interactive refinement mode. Prevents terminal paste issues for long prompts.
- Multi-line input support in interactive mode (end with empty line)

## [0.1.1] - 2026-04-15

### Fixed
- Scanner buffer overflow in `prepare` subcommand — long user input (>64KB) caused
  silent truncation and unexpected behavior. Buffer increased to 1MB with proper
  error checking.

## [0.1.0] - 2026-04-15

### Added
- `analyze` subcommand with sliding window + progressive summarization engine
- `prepare` subcommand for interactive analysis parameter building
- `compile` subcommand for Markdown report generation
- OpenAI-compatible LLM client with retry and nlk integration
- 3-layer configuration (TOML + env vars + CLI flags)
- JSON/JSONL reader (file, directory, stdin support)
- CJK-aware token estimator
- Job management with checkpoint-based resume and idempotency
- Prompt builder with nlk/guard prompt injection defense
- Context budget calculator (memory map)
- HTML report output with embedded CSS
- `both` format for simultaneous md+html output
- Architecture documentation (why-focused, ja/en)
- RFP documents (Japanese and English)
- Cross-platform build (linux/darwin/windows × amd64/arm64)
- Citation verification against original records (anti-hallucination)
