# Compact handoff

**Phase 0 PASSED.** `./scripts/acceptance.sh 0` → all `TestP0*` ok.

## Verified
- `cx serve --root --http` binds loopback only; invalid root exits nonzero before listen.
- `GET /v1/status` reports `ready`, file counters, `semantic_state=off`; no on-disk index artifacts.
- `GET /v1/search` returns empty lexical results (Phase 1 fills this).
- Packages: `cmd/cx`, `internal/ir`, `internal/scan`, `internal/engine`, `internal/server`.
- Fixed `scripts/acceptance.sh` empty-array/`set -u` crash so the mandated gate script runs.

## Next
Implement Phase 1 per `plans/01-java-index.md` and `skills/java-index/SKILL.md`. Tree-sitter Java → IR definitions; lexical exact/prefix symbol search; `/v1/references` without fabricating exact interface dispatch. Gate: `./scripts/acceptance.sh 1`.
