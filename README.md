# cxj

**Stateless Java/Spring code intelligence for coding agents.**

[![Release](https://img.shields.io/github/v/release/umuterturk/cxj?sort=semver)](https://github.com/umuterturk/cxj/releases)
[![CI](https://github.com/umuterturk/cxj/actions/workflows/build.yml/badge.svg)](https://github.com/umuterturk/cxj/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go: 1.23+](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![npm](https://img.shields.io/npm/v/@umutert/cxj)](https://www.npmjs.com/package/@umutert/cxj)
![Platforms: macOS / Linux](https://img.shields.io/badge/platforms-macos_%E2%80%A2_linux-lightgrey)

`cxj` answers "who calls this", "what does this endpoint touch", "which version am I
really on" — about **your** Java/Spring codebase — in milliseconds, from a cold start,
with no daemon, no embeddings, and no on-disk index.

The law of the project: **speed comes from doing less work, never from state.** Every
invocation is a cold `Open → query → exit`. Nothing survives the process, so there is
nothing to invalidate, nothing to serve you a stale answer, and nothing to trust wrong.

## Why

Coding agents navigate Java by `grep` and hope — which means they miss the call your
type-checker could prove exists, and hallucinate the one that was deleted last week.
The established answers are all stateful: an LSP daemon to start and keep warm, a
semantic index to rebuild when it drifts, embeddings to re-embed per branch. Each adds
a freshness tax every agent session pays.

`cxj` is the stateless point on that design curve — **Context7 tells your agent about
your libraries; `cxj` tells it about your code:**

- **Cold is the baseline, not the exception** — a 900-file Spring service answers
  `brief` in ~150 ms and `trail` in ~100 ms, process startup included, measured and
  gated in CI.
- **Parse-level truth, not text matching** — tree-sitter over your sources; `callers`
  matches the receiver's *declared type*, not its name's suffix.
- **Every answer ships its certainty** — `exact` / `candidate` / `ambiguous N:` labels
  and explicit `(+K more)` caps, so an agent knows exactly when to double-check.
- **One skill, every harness** — `cxj install -g` writes to `~/.agents/skills`, which
  OpenCode, Claude Code, and other agent harnesses discover natively. No MCP server to
  babysit.

## Quickstart

```sh
brew install umuterturk/tap/cxj   # or: npx @umutert/cxj · go install · Releases
cd /path/to/your/spring-service
cxj brief
```

```
kind=spring java=21 spring_boot=3.4.1
main_files=895 types=691 test_classes=493
pkg com.example.orders.application 73
http POST /v1/checkout
http GET /v1/orders/{id}
out_http PaymentGatewayClient,ShippingClient
```

Then navigate like an IDE, from the shell:

```sh
cxj trail com.example.orders.CheckoutController.checkout --scope all
cxj callers OrderService.reserve --scope all
cxj endpoints | cxj deps | cxj api spring-kafka
```

Wire your agent in with one more command:

```sh
cxj install -g    # symlinks cxj + writes the agent skill to ~/.agents/skills/cxj
```

The skill teaches the agent the loop — `brief` → `trail`/`callers` → `show` — the
scope rules, the certainty labels, and `batch` mode (twenty lookups for the price of
one cold open). From then on it navigates your repo instead of grepping it.

## Commands

| Command | Question it answers |
| --- | --- |
| `brief` | What is this repo, in 20 lines? (framework, versions, packages, endpoints, clients) |
| `find <query>` | Where does `<query>` live? (substring + camel-hump, main-before-test) |
| `trail <symbol>` | The symbol's full card: definition, route, callers, callees, subtypes, tests |
| `callers` / `callees <symbol>` | Who really calls this / what does it call? (typed receiver match) |
| `references <type>` | Which files touch this type? |
| `show` / `outline <type>` | The file / just its signatures |
| `endpoints` | Every HTTP endpoint with owner and certainty |
| `keys` / `config` | Config property names / where they're read |
| `beans` / `context` | Spring beans by type / the context around a symbol |
| `deps [filter]` | Which version am I really on — from build files + local caches, no network |
| `api <dep> [Type]` | That dependency's real API *at your resolved version*, plus your call sites |
| `map` | The whole repo as a symbol map |
| `batch` | Twenty JSONL queries, one cold open (~17× cheaper than separate calls) |
| `ask <question>` | Natural-language layer; optional — prompts to download its ~35 MB files once |

All of them: `--root` any repo, `--json` for machine output, `--example` for a demo.
`cxj COMMAND --help` is the reference; the output itself is the contract.

## The output contract

Two rules make every answer safe to act on, and both are enforced by golden tests, not
vibes:

- **Never a silent guess.** A call edge counts only if the receiver's declared type
  proves it. Chains no declaration can establish (builders, Mockito `verify(...)`)
  come back labeled `candidate`; a name matching N symbols returns `ambiguous N:` and
  refuses to pick.
- **Never a silent cap.** Truncated sections print `(+K more)`; scope is always
  stated — navigational commands index `src/main` only by default (on a miss, cxj
  tells you to retry `--scope all`).

The corollary, learned the expensive way and now law: an HTTP endpoint has no code
callers; `trail CardPaymentClient.pay` and `trail PaymentController.pay` are different
cards even though the methods share a name. Owner-exact or nothing.

## Performance

Measured on a deterministic ~850-file fixture (CI-**gated**: CPU and RSS fail the
build on regression) and reported on real repos. Apple M3 Pro, cold process:

| | fixture | payment-api (849 files) | pos-integration-api (1,354 files) |
| --- | --- | --- | --- |
| `brief` | 150 ms | 150 ms | 251 ms |
| `trail --scope all` | 190 ms | ~150 ms | 313 ms |
| `batch` ×20 | — | ~400 ms | **265 ms** (≈13 ms/query) |
| `deps` | no index open | 12 ms | 13 ms |

```sh
./scripts/perf-bench.sh                            # fixture mode; fails on regression
./scripts/perf-bench.sh --repo /path/to/java/repo  # report-only on your repo
./scripts/accuracy-bench.sh --repo /path/to/repo   # the accuracy exit checks
```

`wall_ms` belongs to the machine that captured it, so wall is gated on developer
machines and reported (not gated) on shared runners; CPU/RSS are gated everywhere.
Baselines live in `scripts/perf-baseline.txt`; every CI run posts its numbers.

## Accuracy engineering

cxj's accuracy claims were audited against grep ground truth on production Spring
repos before they were believed — [docs/FIX_PLAN.md](docs/FIX_PLAN.md) is the public
record: 20 phases, every finding (like `callers` once returning 106 false positives)
kept in the repo as the collision-shaped golden test that now prevents its return:
`internal/engine/trail_golden_test.go`, `internal/index/typedcall_test.go`, and
friends. The rule the whole suite encodes: **a fix ships with the test that fails on
the old binary.**

## Install

| Channel | Command |
| --- | --- |
| Homebrew | `brew install umuterturk/tap/cxj` |
| npm | `npm i -g @umutert/cxj` · one-off: `npx @umutert/cxj` |
| Go | `go install github.com/umuterturk/cxj/cmd/cxj@latest` (needs a C toolchain for tree-sitter) |
| Release assets | `macos-arm64` · `macos-x86_64` · `linux-x86_64` · `linux-arm64` zips — [Releases](https://github.com/umuterturk/cxj/releases) |

`cxj update` upgrades in place from the newest release asset for your platform.
Windows is not supported (the agent-skill targets are macOS/Linux).

## Build from source

```sh
go build -o bin/cxj ./cmd/cxj
./scripts/bundle.sh          # bundle the agent skill next to the binary
go test ./... && ./scripts/perf-bench.sh
```

## Documentation & status

- [CHANGELOG.md](CHANGELOG.md) — every release, organized from the commit history
- [docs/FIX_PLAN.md](docs/FIX_PLAN.md) — the full audit-to-fix record (P/A/B tracks)
- The bundled agent skill — [`internal/skill/SKILL.md`](internal/skill/SKILL.md), what `cxj install` writes

**Status:** v0.3.x — the navigation, dependency, and distribution surfaces are all
shipped and CI-gated; the tool is in daily use on production Java 25 / Spring Boot 4
services. Bugs: [issues](https://github.com/umuterturk/cxj/issues) — measured
reproductions (command, repo shape, expected vs actual) get fixed fastest, because
that's exactly how this project's own bugs were found.

## License

MIT — see [LICENSE](LICENSE).
