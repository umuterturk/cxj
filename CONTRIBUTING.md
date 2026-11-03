# Contributing to cxj

Bugs found here are found by agents in real Spring services, and fixed the same
way: measure, fix, gate. This file is the short version of how the repo works.

## The law

cxj is **stateless**: one-shot `Open → query → exit`. No daemon, no persisted
index, no embeddings, no cache — speed comes from doing less work, never from
holding state. Proposals that add persistent state will be rejected on sight;
that is the product. The full design record is
[docs/FIX_PLAN.md](docs/FIX_PLAN.md).

## Development loop

```sh
go build ./... && go vet ./...
go test ./...                # deterministic fixtures only; no network, no real repos
./scripts/perf-bench.sh      # the perf gate (CI runs it on every push)
./scripts/sync-skill.sh      # after touching internal/skill/SKILL.md
./scripts/sync-skill.sh --check  # CI's version of the same check
```

Requirements: Go 1.23+ and a C toolchain (tree-sitter is cgo).

- Tests run against inline/temp fixtures. A test that reads a real repository
  or the network will not survive CI and does not belong here.
- A perf regression is a product regression: numbers move only with a
  measurement in the PR/commit message.
- `internal/skill/SKILL.md` is the canonical agent skill (embedded in the
  binary); the plugin and skills.sh copies are mirrors — never edit them
  directly, run the sync script.

## Reporting bugs

The fastest fix you can buy is a **measured reproduction**:

1. `cxj version` and your OS/architecture
2. the repo's shape (`cxj status`, plus the first lines of `cxj brief`)
3. the exact command, actual output, expected output
4. whether it reproduces with `--scope all`

The bug-report template collects exactly this. Every bug this project has
fixed arrived that way.

## Commits and releases

- Conventional prefixes (`feat:`, `fix:`, `test:`, `docs:`, `ci:`), one green
  commit per unit of work — the history is the changelog's source.
- Every user-visible claim in README/docs (flags, output formats, digests,
  install commands) is verified live before the commit that ships it; a
  listing or doc that drifts from the tool is treated as a bug.
- Releases are tags; CI builds all four platform assets and the tap/npm
  channels follow the tag.

## Design questions

Ask in an issue before building anything that touches the engine's output
contract (certainty labels, caps, exit codes) or the asset names — both are
load-bearing for agents and downstream channels.
