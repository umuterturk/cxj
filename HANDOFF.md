# Compact handoff

**Phase 1 PASSED.** `./scripts/acceptance.sh 1` → all `TestP0*` + `TestP1*` ok.

## Verified
- Java classes/methods indexed with real 1-based lines; overloads keep distinct locations.
- Lexical search matches `Owner.method` and simple names (exact/prefix).
- `/v1/references` does not mark interface dispatch as exact to concrete impls.
- Deps: `github.com/tree-sitter/go-tree-sitter v0.25.0`, `tree-sitter-java v0.23.5` (CGO).

## Next
Phase 2: text lexical search, `.cxignore` / `--exclude`, fuzzy/exact ranking, UTF-8/size skips. Plan: `plans/02-text-exclusions.md`. Gate: `./scripts/acceptance.sh 2`.
