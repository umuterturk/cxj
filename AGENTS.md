# Operating instructions for autonomous coding agents

**CURRENT STATE: Specification delivered; implementation unverified. Read `STATE.md` as canonical checkpoint.**

This replaces all previously issued agent instructions for this project. Use only this kit's acceptance contract.

## Mandatory workflow
1. Read `STATE.md`, `CONTRACT.md`, the **current phase** plan, and the relevant skill(s) in `skills/`. Do not load all plans into active context.
2. Implement **only** the current phase's scoped features. Do not optimize or refactor unrelated parts unless necessary for a failing gate.
3. Implement behaviors required by Go acceptance tests under `acceptance/`; `go test` is the acceptance framework, not an excuse to add mock-based unit suites.
4. Run `./scripts/acceptance.sh N` after Phase N. This runs **all** acceptance tests 0..N. Tests never use `t.Skip` or silently downgrade semantics.
5. If passing: run formatting / vet, commit phase changes, write `STATE.md` and `HANDOFF.md` (short, factual, with exact commands/outcomes), then compact context with the host's `/compact` mechanism if available; otherwise begin a fresh agent session using only `AGENTS.md`, `STATE.md`, and the next plan.
6. If failing: make at most three focused repair attempts on the same failure; if still blocked, record failure, command, output, cause, attempted fixes and suggested next step in `STATE.md`; **STOP**. Do not claim a pass, drop tests, switch models, fabricate data, or skip model-related gates.

## Never do these
- Never introduce a hardcoded synonym list or special-case `odeme.isle` / PetClinic-specific identifiers.
- Never add SQLite, RocksDB, on-disk vector cache/index, cloud inference, telemetry, or background uploads.
- Never mark ambiguous Java/Spring call targets as `exact`.
- Never print or embed secret values that can be identified as credentials/tokens/passwords.
- Never put an auto-skip in tests because Ollama, PetClinic, network, or tools are missing: fail with actionable setup instructions.
- Never silently expand scope to Kotlin or languages other than Java; other text formats are lexical only except Spring config files.
- Never write source-specific tests that trivially assert the substring the implementation is already given; evaluate real retrieval ranking and relationships.

## Quality constraints
- KISS: prefer single writer + immutable read snapshot; add complexity only after failing benchmarks.
- DRY: one IR, one canonical path normalization, one query schema and one exclusion matcher shared by scanning + watching + indexing.
- SOLID: parser, embedding provider, search index, Spring resolver, transport each behind a small and *necessary* contract. No excessive abstractions.
- Clean code: named domain types, explicit errors, deterministic results, context cancellation, bounded concurrency and limits; zero data races.
- All acceptance tests are **black-box Go tests against the running real binary and real Ollama model** where applicable; no fake providers. Tests may create fixture source files.

## Important setup
`CX_BIN` points at the built executable (script sets it). `CX_EMBED_URL` defaults to `http://127.0.0.1:11434`; `CX_EMBED_MODEL` defaults to `qwen3-embedding:0.6b`. Phase 3+ requires reachable locally-installed model. Phase 7 requires `PETCLINIC_ROOT` cloned at pinned commit.

## Context compaction protocol
At the END of each phase, before any next-phase implementation:
- Record `current_phase`, `last_passed_phase`, `test_command`, observed result, pinned versions, architectural decisions, known limitations and next action in `STATE.md` (≤400 words).
- Save `HANDOFF.md` (≤250 words) with **verified** facts only, no hidden chain-of-thought.
- Commit. **Compact the agent context** (`/compact` where supported); if not, start a clean agent context/session. Read the next plan's `CURRENT STATE` heading and reconcile it with the actual `STATE.md`. `STATE.md` prevails; never infer tests passed from a plan's prerequisites.
- Future plans may be present on disk, but remain OUT OF SCOPE until the gate passes.
