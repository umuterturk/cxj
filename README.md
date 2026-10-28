# cxj

One-shot Java/Spring code intelligence CLI. Indexes `--root` (default: cwd) in RAM and exits. No daemon, no embeddings, no on-disk index.

Speed is the moat, and it comes from doing less work — never from state: no cache survives the
exit, so there is nothing to invalidate and nothing to trust wrong. Cold `Open → query → exit`
is the product, and every metric that proves it is gated in CI.

`ask` is optional. If Needle 3 files are missing, `cxj ask` prompts to download them (~35 MB) into `needle/` (or next to the binary). Other commands do not need Needle.

## Install

Homebrew (macOS and Linux):

```sh
brew install umuterturk/tap/cxj
cxj install -g   # /usr/local/bin/cxj + global agent skill
```

Go (any platform Go supports — builds from source, needs a C toolchain for tree-sitter):

```sh
go install github.com/umuterturk/cxj/cmd/cxj@latest
```

Direct download — release assets for `macos-arm64`, `macos-x86_64`,
`linux-x86_64`, `linux-arm64` from [Releases](https://github.com/umuterturk/cxj/releases):

```sh
# unzip the zip for your platform, then:
./cxj install -g
```

`cxj update` later upgrades in place: it fetches the newest release asset for
your platform and swaps the binary. `cxj install -g` writes the agent skill to
`~/.agents/skills/cxj`, which OpenCode, Claude Code, and other agent harnesses
discover natively.

## Build

```sh
go build -o bin/cxj ./cmd/cxj
./scripts/bundle.sh
```

```sh
./bin/cxj install      # project skill (.agents + .claude symlink)
./bin/cxj install -g   # global skill + /usr/local/bin/cxj
./bin/cxj brief --root /path/to/workspace
./bin/cxj ask 'who calls OwnerController' --root /path/to/workspace
```

## Commands

```
cxj brief | find | show | outline | map | trail
cxj callers | callees | references | keys
cxj endpoints | beans | config | context | status
cxj deps [FILTER]        # dependency versions from your build files + local caches (no network)
cxj api DEP [Type]       # a dependency's API at YOUR resolved version, + how your repo uses it
cxj batch                # many JSONL queries, one Open; answers in request order
cxj ask                  # optional; prompts to download Needle 3 if missing
cxj COMMAND --example    # 2-3 workspace examples
cxj install | install -g | version | update
```

Navigational commands (`show`/`outline`/`trail`/`callers`/`callees`/`references`/`endpoints`/`beans`/`config`/`keys`) index **main sources only** by default — skipping `src/test` cuts CPU ~3-5x on large repos. Pass `--scope all` (or `--scope test`) to change this; on a miss the CLI hints you to retry with `--scope all`.

Two honesty rules worth knowing:

- **`callers`/`references` match the receiver's declared type.** `callers C.m` lists calls on a receiver typed `C` (or a subtype), plus name-only matches whose receiver type no declaration can establish (chained calls — labeled `candidate`). An HTTP endpoint honestly has ~no code callers; its route is in its `trail` `http` row.
- **Truncation says so.** `trail` sections cap at 5 and print `(+K more)`; `ambiguous N:` means your query matched N same-named symbols — qualify it and re-run.

## Benchmarks

Measured on the deterministic fixture (`internal/benchfixture`, ~850 Java files — comparable
across machines, so this mode is GATED) and reported on real repos. Latest numbers, Apple M3
Pro, cold process:

| command | fixture wall | fixture cpu | payment-api wall |
|---|---|---|---|
| `brief` | 150 ms | ~1280 ms | 150 ms |
| `find` (main) | 70 ms | ~450 ms | 100 ms |
| `find --scope all` | 140 ms | ~1180 ms | 140 ms |
| `keys` | 40 ms | ~300 ms | 60 ms |
| `trail` (main) | 90 ms | ~710 ms | 100 ms |
| `trail --scope all` | 190 ms | ~1830 ms | — |
| `deps` | — | ~0 ms (opens no index) | 12 ms |

Reproduce them:

```sh
./scripts/perf-bench.sh                             # fixture mode; fails on regression
./scripts/perf-bench.sh --repo /path/to/java/repo   # report-only on a real repo
./scripts/accuracy-bench.sh --repo /path/to/repo    # report-only §4.4 exit checks (see below)
go test ./internal/engine -run '^' -bench Benchmark -benchtime=3x
```

The gate policy: `cpu_ms` and RSS are gated everywhere; `wall_ms` belongs to the machine that
captured the baseline, so it is gated on developer machines and only reported on shared runners.
Baselines live in `scripts/perf-baseline.txt`; move one with `--write-baseline` and review the
diff. Every CI run posts its numbers as a comment on the commit.

Accuracy is benchmarked too: `scripts/accuracy-bench.sh` runs the §4.4 exit criteria from
[docs/FIX_PLAN.md](docs/FIX_PLAN.md) against a real repo (report-only — real repos drift, so
the law is gated by the deterministic collision-shaped tests instead:
`internal/engine/trail_golden_test.go`, `internal/index/typedcall_test.go`,
`internal/exclude/exclude_parity_test.go`, and friends).

## What changed, and why (2026-10-28)

[docs/FIX_PLAN.md](docs/FIX_PLAN.md) is the full audit-to-fix record: 20 phases across three
tracks — performance (P), navigation accuracy (A), and dependency surface (B). Highlights:

- **P:** per-command capability trimming, main-scope default, one file read instead of two,
  a collapsed exclude matcher, parallel index builds — `keys` at 40 ms, and a `batch` mode
  that answers twenty questions for one `Open`.
- **A:** `find Controller` stopped returning nothing; `trail` stopped matching the package
  name; `callers …PaymentController.pay` went 106 → honest (106 calls to *other* classes'
  `pay` methods; nothing in the repo calls its own HTTP endpoint).
- **B:** `deps` and `api` answer "which version am I really on" and "what is its real API"
  from your build files and local caches, no network, no corpus, no daemon.
