# Compact handoff

**Phase 3 PASSED.** `./scripts/acceptance.sh 3` → all ok; semantic 8/8 + multilingual TR≥2.

## Verified
- `--embed-url/--embed-model` enables background indexing; `semantic_state` building→ready|unavailable.
- Semantic/hybrid return 503 when off/unavailable; hybrid preserves exact identifier rank.
- Excluded Java absent from semantic results. Host needs `ollama serve` + `qwen3-embedding:0.6b`.

## Next
Phase 4: Spring endpoints, beans, DI graph. Plan: `plans/04-spring.md`. Gate: `./scripts/acceptance.sh 4`.
