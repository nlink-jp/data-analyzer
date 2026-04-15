# RFP: data-analyzer

> Generated: 2026-04-15
> Status: Draft

## 1. Problem Statement

We need to extract insights from large-scale log data (JSON/JSONL, up to ~100K records) using natural language analysis instructions. The tool uses local LLMs (LM Studio / Ollama) via OpenAI-compatible API, but the context window limitation prevents feeding all data at once.

The conventional Map-Reduce approach suffers from excessive context compression during the Reduce phase, degrading analysis accuracy. This tool adopts a **sliding window + progressive summarization** approach: it iteratively synthesizes previous summaries with new RAW data and accumulated Findings. To prevent hallucination, every Finding must cite source RAW data records.

The target users are the organization's security and operations teams. The goal is to obtain natural language reports containing discoveries and insights from large volumes of endpoint operation logs and access logs, guided by specific analysis perspectives.

## 2. Functional Specification

### Commands / API Surface

Three subcommands:

| Subcommand | Purpose |
|---|---|
| `prepare` | Interactively build analysis prompts and output a parameter JSON file |
| `analyze` | Run sliding window analysis with parameters + input data, output structured JSON |
| `compile` | Render analysis result JSON into Markdown/HTML |

**analyze subcommand key flags:**
- `--params <file>` — Parameter JSON file (or BASE64 string)
- `--resume <job-id>` — Resume an interrupted job
- `--output <file>` — Output file (default: stdout)

**compile subcommand key flags:**
- `--format <md|html|both>` — Output format
- `--output <file>` — Output file (default: stdout)

**prepare subcommand key flags:**
- `--output <file>` — Parameter JSON output path

### Input / Output

**Input:**
- JSON (array format) or JSONL (one object per line)
- File path, directory (batch processing), or stdin

**Output:**
- `analyze` → Structured JSON (AnalysisResult: summary, findings, citations)
- `compile` → Markdown / HTML report
- Progress information displayed on stderr

### Configuration

Three-layer configuration (following organization conventions):
1. Compiled-in defaults
2. Config file: `~/.config/data-analyzer/config.toml`
3. Environment variables: `DATA_ANALYZER_API_ENDPOINT`, `DATA_ANALYZER_API_MODEL`, etc.
4. CLI flags (highest priority)

```toml
[api]
endpoint = "http://localhost:1234/v1"
model = "google/gemma-4-26b-a4b"
api_key = ""

[analysis]
context_limit = 131072  # 128K tokens
overlap_ratio = 0.1
max_findings = 100

[job]
temp_dir = ""  # default: os.TempDir()/data-analyzer
```

### External Dependencies

- Local LLM: LM Studio or Ollama (OpenAI-compatible API)
- Go standard library + `nlk` (organization shared library)
- No external cloud service dependencies

## 3. Design Decisions

**Language: Go**
- Easy cross-platform distribution (single binary) for mixed macOS/Windows environments
- Existing nlk library (guard, backoff, strip, jsonfix, validate) available
- Consistent ecosystem with existing util-series tools (gem-query, mail-analyzer-local, etc.)

**LLM calls: Direct OpenAI-compatible API implementation**
- Direct HTTP requests via net/http (no external SDK)
- Modularized internally (LLMClient interface) for testability and extensibility
- nlk/backoff for retries, nlk/strip for thinking tag removal, nlk/jsonfix for JSON repair

**Core algorithm: Sliding window + progressive summarization**
- Avoids Map-Reduce information loss
- Overlapping windows preserve boundary context
- Finding accumulation with priority management (keep high-severity, FIFO-evict low-severity)
- Mandatory RAW data citations for hallucination prevention

**Relationship with existing tools: Independent**
- gem-query is search-focused, json-filter is for JSON repair — different purposes
- Ports and generalizes the LLM client pattern from mail-analyzer-local

**Out of scope:** None at this point (Phase 1)

## 4. Development Plan

### Phase 1: Core
- Project scaffold (`_wip/data-analyzer/`)
- LLM client module (OpenAI-compatible API, retries, nlk integration)
- Three-layer config management (config.toml + env + flags)
- JSON/JSONL reader (file/directory/stdin)
- Token estimator (CJK-aware)
- Sliding window engine (core algorithm)
- Memory map (dynamic context budget allocation)
- Job manager (ID, checkpoints, resume, idempotency)
- `analyze` subcommand
- Unit tests for all core modules

### Phase 2: Features
- `prepare` subcommand (interactive prompt builder)
- `compile` subcommand (Markdown output)
- stderr progress display
- Directory batch input, stdin support
- BASE64 parameter input

### Phase 3: Release
- HTML output (compile)
- Documentation (README.md / README.ja.md)
- CHANGELOG.md
- Cross-platform build and release

Each phase can be reviewed independently.

## 5. Required API Scopes / Permissions

- No external cloud authentication required (local LLM usage)
- API key support implemented (typically unused for local API, but available for future remote API use)

## 6. Series Placement

Series: **util-series**
Reason: Positioned as a pipe-friendly data transformation/processing CLI alongside gem-query, json-filter, mail-analyzer-local, etc. While it uses local LLMs, the tool's essence is data analysis CLI, which differs from lite-series (local LLM interaction tools) in purpose.

## 7. External Platform Constraints

- **Model:** google/gemma-4-26b-a4b (256K context) assumed
- **Design context limit:** 128K tokens (limited to half for optimal performance)
- **Endpoint:** Primarily LM Studio (localhost:1234/v1)
- **LM Studio / Ollama API differences:** Variations in response format support
- **Hardware dependency:** Response speed depends on local GPU performance
- **Think mode:** Based on mail-analyzer-local findings, Think mode may degrade accuracy. Designed with default OFF.

---

## Discussion Log

1. **Context limit breakthrough approach** — Discussed Map-Reduce information loss. Adopted sliding window + progressive summarization. Previous summary + new RAW data + accumulated Findings are iteratively synthesized, avoiding accuracy degradation from Reduce-phase context compression.

2. **Two-pass architecture** — Initially considered three stages (Compile→Analyze→Report), changed to two-pass (analyze → compile). Each subcommand is independently executable following Unix philosophy.

3. **Interactive prompt building** — Determined that analysis perspectives should not be passed directly to the LLM; a "compilation" step is needed to structure the required analysis information. Implemented as the `prepare` subcommand with interactive interface. Completed parameters output as JSON file, specified during `analyze`. BASE64 CLI argument passing also supported.

4. **Job management and idempotency** — Processing 100K records requires significant time, so checkpoint-based resume implemented. Job IDs generated from timestamp + input hash. Completed jobs skip re-execution (idempotency). Content idempotency cannot be guaranteed due to LLM non-determinism, which is accepted.

5. **Hallucination prevention** — Findings require RAW data citations via record index. Prompts request "[Record #N]" format references, structurally saved as Citations.

6. **Language selection** — Go selected for cross-platform requirements in mixed macOS/Windows environments. Consistency with existing nlk library and util-series ecosystem was also decisive.

7. **Memory map design** — 128K token budget dynamically allocated. RAW data budget shrinks as Findings accumulate, but later windows benefit from richer context. Priority-based FIFO eviction handles Finding overflow.
