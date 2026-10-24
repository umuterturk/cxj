**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 7 — 500K LOC validation, real repo, and release decision

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..6 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 7 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Prove correctness and reasonable local performance at the product constraint for one-shot lexical indexing.

**Implement:** Benchmark script/report: one-shot exact/fuzzy/text p50/p95, initial Java parse time, active Go heap. Pin official `spring-projects/spring-petclinic` commit and test Java controller discovery against it. Generate nontrivial synthetic <=500K LOC fixture with realistic file sizes and symbols; no persistent index. Profile before optimization.

**Gate:** `TestP7*`: 500K LOC scan returns ready and reliable results; Go heap target <=512 MiB; PetClinic smoke checks real named structures; benchmark report exists and honestly reports numbers. `./scripts/acceptance.sh 7` and `CX_RACE=1 ./scripts/acceptance.sh 7`.

**Exit:** Release only with *all* previous gates green. Otherwise stop with failure inventory. Do not silently lower targets.
