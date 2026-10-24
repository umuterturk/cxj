**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 0 — Runnable service + contract

> **CURRENT STATE:** Specification only; no phase has passed. Begin here.
> **NEXT STATE IF PASS:** Update `STATE.md` to last passed phase 0, write handoff, commit and compact before phase 1.

**Goal:** Small Go daemon that can scan a root, serve JSON over loopback HTTP, expose meaningful startup/status errors, and shut down when killed. Build works offline after dependencies acquired.

**Implement:** `cmd/cx`, `internal/scan`, `internal/server`, `internal/ir` minimal types. `cx serve --root --http`. `GET /v1/status`, `GET /v1/search` (empty until later phases). Define stable JSON types in `CONTRACT.md`. Reject unsafe non-loopback binding. No database, index writes or embedding dependency. `cx find` CLI may be completed Phase 6.

**Gate:** Go black-box tests `TestP0*`: readiness, invalid root rejection, zero disk index, status metadata. Run `./scripts/acceptance.sh 0`.

**Implementation boundaries:** Keep request handlers thin; simple service struct. Don't prematurely create plugins, DI containers or generalized parsers. Create minimal logging to stderr. If gate fails: fix or stop, record blocker.
