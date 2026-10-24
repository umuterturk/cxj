# Compact handoff

**ALL PHASES 0–7 PASSED.** Release-ready under this kit's contract.

## Verified gates
- `./scripts/acceptance.sh 7` PASS
- `CX_RACE=1 ./scripts/acceptance.sh 7` PASS
- PetClinic pin: `500158f732419217507c7656904b8e6aa1bcc0d6`
- Benchmark report present with measured fields (`./scripts/benchmark.sh`)

## Product surface
`cx serve` + CLI (`find`, `references`, `endpoints`, `beans`, `config`, `context`, `status`); agent skill `skills/cx-cli/SKILL.md` (MCP removed). HTTP `/v1/*`; semantic via Ollama; live fsnotify reindex; no on-disk index.
