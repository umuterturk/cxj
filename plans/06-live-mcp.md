**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 6 — Re-run-sees-edits and CLI

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..5 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 6 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Each CLI invocation indexes the current tree, so edits are visible on the next run (no daemon, no MCP, no watcher).

**Implement:** One-shot `cx find|references|endpoints|beans|config|context|status --root --json` (see `skills/cx-cli/SKILL.md`). Logs to stderr. Local-only paths and no external fetch. Exclusions apply on every scan.

**Gate:** `TestP6*`: create/edit/delete and rename visible on a subsequent `cx find`; excluded files stay excluded; CLI `find --json` returns real source. `./scripts/acceptance.sh 6` and `CX_RACE=1 ./scripts/acceptance.sh 6`.

**Non-goals:** MCP, HTTP daemon, embeddings, distributed sync, IDE plugins, on-disk cache.
