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
3. **Safety margin** — Token estimation is approximate (CJK heuristic), so a
   buffer prevents truncation errors.

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
