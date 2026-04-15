# Architecture: data-analyzer

## Why This Tool Exists

When analyzing large log datasets (100K+ records) with LLMs, the naive approach
of feeding all data into a prompt hits the context window limit. The two common
workarounds each have fundamental flaws:

1. **Truncation** — You only see part of the data. Patterns spanning the entire
   dataset are invisible.
2. **Map-Reduce** — Each chunk is analyzed independently, then results are
   merged. The Reduce step compresses intermediate context so aggressively that
   subtle cross-chunk patterns are lost. We observed significant accuracy
   degradation in practice.

data-analyzer solves this with a **sliding window + progressive summarization**
approach that preserves context across the entire dataset.

## Why Sliding Window + Progressive Summarization

The key insight: human analysts don't read 100K records at once either. They
read a batch, form a mental model, then read the next batch with that model
in mind. New observations refine and update the model.

This is exactly what the sliding window engine does:

```
Window 0: [RAW data chunk 0] → LLM → Summary₀ + Findings₀
Window 1: [Summary₀] + [Findings₀] + [RAW data chunk 1] → LLM → Summary₁ + Findings₁
Window 2: [Summary₁] + [Findings₀₊₁] + [RAW data chunk 2] → LLM → Summary₂ + Findings₂
...
```

**Why this beats Map-Reduce:**
- The running summary acts as a "mental model" that carries context forward.
  Map-Reduce has no equivalent — each mapper is stateless.
- Findings accumulate separately from the summary. This prevents important
  discoveries from being compressed away during summarization.
- Window overlap ensures records near chunk boundaries aren't split from their
  context.

