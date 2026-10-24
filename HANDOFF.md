# Compact handoff

**ALL PHASES 0–7 PASSED.** One-shot CLI; embeddings and HTTP serve removed.

## Verified gates
- `./scripts/acceptance.sh 7` PASS
- `CX_RACE=1 ./scripts/acceptance.sh 7` PASS
- PetClinic pin: `500158f732419217507c7656904b8e6aa1bcc0d6`
- Benchmark report present (`./scripts/benchmark.sh`)

## Product surface
`cx find|references|endpoints|beans|config|context|status [--root PATH]` (default cwd). Agent skill `skills/cx-cli/SKILL.md`. Lexical only; no daemon, no Ollama, no on-disk index. Re-run after edits.
