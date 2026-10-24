**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 3 — Real multilingual local embeddings + hybrid retrieval

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..2 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 3 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Semantic discovery without vocabulary overlap, including multilingual questions, using a real embedding model. This is a REQUIRED capability, not a future enhancement.

**Implement:** Local Ollama `/api/embed` adapter to `qwen3-embedding:0.6b` as default. Model and endpoint configurable. Chunk Java by Tree-sitter classes/methods, include bounded signatures, comments and annotations. Query and indexing use same model. Keep float32 vectors entirely in RAM, brute-force cosine/dot first; RRF hybrid with lexical. Surface `building|ready|unavailable`; semantic/hybrid query 503 if model not ready (no dishonest lexical fallback). No synonyms, translation tables, LLM descriptions, fake embedding responses or preloaded fixture vectors.

**Gate:** `TestP3*`: real model availability, semantic intent search across unfamiliar English identifiers, Turkish-to-English retrieval, exact precedence in hybrid, model absent produces explicit error, index never writes embedding files. Use multiple held-out concepts and aggregate Recall@5/10 thresholds rather than asserting one magic translation. Run `./scripts/acceptance.sh 3`.

**Performance reality:** Initial embedding of many methods may be CPU-intensive. Lexical service becomes ready first. Track semantic progress; do not promise 500K LOC semantic index in seconds. Fail visibly, do not skip the model gate.
