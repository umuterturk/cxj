**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 2 — Deterministic search, text files, exclusions

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..1 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 2 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Exact + prefix + typo-tolerant Java symbol search, and reliable ordinary text search of other files. Exclusions affect ALL indexing paths.

**Implement:** Fuzzy identifiers (bounded edit distance, not full-scan on every query), case normalization, deterministic ranking, reusable UTF-8 text scanner; text result includes correct file and line. `.md`, `.txt`, `.xml`, `.json`, `.sql`, `.sh`, `.gradle`, `Dockerfile`, `.yml` and any safe UTF-8 file treated as text. Binary/oversize skipped. Repeatable `--exclude`, `.cxignore`, default directory excludes. No symlink walking. File patterns canonicalized once. Exclusion enforced before every index operation.

**Gate:** `TestP2*`: typos, exact outranks fuzzy, UTF-8 text search, binaries ignored, flags and `.cxignore`, excluded files absent in symbol and text indexes. `./scripts/acceptance.sh 2`.

**Explicit rule:** `odeme.isle` is **not** translated with a dictionary. Semantic discovery arrives in Phase 3 using a real model. Do not build queries into source code.
