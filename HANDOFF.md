# Compact handoff

**Phase 6 PASSED.** `./scripts/acceptance.sh 6` and `CX_RACE=1 ./scripts/acceptance.sh 6` ok.

## Verified
- Create/modify/rename/delete reflected live; excluded paths stay out.
- Concurrent lexical queries consistent; MCP initialize/tools/list/tools/call(find) works on stdio.
- Java edits re-embed and bump semantic generation; stale symbols removed.

## Next
Phase 7: PetClinic pin + 500K LOC reliability/perf gates. Plan: `plans/07-benchmark.md`. Gate: `./scripts/acceptance.sh 7`. Run `./scripts/prepare-petclinic.sh` and record commit in STATE.
