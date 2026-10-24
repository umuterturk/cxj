# CURRENT STATE — CANONICAL CHECKPOINT

- Current phase: **3** (next to implement)
- Status: **Phase 2 PASSED**
- Last verified passed phase: **2**
- Last acceptance command/result: `./scripts/acceptance.sh 2` → PASS (`TestP0*`..`TestP2*` all ok)
- Engine: exclusions (`internal/exclude` + `.cxignore`/`--exclude`/defaults), text line index, fuzzy symbol search (bounded Levenshtein), exact>prefix>fuzzy>text ranking
- Embedding model: planned `qwen3-embedding:0.6b` via Ollama; not wired yet
- Reference repository pin: **not selected** (Phase 7)
- Architectural decisions: one Matcher shared by scan; no symlink follow; binary/oversize skipped before index; no synonym dictionary
- Known limitations: no embeddings/hybrid, no Spring/config, no live watch/MCP
- Next action: Read `plans/03-embeddings.md` + `skills/semantic-search/SKILL.md`, implement Phase 3, run `./scripts/acceptance.sh 3` (requires local Ollama + model).

This file is rewritten after every gate. Do not treat any plan file as proof of implementation completion.
