---
name: handoff
description: Produce compact evidence-based phase transitions so an autonomous coding agent can safely reset context and continue from files without guessing history.
---
# Phase handoff and compaction

After an acceptance gate passes, update `../../STATE.md` with phase status, exact commands and outcomes, fixture pins and outstanding limits. Update `../../HANDOFF.md` (<=250 words), commit implementation and state, then invoke agent host context compaction if available. If no compaction API exists, end the phase and begin a fresh session that loads only `AGENTS.md`, `STATE.md`, `CONTRACT.md` and next plan.
