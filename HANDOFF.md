# Compact handoff

**Phase 2 PASSED.** `./scripts/acceptance.sh 2` → `TestP0*`..`TestP2*` ok.

## Verified
- Fuzzy typos (edit distance) find symbols; never labeled exact; exact outranks fuzzy.
- UTF-8 text lines searchable with `kind=text`; binaries/oversized skipped (`indexed_files=0` in that fixture).
- `--exclude`, `.cxignore`, and default `target`/`build`/… applied before indexing.

## Next
Phase 3: real Ollama embeddings (`qwen3-embedding:0.6b`), semantic + hybrid search, 503 when not ready. Plan: `plans/03-embeddings.md`. Gate: `./scripts/acceptance.sh 3`. Ensure `ollama serve` and model pulled first.
