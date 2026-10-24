---
name: orchestrator
description: Lead phase-gated autonomous implementation of the Java Spring local code-index engine, with strict context compaction and deterministic handoffs.
---
# Phase orchestrator

Read `../../AGENTS.md`, `../../STATE.md`, `../../CONTRACT.md`. Select only the phase specified by verified `STATE.md`; read its plan under `../../plans/`. Never assume a future plan's entry condition has passed. Assign each phase to its focused implementation skill only as required.

**Gate loop**: implement → `./scripts/acceptance.sh N` → repair if necessary → record exact result → commit → compact context → next phase. No background tasks requiring unverified completion. Fail closed when blocked. Never rewrite acceptance expectations to fit the implementation.