**Why not RAG/embedding-based retrieval?**
- RAG answers specific questions well, but discovery-oriented analysis ("find
  anomalies") requires seeing data sequentially. You don't know what to query
  until you've seen the patterns.
- Embedding-based similarity misses temporal patterns (e.g., brute-force
  attempts that only look suspicious as a sequence, not individually).

## Why Cap Window Size (max_records_per_window)

Even when the token budget allows 500+ records in a single window, LLM output
quality degrades significantly with very large inputs. We observed this in
production: 559 records fit within 128K tokens mathematically, but the model
produced truncated JSON, omitted fields from citations, and generated
shallower analysis compared to smaller windows.

`max_records_per_window` (default: 200) enforces a hard cap regardless of
token budget. This is a **quality guard**, not a memory guard. The token budget
controls what fits; the record cap controls what produces good results.

With 559 records and max 200 per window, the engine processes ~3 windows of
200 + overlap, producing better findings than 1 window of 559.

## Why Separate Findings from Summary

Findings and summary serve different purposes:

- **Summary** is a compressed representation of everything seen so far. It
  grows then stabilizes as the analysis progresses. It provides context for
  interpreting new data.
- **Findings** are discrete, citable discoveries. They must retain their
  citations (record indices) to prevent hallucination. They grow monotonically.

If findings were folded into the summary, two problems arise:
1. The summary's compression would strip citations, making claims unverifiable.
2. Important but early findings would be diluted as the summary evolves.

By keeping them separate, we can also apply priority-based eviction: when
findings exceed the context budget, we evict low-severity items (FIFO) while
always retaining high-severity ones. Evicted findings remain in the final
output — they're just not included in subsequent LLM context.

## Why Citation Verification (Not Trust)

Every Finding must reference at least one source record by its `[Record #N]`
index. But **we never trust the LLM's excerpt**. Instead, the engine performs
a three-layer verification:

### Layer 1: Record Index Validation
If the record index is out of range, the citation is removed. This catches
cases where the LLM fabricates a record number entirely.

### Layer 2: Relevance Check
The engine checks whether the LLM's excerpt values actually exist in the
original record. This is a substring match: if at least one non-trivial value
from the excerpt appears in the original JSON, the citation is considered
relevant. This catches fabricated field values while tolerating field selection
(the LLM may cite only a subset of fields).

### Layer 3: Forced Original Replacement
Regardless of relevance, the excerpt is **always replaced with the full
original record**. LLM excerpts frequently omit fields, reformat values, or
include partial data. By replacing with the original, every citation in the
final report contains the complete, unmodified source data.

### Hallucination Warning
If a citation's excerpt values are entirely absent from the original record
(Layer 2 fails), the engine emits a warning. The citation is still included
with the original record — the user sees the actual data and can judge
whether the finding is valid.

### Citation Recovery
When a Finding has no citations at all (LLM returned empty `citations[]`),
the engine extracts `Record #N` references from the finding's description
text and builds citations from the original records. This recovers evidence
that the LLM referenced in prose but failed to structure as JSON.

**Why not just validate and keep the LLM's excerpt?**
Because local LLMs consistently produce excerpts that are technically valid
JSON but omit important fields. In a 559-record test, every single citation
needed correction — the LLM always selected a subset of fields. Showing
partial excerpts hides context that may be critical for the analyst.

## Why Mandatory Citations

Every Finding must reference at least one source record by its `[Record #N]`
index. This is a deliberate anti-hallucination measure.

Local LLMs are particularly prone to plausible-sounding fabrications. By
requiring citations, we create a verifiable chain: the compile step can look up
the original record and confirm the finding is grounded in actual data, not
generated from the model's priors.

## Why the 128K Design Limit

The target model (gemma-4-26b-a4b) has a 256K token context window. We
deliberately limit to 128K because:

1. **Quality degrades at the edges** — Models produce lower-quality output as
   they approach their context limit. The 50% margin keeps us in the
   high-quality zone.
2. **Response budget** — The model needs room to generate output (5K reserved).
3. **Safety margin** — Token estimation is approximate (see below), so a
   buffer prevents truncation errors.

## Why Dual Token Estimation

Token estimation for mixed CJK/ASCII/JSON content is inherently approximate.
We use two independent estimators and take the maximum:

1. **Word-based** — CJK chars × 2 + ASCII words × 1.3 + punctuation count.
   Good for natural language prose.
2. **Char-based** — Total characters ÷ 4. Good for structured data (JSON)
   where punctuation dominates.

**Why the dual approach?** Early versions used word-based estimation only,
which counted JSON punctuation (`{`, `"`, `:`, `,`) as zero tokens. This
caused a ~4-5× undercount for JSON data, allowing 559 records into a single
window when the LLM could only handle ~200 effectively. The char-based
estimator catches these cases.

## Why the Memory Map

The 128K budget is divided dynamically rather than statically because the
composition changes as analysis progresses:

```
Early windows:  [2K system] [0 summary] [0 findings] [121K data] [5K response]
Mid windows:    [2K system] [10K summary] [8K findings] [103K data] [5K response]
Late windows:   [2K system] [15K summary] [20K findings] [86K data] [5K response]
```

Later windows process fewer records per window, but this is acceptable because:
- They have richer context from the accumulated summary and findings.
- Patterns that span the dataset are already captured in earlier summaries.
- The overlap ensures nothing falls through the cracks.

Note: the memory map operates alongside `max_records_per_window`. The actual
window size is `min(token_budget_allows, max_records_per_window)`.

## Why Three Subcommands

The `prepare → analyze → compile` pipeline follows Unix philosophy: each step
does one thing, produces an intermediate artifact, and can be run independently.

**Why not a single command?**
- `prepare` is interactive; `analyze` is batch. Mixing them creates awkward UX.
- The parameter file is reusable: run the same analysis on different datasets,
  or tweak parameters without re-running the LLM conversation.
- `compile` can be re-run without re-analyzing: change report format, add
  sections, or pipe through other tools.
- Debugging is easier: inspect `params.json` and `result.json` independently.

**Why `prepare --input`?**
When users paste large multi-line prompts into a terminal, `bufio.Scanner`
reads only the first line — remaining lines leak to the shell and are
executed as commands. This caused real incidents (zsh executing domain names
as commands). `--input` loads requirements from a file, bypassing terminal
paste issues entirely, then enters interactive refinement mode.

## Why Output Language Control

LLMs default to the language of their prompts (English in our case). For
Japanese-speaking analysts reviewing Japanese log data, English reports create
unnecessary friction. The `--lang` flag injects a language instruction into
both the window analysis prompt and the final report prompt, ensuring summary
text and finding descriptions are in the specified language.

Language is configurable at three levels (highest priority wins):
1. CLI flag: `--lang Japanese`
2. Parameter file: `"lang": "Japanese"` in params.json
3. Config: `analysis.lang` in config.toml or `DATA_ANALYZER_LANG` env var

## Why Parse Retry

Local LLMs occasionally produce malformed JSON that `nlk/jsonfix` cannot
repair (e.g., truncated mid-string with markdown fencing). Rather than
discarding an entire window's analysis, the engine retries once. The retry
often succeeds because LLM output is non-deterministic — the same prompt
may produce valid JSON on the second attempt.

If both attempts fail, the window is skipped and analysis continues. This
is preferable to aborting the entire job, which could waste hours of prior
computation.

## Why Checkpoints and Idempotency

Analyzing 100K records takes significant time (potentially hours with a local
LLM). Two guarantees make this practical:

1. **Checkpoints** — After each window, the engine saves its state (summary,
   findings, record offset). If interrupted (Ctrl+C, crash, power loss), the
   analysis resumes from the last checkpoint. Without this, any interruption
   would waste all prior compute.

2. **Idempotency** — Completed jobs are cached. Re-running the same analysis
   returns the cached result instantly. This prevents accidental re-execution
   and makes the tool safe to use in scripts and automation.

Checkpoints use atomic file writes (write to temp, then rename) to prevent
corruption from mid-write crashes.

## Why OpenAI-Compatible API (No SDK)

The LLM client uses raw HTTP rather than an SDK because:

1. **No external dependency** — OpenAI Go SDKs add transitive dependencies and
   version churn. The API surface we need is tiny (one endpoint).
2. **Compatibility** — LM Studio, Ollama, vLLM, and other local servers all
   implement the same `/chat/completions` endpoint with minor variations. A
   thin HTTP client handles these differences more easily than an opinionated
   SDK.
3. **Testability** — `httptest.Server` makes the client trivially testable
   without mocking complex SDK interfaces.

The client is behind an interface (`llm.Client`) so the engine can be tested
with a mock, and alternative backends can be added without changing the core
algorithm.

## Why nlk Integration

The `nlk` library provides five utilities essential for local LLM interaction:

| Module | Why |
|--------|-----|
| `guard` | Wraps untrusted data in nonce-tagged XML to prevent prompt injection. Log data can contain anything, including text that looks like instructions. |
| `strip` | Removes `<think>` tags. Local models (especially Gemma) emit chain-of-thought reasoning that must be stripped before JSON parsing. |
| `jsonfix` | Repairs malformed JSON. Local models frequently produce JSON with trailing commas, missing quotes, or markdown fencing. |
| `validate` | Rule-based output validation. Ensures severity values are from the allowed set. |
| `backoff` | Exponential retry. Local LLMs occasionally fail under memory pressure or concurrent load. |

These are battle-tested across multiple nlink-jp projects (mail-analyzer-local,
gem-query, lite-llm) and handle the full spectrum of local LLM quirks.

## Why Go

- **Single binary distribution** — The tool must run on macOS and Windows
  without runtime dependencies. Go's static compilation is ideal.
- **Ecosystem consistency** — Most util-series tools are Go. Shared patterns
  (Cobra CLI, TOML config, nlk library) reduce cognitive overhead.
- **No CGO needed** — Unlike gem-query (DuckDB), this tool has no C
  dependencies, so cross-compilation works on all 5 target platforms.
