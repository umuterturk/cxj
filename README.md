# Java + Spring local code intelligence: agent build kit

**Status: SPECIFICATION ONLY — not implemented, not tested.** This kit replaces the previous MVP/AGENTS/MVP_PLUS instructions. Do not combine their test contracts.

## Goal
Build `cx`, a **fully RAM-resident index** for one local workspace (up to 500,000 source lines): one-shot Go CLI, Tree-sitter Java syntax, Spring-aware relationships, configuration intelligence, ordinary text search, and folder exclusion. No persistent index/database, no HTTP daemon, no embeddings, and no cloud API. Each command indexes `--root` (default: cwd) in RAM, answers, and exits.

## Start here
1. Copy the entire kit into the **new project's root** (do not overwrite any existing working code without review).
2. Read `AGENTS.md`, `STATE.md`, `CONTRACT.md`, and `plans/00-foundation.md`.
3. Start only with **Phase 0**; follow the gate sequence in `AGENTS.md`.
4. Run `./scripts/acceptance.sh 0` after implementation. Each successive gate runs **every test from phase 0 through the selected phase**.
5. At each passed phase: commit, update `STATE.md`, create a short handoff in `HANDOFF.md`, and **compact/restart agent context** before reading the next plan.

## Environment
- Go: supported stable Go version (the supplied Go acceptance suite uses only the standard library).
- Java parsing: official Tree-sitter Go bindings and Java grammar. Core index independent of parser.
- Agent transport is the CLI (`skills/cx-cli/SKILL.md`). No daemon, no MCP, no Ollama.
- Main real-world validation repo: https://github.com/spring-projects/spring-petclinic . Run `./scripts/prepare-petclinic.sh`, then pin its output commit in `STATE.md` in Phase 7. The primary acceptance suite uses controlled Java fixtures so upstream changes cannot make tests pass/fail unpredictably.

## Phase plans (all self-contained)
| Phase | Plan | Main gate |
|---|---|---|
| 0 | `plans/00-foundation.md` | Contract, CLI status, failure reporting |
| 1 | `plans/01-java-index.md` | Java definitions and references, IR |
| 2 | `plans/02-text-exclusions.md` | Fuzzy/exact search, files, exclusions |
| 3 | `plans/03-embeddings.md` | Historical — embeddings removed |
| 4 | `plans/04-spring.md` | Endpoints, beans, DI, transactions, repository discovery |
| 5 | `plans/05-config.md` | YAML/properties, profiles, Java bindings, secrets |
| 6 | `plans/06-live-mcp.md` | Re-run-sees-edits, CLI |
| 7 | `plans/07-benchmark.md` | 500K LOC, real PetClinic, reliability/performance |

## Important decisions
- No synonym dictionary and **no acceptance examples hardcoded into product logic**.
- Exact matches always win; heuristic evidence must not be called exact.
- Search is lexical only (exact/prefix/fuzzy identifiers and plain text lines).
- Other text files: **simple lexical search** (UTF-8 text, line references) unless they are Spring configs; don't pretend arbitrary text has Java symbols.
- No hidden/skipped acceptance tests, no unit-test detour, no weakening gate thresholds, no LLM-generated code descriptions.
- KISS, DRY, SOLID, clean code: favor small cohesive packages and plain Go; add interfaces only at meaningful boundaries.

## Primary technical references
- Tree-sitter Go: https://github.com/tree-sitter/go-tree-sitter
- Spring Boot config: https://docs.spring.io/spring-boot/reference/features/external-config.html
- Agent Skills: https://agentskills.io/specification
- Agent CLI skill: `skills/cx-cli/SKILL.md`
