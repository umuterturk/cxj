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

## Five checkable promises

Every line below is a command you can run, not an adjective:

1. **The only latency is one cold read of your repo.** `brief` answers in
   30–250 ms on 900-file production Spring services — and in **2.4 s on Spring
   Framework itself** (1.53M lines, 9,295 files), from a standing start. There is no
   first-session index to wait for and nothing to keep warm, because there is no
   index and nothing to warm.
2. **Trusts the code. Only the code.** tree-sitter parses your sources; `callers`
   resolves the receiver's *declared type*; every answer carries its `exact` /
   `candidate` / `ambiguous` label and its `(+K more)` caps. No embeddings, no
   ranking model, no LSP heuristics — nothing that can be plausibly wrong.
3. **Read-safe and write-safe.** Query commands read your repo and write nothing:
   no index on disk, no cache, no telemetry, no network — your source never leaves
   the machine (`install` and `update` are the only commands that touch disk or the
   net, and you type those). Safe to point at anything. And safe to edit *against*:
   answers are re-derived from the bytes on disk now, so an agent's next change is
   never grounded in a stale view.
4. **The pragmatist's bet: your repo is not the Linux kernel.** Persistent graphs
   are infrastructure for 28M-LOC outliers and their minutes of indexing. The real
   Java world lives under a million lines — and the biggest "normal" repo just past
   that ceiling reads cold in 2.4 seconds. Below the ceiling, state buys a freshness
   tax and nothing else.
5. **Spring is the domain model, not a keyword.** HTTP routes, beans and
   qualifiers, profiles, config keys, and dependency versions *as your build
   actually resolves them* are first-class questions with first-class answers — not
   generic symbol nodes wearing a Spring hat.

## Why

Coding agents navigate Java by `grep` and hope — which means they miss the call your
type-checker could prove exists, and hallucinate the one that was deleted last week.
The established answers are all stateful: an LSP daemon to start and keep warm, a
semantic index to rebuild when it drifts, embeddings to re-embed per branch. Each
adds a freshness tax every agent session pays — payment in staleness risk for
per-query savings no repository under a million lines needed.

`cxj` is the stateless point on that design curve — **Context7 tells your agent about
your libraries; `cxj` tells it about your code** — and it walks in through the door
every harness already holds open: `cxj install -g` writes one skill to
`~/.agents/skills`, which OpenCode, Claude Code, and the rest discover natively, and
`/plugin install cxj --marketplace umuterturk/cxj` does the same inside Claude Code.
No MCP server to babysit.

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
one cold open). From then on it navigates your repo instead of grepping it. The same
skill ships through the harness-native channels too: `/plugin install cxj
--marketplace umuterturk/cxj` inside Claude Code, or `npx skills add
umuterturk/cxj` across agents — see [Install](#install).

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

Scale ceiling, for the pragmatist's bet: on **Spring Framework** (1,527,748 LOC,
9,295 files — far past the 1M-line line almost every real repo stays under), the
cold process still answers `brief` in 2.4 s and `trail` in 1.7 s. Stateless holds
well past the size where anyone claims it must stop holding.

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
| Claude Code | `/plugin install cxj --marketplace umuterturk/cxj` — or the two-step: `claude plugin marketplace add umuterturk/cxj` then `claude plugin install cxj@cxj` |
| Any agent, via skills.sh | `npx skills add umuterturk/cxj` (Claude Code, Codex, Copilot, OpenCode) |
| Release assets | `macos-arm64` · `macos-x86_64` · `linux-x86_64` · `linux-arm64` zips — [Releases](https://github.com/umuterturk/cxj/releases) |

Every channel delivers the same binary and the same
[agent skill](internal/skill/SKILL.md); `cxj install` is the do-it-yourself
version (writes `.agents/skills` and symlinks `.claude/skills`; `-g` does the
same under `$HOME`). `cxj update` upgrades in place from the newest release
asset for your platform. Windows is not supported (the agent-skill targets are
macOS/Linux).

### Verify the binaries

Assets are built from the tagged source on public GitHub Actions runners
([release.yml](.github/workflows/release.yml)) and smoke-tested on their own
runner before upload. The zips are unsigned, so their SHA-256 digests are the
contract — published here and pinned by the Homebrew formula:

| v0.4.0 asset | SHA-256 |
| --- | --- |
| `cxj-macos-arm64.zip` | `f557ad67610d6b08fb35002c332efa6605f1cd61a3c02f72885a8f4e9c2cd0ee` |
| `cxj-macos-x86_64.zip` | `3c7a66fb5121611276c31543644c17dada60fd8ea3d44cadd8131dcf6dfe3a8e` |
| `cxj-linux-x86_64.zip` | `aebce3c75b6a2b395c93a42c6d9a894a98be96bd611627a0f6db444df267f75b` |
| `cxj-linux-arm64.zip` | `986cd706aaa885def59703894cb85a928641b141c90d94fac343a8a02fa0a91e` |

```sh
shasum -a 256 cxj-macos-arm64.zip    # macOS
sha256sum   cxj-linux-x86_64.zip     # Linux
```

macOS Gatekeeper: the quarantine attribute is added by browsers, so a
zip-downloaded binary is the one channel that may prompt "unidentified
developer" on first run — clear it with
`xattr -d com.apple.quarantine ./cxj`. Homebrew and npm fetch the binary
without ever acquiring the attribute; nothing to do there.

### Uninstall

`brew uninstall umuterturk/tap/cxj` · `npm rm -g @umutert/cxj` ·
`rm "$(which cxj)"`. The only other footprint is the skill folders:
`.agents/skills/cxj` (plus the `.claude/skills` symlink) and/or
`~/.agents/skills/cxj`. That is everything — cxj keeps no state, so there is
nothing else left to remove.

## Build from source

```sh
go build -o bin/cxj ./cmd/cxj
./scripts/bundle.sh          # bundle the agent skill next to the binary
go test ./... && ./scripts/perf-bench.sh
```

## Documentation & status

- [CHANGELOG.md](CHANGELOG.md) — every release, organized from the commit history
- [docs/FIX_PLAN.md](docs/FIX_PLAN.md) — the full audit-to-fix record (P/A/B tracks)
- [docs/MARKET_PLAN.md](docs/MARKET_PLAN.md) — the public distribution + marketing plan
- The bundled agent skill — [`internal/skill/SKILL.md`](internal/skill/SKILL.md), what `cxj install` writes

**Status:** v0.4.x — the navigation, dependency, and distribution surfaces are all
shipped and CI-gated; the tool is in daily use on production Java 25 / Spring Boot 4
services. Bugs: [issues](https://github.com/umuterturk/cxj/issues) — measured
reproductions (command, repo shape, expected vs actual) get fixed fastest, because
that's exactly how this project's own bugs were found.

## License

MIT — see [LICENSE](LICENSE).
