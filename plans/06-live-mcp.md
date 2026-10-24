**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 6 — Incremental updates, concurrency, MCP and CLI

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..5 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 6 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Truly live, consistent in-memory index with AI-friendly transport.

**Implement:** File watcher changes/renames/deletions; debounce; per-file invalidation and reparse; re-embed changed Java chunks; purge stale vectors/edges and excluded files; immutable read snapshots or safe reader-writer design. Concurrent query safety. `cx find --server --mode --json`; MCP stdio JSON-RPC implement initialize/tools/list/tools/call and tools `find`, `references`, `endpoints`, `config`, `context`, with bounded outputs. Logs to stderr (never MCP stdout). Local-only paths and no external fetch. Explicit semantic progress / stale state. Apply exclusion to watcher also.

**Gate:** `TestP6*`: create/edit/delete and rename visible; excluded changes stay excluded; semantic results refresh; no data races in Go acceptance race run; proper real MCP stdio handshake and tool invocation. `./scripts/acceptance.sh 6` and `CX_RACE=1 ./scripts/acceptance.sh 6`.

**Non-goals:** Distributed sync, multiple machines, network-exposed API, IDE plugins, on-disk cache.
