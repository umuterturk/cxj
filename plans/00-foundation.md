**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 0 — Runnable CLI + contract

> **CURRENT STATE:** Specification only; no phase has passed. Begin here.
> **NEXT STATE IF PASS:** Update `STATE.md` to last passed phase 0, write handoff, commit and compact before phase 1.

**Goal:** Small Go CLI that can scan a root, emit JSON status, and fail clearly on a missing workspace. Build works offline after dependencies acquired.

**Implement:** `cmd/cx`, `internal/scan`, `internal/ir` minimal types. `cx status [--root] [--json]`. Reject nonexistent roots. No database, index writes, HTTP daemon, or embedding dependency.

**Gate:** Go black-box tests `TestP0*`: readiness via `cx status --json`, invalid root rejection, zero disk index. Run `./scripts/acceptance.sh 0`.

**Implementation boundaries:** Keep command handlers thin. Don't prematurely create plugins, DI containers or generalized parsers. Create minimal logging to stderr. If gate fails: fix or stop, record blocker.
