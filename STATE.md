# CURRENT STATE — CANONICAL CHECKPOINT

- Current phase: **1** (next to implement)
- Status: **Phase 0 PASSED**
- Last verified passed phase: **0**
- Last acceptance command/result: `./scripts/acceptance.sh 0` → PASS (`TestP0*` all ok)
- Engine implementation: `cmd/cx`, `internal/{ir,scan,engine,server}` — lexical scan + loopback HTTP; search empty until Phase 1
- Embedding model: planned `qwen3-embedding:0.6b` via local Ollama; not wired (semantic_state=`off`)
- Reference repository pin: **not selected** (Phase 7)
- Scope: Go; Java + Spring; ordinary UTF-8 text; in-RAM only; default exclude dirs in scan
- Architectural decisions: single-writer Engine snapshot; thin HTTP handlers; scan validates UTF-8/size; refuse non-loopback bind; `scripts/acceptance.sh` fixed for empty `go_flags` under `set -u`
- Known limitations: no Java AST/index, no text search, excludes flags accepted but unused, no embeddings
- Next action: Read `plans/01-java-index.md` + `skills/java-index/SKILL.md`, implement Phase 1, run `./scripts/acceptance.sh 1`.

This file is rewritten after every gate. Do not treat any plan file as proof of implementation completion.
