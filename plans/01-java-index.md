**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 1 — Java structural extraction and RAM index

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..0 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 1 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Parse Java definitions via official Tree-sitter Go Java grammar; convert to parser-neutral IR; build in-RAM symbol/definition lookup.

**Implement:** `JavaExtractor.Extract(path,bytes) -> FileFacts` of definitions, occurrences and candidate edges. Tree-sitter objects lifecycle closed. IR representation records name, qualified name, kind, owner, file and byte/line location; certainty. One small index consumer that uses numeric IDs/interned strings when warranted. No Java syntax types in core index. Search Java class and method names exactly and by prefix. Anonymous/overloaded methods must not be conflated.

**Gate:** `TestP1*`: symbols/classes/methods, overloaded signatures and location, reference candidates without false exact resolution; run `./scripts/acceptance.sh 1`.

**Non-goals:** Compiling Java, resolving arbitrary dynamic dispatch, Spring, embedding, generic JVM semantics. Zero fabricated exact edges.
