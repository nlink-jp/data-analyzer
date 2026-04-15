# Changelog

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
