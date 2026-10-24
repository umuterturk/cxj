# CURRENT STATE — CANONICAL CHECKPOINT

- Current phase: **2** (next to implement)
- Status: **Phase 1 PASSED**
- Last verified passed phase: **1**
- Last acceptance command/result: `./scripts/acceptance.sh 1` → PASS (`TestP0*` + `TestP1*` all ok)
- Engine: Tree-sitter Java (`go-tree-sitter` v0.25.0, `tree-sitter-java` v0.23.5) → `internal/java` FileFacts → `internal/index` RAM symbol/ref lookup; `GET /v1/references`
- Embedding model: planned `qwen3-embedding:0.6b`; semantic still `off`
- Reference repository pin: **not selected** (Phase 7)
- Architectural decisions: parser-neutral IR; calls/implements never marked exact for virtual dispatch; exact > prefix search ranking; deterministic path/line/symbol ties
- Known limitations: no fuzzy/text search, exclusions unused, no Spring/config/embeddings
- Next action: Read `plans/02-text-exclusions.md` + `skills/files-exclusions/SKILL.md`, implement Phase 2, run `./scripts/acceptance.sh 2`.

This file is rewritten after every gate. Do not treat any plan file as proof of implementation completion.
