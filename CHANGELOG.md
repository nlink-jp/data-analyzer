# Changelog

## [0.3.1] - 2026-04-16

### Fixed
- **Findings token budget not enforced in prompt** — as findings accumulated
  with full original records as citation excerpts, the actual prompt size grew
  far beyond the budgeted 20K tokens (up to 80K+), causing progressively longer
  LLM processing times and eventual timeouts. The memory map calculated the
  budget correctly but never truncated the findings sent to the LLM.
- The engine now creates a trimmed copy of findings for each prompt, stripping
  citation excerpts from the oldest findings first (`"[see original]"`). Full
  findings with original excerpts are preserved in checkpoints and final output.
- Debug logging now shows both raw and prompt-trimmed findings token counts.

## [0.3.0] - 2026-04-16

### Added
- **LLM backend resilience** — automatic retry and health-check on model crash
  or unload during long analysis sessions. The client detects crash errors
  (e.g., `"The model has crashed"`), polls `/v1/models` until the model reloads,
  then retries the request.
- `[resilience]` config section with `max_retries` (default: 10),
  `max_backoff_sec` (default: 120), `health_check_interval_sec` (default: 10),
  and `health_check_timeout_sec` (default: 300).
- Pre-flight model health check before each window and final report LLM call.
- Model crash/unload error patterns (`crashed`, `model not found`, `unloaded`)
  are now retryable regardless of HTTP status code.

### Changed
- Default max retries increased from 5 to 10.
- Default max backoff increased from 30s to 120s.

## [0.2.0] - 2026-04-15

### Added
- `clean` subcommand to remove old job caches (`--max-age 7d`, `--all`)
- `[tuning]` config section for token estimation coefficients and memory map
  parameters, enabling adaptation to different LLMs without code changes
- Debug logging of failed LLM responses (`--debug`) for troubleshooting
  parse failures

### Changed
- Token estimator coefficients are now configurable via `[tuning]` in config.toml
- Memory map reserves (system, response, summary, findings, raw data) are now
  configurable via `[tuning]` in config.toml

## [0.1.9] - 2026-04-15

### Added
- `prepare --sample` flag to provide sample data for field discovery. The LLM
  sees actual record structure and values, producing more accurate target_fields
  and attention_points. Sample data is also included in refinement prompts.

## [0.1.8] - 2026-04-15

### Fixed
- Display Job ID at analysis start so users can identify which job to resume
  after interruption.

## [0.1.7] - 2026-04-15

### Changed
- **Description in reports is no longer truncated** — shows full text in both
  overview table and detail sections. CJK text was being garbled by byte-based
  truncation.
- **Evidence always shows full original record** — citation verification now
  checks if LLM excerpt values exist in the original record (relevance check).
  Relevant excerpts are replaced with the complete original. Irrelevant excerpts
  (hallucination) trigger a warning and are still replaced with the original
  for the user to judge.

## [0.1.6] - 2026-04-15

### Fixed
- Citation evidence now shows the full original record instead of a partial
  subset. Previously, mismatch correction only kept fields the LLM mentioned,
  which could result in nearly empty excerpts when the LLM hallucinated
  field names.

## [0.1.5] - 2026-04-15

### Fixed
- Description truncation garbled CJK characters — `truncate` was cutting on byte
  boundaries instead of rune boundaries. Now uses `[]rune` slicing and `…` suffix.
- Evidence/citation logs missing from reports — excerpts with `null` value or
  prettyJSON failure now have fallback rendering.

### Added
- `max_records_per_window` config (default: 200) to cap window size and maintain
  LLM output quality. Even when token budget allows more records, large windows
  degrade analysis accuracy.
- Parse failure retry for window responses.

## [0.1.4] - 2026-04-15

### Fixed
- Token estimator severely undercounted JSON/structured data (~4-5x undercount)
  by ignoring punctuation characters. Now counts punctuation as tokens and uses
  the higher of word-based and char-based estimates.
- Window progress display now shows actual record range instead of misleading totals.
- Parse failures trigger one automatic retry before skipping the window.

### Added
- Improved debug output (`--debug`) showing offset, count, budget, summary and
  findings token sizes per window.

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
