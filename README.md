# Java + Spring local code intelligence: agent build kit

**Status: SPECIFICATION ONLY — not implemented, not tested.** This kit replaces the previous MVP/AGENTS/MVP_PLUS instructions. Do not combine their test contracts.

## Goal
Build `cx`, a **fully RAM-resident index** for one local workspace (up to 500,000 source lines): Go service, Tree-sitter Java syntax, Spring-aware relationships, **real local multilingual embeddings** for concept discovery, configuration intelligence, ordinary text search, and folder exclusion. No persistent index/database and no cloud API. Source files and model weights may reside on disk; **generated indexes and vectors never do**. The embedding inference process may run separately on localhost.

## Start here
1. Copy the entire kit into the **new project's root** (do not overwrite any existing working code without review).
2. Read `AGENTS.md`, `STATE.md`, `CONTRACT.md`, and `plans/00-foundation.md`.
3. Start only with **Phase 0**; follow the gate sequence in `AGENTS.md`.
4. Run `./scripts/acceptance.sh 0` after implementation. Each successive gate runs **every test from phase 0 through the selected phase**.
5. At each passed phase: commit, update `STATE.md`, create a short handoff in `HANDOFF.md`, and **compact/restart agent context** before reading the next plan.

## Environment
- Go: supported stable Go version (the supplied Go acceptance suite uses only the standard library).
- Java parsing: official Tree-sitter Go bindings and Java grammar. Core index independent of parser.
- Embeddings: **Ollama** on localhost with `qwen3-embedding:0.6b` by default; this is a real multilingual/code embedding model, not a hardcoded dictionary. `ollama pull qwen3-embedding:0.6b` before Phase 3.
- Host-local HTTP API for acceptance tests; agent transport is the CLI (`skills/cx-cli/SKILL.md`).
- Main real-world validation repo: https://github.com/spring-projects/spring-petclinic . Run `./scripts/prepare-petclinic.sh`, then pin its output commit in `STATE.md` in Phase 7. The primary acceptance suite uses controlled Java fixtures so upstream changes cannot make tests pass/fail unpredictably.

## Phase plans (all self-contained)
| Phase | Plan | Main gate |
|---|---|---|
| 0 | `plans/00-foundation.md` | Contract, service, failure reporting |
| 1 | `plans/01-java-index.md` | Java definitions and references, IR |
| 2 | `plans/02-text-exclusions.md` | Fuzzy/exact search, files, exclusions |
| 3 | `plans/03-embeddings.md` | Local semantic + multilingual + hybrid search |
| 4 | `plans/04-spring.md` | Endpoints, beans, DI, transactions, repository discovery |
| 5 | `plans/05-config.md` | YAML/properties, profiles, Java bindings, secrets |
| 6 | `plans/06-live-mcp.md` | Incremental edits, concurrency, CLI |
| 7 | `plans/07-benchmark.md` | 500K LOC, real PetClinic, reliability/performance |

## Important decisions
- No synonym dictionary and **no acceptance examples hardcoded into product logic**.
- Exact matches always win over semantic guesses; heuristic/semantic evidence must not be called exact.
- Query semantics: `lexical`, `semantic`, `hybrid`. Semantic queries fail explicitly if model unavailable; no false success or empty-result disguise.
- Other text files: **simple lexical search** (UTF-8 text, line references) unless they are Spring configs; don't pretend arbitrary text has Java symbols.
- No hidden/skipped acceptance tests, no unit-test detour, no weakening gate thresholds, no LLM-generated code descriptions.
- KISS, DRY, SOLID, clean code: favor small cohesive packages and plain Go; add interfaces only at meaningful boundaries.

## Primary technical references
- Tree-sitter Go: https://github.com/tree-sitter/go-tree-sitter
- Ollama embed API: https://docs.ollama.com/capabilities/embeddings
- Model: https://ollama.com/library/qwen3-embedding:0.6b
- Spring Boot config: https://docs.spring.io/spring-boot/reference/features/external-config.html
- Agent Skills: https://agentskills.io/specification
- Agent CLI skill: `skills/cx-cli/SKILL.md`
