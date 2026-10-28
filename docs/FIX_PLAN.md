# cxj — performance + accuracy + the Context7 play (master plan)

Three goals, one law. In priority order:

1. **Performance is the product.** cxj is stateless, so every agent call pays a full cold
   `Open → query → exit`. On the multi-core laptops agents actually run on, one Open must be
   as cheap as possible, or the tool loses. This is the moat: a local Open is milliseconds;
   Context7 is a network round-trip. **Track P (§3) is the highest-priority track.**
2. Make navigation (`trail`/`callers`/`callees`/`find`) correct in large Spring codebases
   (Track A, §4).
3. Compete with Context7 on the thing it cannot do statelessly: **ground-truth about the
   dependencies this project actually uses, offline, from disk**, plus how this repo already
   calls them (Track B, §5).

**The law: cxj has no state.** One-shot `Open → query → exit`. No daemon, no listening
socket, no persisted index, no embeddings, no crawl, no hosted corpus, no network on the hot
path. Every change below — perf, bug fix, or new feature — must pass the §2 compliance
checklist. Note especially: **speed must come from doing less work and escaping the serial
path, never from a persisted cache** — caching is precisely the state we refuse to hold, and
staying stateless is the differentiator vs hosted tools.

Grounded in `/Users/umut.erturk/mycode/cxj` and a reproduction against a real service
(`payment-api`, Java 25 / Spring Boot 4.1, 561 main files, 539 types, 384 test classes).

> Note: verified 2026-10-28 against Upstash's public materials. Context7 (Upstash) is a
> **hosted index of first-party, library-owner-published docs**, served two ways: the MCP
> server (two read-only tools) and, since Sep 2026, a **Context7 Search API** — a single REST
> grounding GET (`context7.com/api/v3/search`), billed as "Exa for code," with a TypeScript SDK
> and a free tier (1,000 calls/mo; Pro 2,000/seat then $5/1k). It supports `library`/`language`
> hints, a `version=` param, per-snippet `codeTokens` counts, source attribution, and
> pre-index malware/prompt-injection scanning. This is a genuinely strong product; the
> comparison below is honest about where it wins and where cxj's statelessness wins.

---

## 1. Where the competition actually is

Context7 answers: *"what is the current, version-specific API/example for library X?"* by
serving a **hosted index of official, library-owner-published docs** (first-party docs sites and
API references only — no forums/SO), over MCP or the new Search API. That needs the network,
their service, their indexing coverage, and it rate-limits (their own docs describe 429
cooldowns).

cxj's wedge is the opposite posture, and it is defensible **because cxj has no state**:

| | Context7 (hosted doc index) | grep / IDE | **cxj (stateless, local)** |
|---|---|---|---|
| Your **own code** graph | no | partial (IDE stateful) | **yes, lexical, one-shot** |
| Dependency **resolved version** | `version=` is a hint; reflects indexed docs | build-tool only | **derived from your build + local cache** |
| Third-party **API for your version** | official-doc snippets | only if indexed | **read from your resolved jar/sources** |
| **Your** usage examples | generic snippets | manual | **real call sites from your repo** |
| Offline / air-gapped / CI | no (needs service + network) | yes | **yes** |
| Reproducible (same input → output) | no (index + model drift) | — | **yes** |
| Rate limits / per-seat cost | yes (429 cooldowns; call billing) | none | **none (local)** |
| Needs a running server | yes (MCP / hosted API) | IDE process | **no — one-shot CLI + skill** |
| Source leaves machine | your queries hit a third party | — | **never** |
| Broad curated first-party docs | **yes (thousands of libs)** | no | no (cxj won't crawl — no-state) |
| Injection-scanned, attributed snippets | **yes** | — | n/a (reads your own files) |

Where **Context7 wins**: breadth of curated, first-party documentation for arbitrary
libraries, with attribution and safety scanning — cxj deliberately does not crawl docs (that
would break the no-state law). Where **cxj wins**: everything *project-specific* — the actual
repo graph, the *build-resolved* dependency version, the API of the *exact artifact you
compiled against*, and your *own* usage of a library — all offline, deterministic, free, with
your source never leaving the machine. They are complementary; cxj's bet is that the
highest-value grounding is your project's ground truth, which a hosted doc index cannot supply.

The two new features (§5) turn that wedge into commands: `deps` (version ground truth) and
`api` (your-version third-party API + your usage). They compose: `api` uses `deps` to know
which jar to open.

The §4 accuracy work is **table stakes**: to be the workspace-knowledge tool, the graph has to
be trustworthy. Context7 doesn't touch your code at all, so a correct graph is pure advantage.
None of this is credible unless §3 makes the cold path fast — a slow tool is not consulted.

---

## 2. No-state law — compliance checklist (every PR passes this)

A change is compliant only if all hold:
- [ ] Derives everything on demand from files that already exist (repo, build files, the
      user's own local caches). Persists nothing; writes nothing during a query command.
- [ ] Opens no socket / starts no background process. Ships as a one-shot subcommand.
- [ ] Adds no embeddings, no vector store, no crawl, no hosted corpus.
- [ ] Network stays off the hot path. `ask`/Needle remains the only optional LLM path and is
      never a dependency of these commands.
- [ ] Same inputs → same outputs (no server drift, no hidden cache written by cxj).

The one place this is argued: reading `~/.m2`/`~/.gradle` is **the build tool's** cache, not
cxj's. cxj reads it read-only, exactly like it reads the repo; it writes nothing there. That
keeps "cxj has no state" true.

Optional guardrail (nice-to-have): a CI test asserting query subcommands create no files under
a temp home and bind no ports.

---

## 3. Track P — performance (highest priority)

The law forces a full cold `engine.Open` per command. An agent makes dozens of calls per task,
so per-invocation wall time, CPU, and battery are the whole game. **Measured baseline** —
`payment-api` (561 main / 849 total Java), Apple M3 Pro (6P+6E), warm file cache, v0.2.3,
seconds:

| Command | GOMAXPROCS | wall | cpu | note |
|---|---|---|---|---|
| `cxj version` (no scan) | default | ~0.0 | ~0 | startup is free; all cost is Open |
| `find` full Open | 1 | 3.29 | 3.24 | serial reference |
| `find` full Open | 4 | 0.88 | 3.21 | |
| `find` full Open | 8 | 0.63 | 3.91 | plateaus |
| `find` full Open | 12 | 0.63 | 4.48 | more cores → **more CPU, same wall** |
| `trail` full Open | 12 | 0.62 | 4.4 | same profile |
| `find --exclude src/test` | 12 | **0.15** | **0.85** | 4.2× wall, 5.3× CPU |

Read the table: peak RSS is only ~95 MB, so we are CPU/latency-bound, not memory-bound. Scaling
tops out ~0.6s (≈5× on 12 cores ⇒ ~19% serial, Amdahl-bound), and cores past ~4 add CPU with no
wall win — on the P/E laptops agents run on, that surplus is efficiency cores doing nothing for
latency while burning battery. **The wins are: parse fewer files, cut total CPU, and shrink the
serial fraction — not more cores.**

### 3.1 Root causes of cost (code-anchored)
- **P-RC1 whole-workspace parse per command.** `engine.Open` → `scan.Scan` walks the tree,
  reads every file, and runs `java.Extractor` on **all** Java including `src/test` (engine.go).
  A `trail`/`callees`/`endpoints` query almost never needs test files, yet pays ~3× to parse them
  (they are invocation-dense: dense `calls` edges + text lines).
- **P-RC2 every sub-index is always built. ✅ fixed by P2.** Non-`BriefOnly` Open built symbol +
  text + binding + spring + config unconditionally. `show`/`outline` need only definitions;
  `endpoints`/`beans` need only spring; `keys`/`config` need only config. Over-building wasted CPU
  and GC on the hot path. Now a `Capabilities` bitmask derived from the command name
  (`internal/engine/caps.go`), resolved per file *before* `ReadFile`.
- **P-RC3 serial fraction caps scaling.** `scan.Scan` is a single-threaded `filepath.WalkDir`,
  and the post-walk accumulation + `index.Build` + `sort.SliceStable` in `mergeLexical` run
  largely serially (scan.go, engine.go). ~19% serial ⇒ ~5× ceiling regardless of core count.
- **P-RC4 linear line-scanning. ✅ fixed by P5.** `lineAt` rescanned from byte 0 for every
  invocation/definition, and `spanOf` called it twice. Quadratic in file size, and it turned out
  to be **36.5 % of a cold Open on a real repository** (8 % on the fixture — see §3.6). Fixed by
  reading the row tree-sitter already computes, not by indexing newlines: three private copies of
  `lineAt` became one `tshelp.Line`. The plan's original prescription ("a per-file cumulative
  newline index") would have worked and was the wrong instinct: the information was already there.
- **P-RC5 no work amortization across an agent turn.** Stateless by law, so N commands = N
  Opens. That is fixable *within* the law by batching queries in one process (§3.2 P4).

### 3.2 Phases (ordered by wall-time win per unit risk; every PR keeps the §3.3 budget green)
- **P0 — Benchmark harness (prerequisite, gates everything). ✅ shipped.** Landed as:
  `internal/benchfixture` (deterministic synthetic Spring workspace, one definition shared by
  every measurement — sized to `payment-api`: 560 main / 290 test java files),
  `internal/engine/bench_test.go` (`BenchmarkOpen`/`OpenMain`/`Brief`/`Trail`/`Callees`/
  `Endpoints`, each reporting the workspace's java-file count so no number can be read without
  its denominator), and `scripts/perf-bench.sh` (end-to-end: process start + cold Open + one
  query, reporting median wall, median cpu and max RSS). CI runs both (`.github/workflows/build.yml`,
  `perf` job) and the script **fails the build** past baseline × tolerance (1.25 local, 2.0 on
  shared runners) plus an absolute RSS ceiling. Baseline is committed at
  `scripts/perf-baseline.txt`, so every intended perf move is a reviewed diff. Two honest notes:
  real repositories are measured **report-only** (laptop-to-laptop variance is too high to gate
  on), and the gate was verified by forcing it red (16 `FAIL` lines, exit 1) as well as green.
  (It also gated `wall_ms` on CI, which the first P2 push proved unworkable on shared runners —
  see §3.3.1.)
- **P1 — Parse less: skip test sources unless needed (P-RC1). ✅ shipped.** `--scope all|main|test`
  with main as the default for navigational commands; the filter runs *before* `ReadFile`, so it
  skips the I/O and the Tree-sitter parse, not just the indexing. A miss on a main-scoped query
  prints `note: indexed main sources only; retry with --scope all` on stderr (never in JSON), so
  a test symbol can't silently read as "not found". `find`/`map`/`brief`/`ask` keep everything —
  `brief` deliberately stays full-scope or its `TestClasses` count would read zero, which
  `TestBriefKeepsTestClasses` now guards.
- **P2 — Sub-index selection (P-RC2). ✅ shipped.** `Options.Caps` carries a `Capabilities`
  bitmask and `Open` builds only what is asked for, generalized from `BriefOnly` as planned. Three
  things the one-line version of this phase did not anticipate:
  - **The set is derived from the command name and nowhere else** (`CapsFor(cmd)` → one table). A
    phase that handed each call site its own bit list would have drifted the moment a command grew
    a new query, and drift in this table is invisible: the fallback for an unknown command is
    `CapsAll`, deliberately correct-but-slow. `TestEveryCommandDeclaresItsCapabilities` closes the
    loop by scanning `main()`'s command switch and requiring every workspace-opening command to
    appear in the table (verified red by deleting a row).
  - **Skipping the parse, not just the index.** Each worker resolves a per-file work set and
    `continue`s **before `ReadFile`**, so `keys` neither opens nor parses the 850 Java files it
    never reads, and loads zero Tree-sitter grammars instead of three.
  - **`CapSymbolIndex` had to be split out of `CapJava`.** Building every index unconditionally —
    the safe-looking way to avoid nil-index panics — made `brief` 6 % slower and doubled its RSS
    (44 MB → 83 MB), because `brief` reads per-file *facts* for its counts and never queries the
    symbol index. Facts and index are different needs. Panic-safety is kept by building an *empty*
    index when the bit is off: `TestNoCapabilitiesLooksLikeAnEmptyWorkspace` asserts a
    misdeclared command answers exactly what it would answer on an empty repo, so it degrades to
    silence rather than to a panic.
  **Exit — met.** A/B of the pre-P2 binary against this one on the same workspace: `keys`
  1090 → 300 ms cpu (−72 %), `endpoints` 1090 → 339 (−69 %), `show` 1120 → 660 (−41 %), `outline`
  1120 → 660 (−41 %), all ≥ 30 %, RSS 42 MB → 20 MB on the config/spring paths. One correction to
  the exit wording: `show`/`outline` cannot skip the *symbol* index, because resolving a symbol
  query is what the symbol index does — they skip spring, config, bindings and text lines.
  `endpoints`/`beans`/`keys`/`config` skip symbol, text and bindings entirely.
  **Correctness proof** (the part that matters more than the numbers): every trimmed reply must
  equal the reply from a full build. `TestTrimmedCapsAnswerIdentically` runs 22 command-shaped
  questions through `CapsFor(cmd)` and a full build at both scopes and requires `reflect.DeepEqual`
  — 44 cases, all equal. Each case is first required to answer *differently than it answers an
  empty workspace*, since two empty answers agree with each other and prove nothing.
  End to end, 28 invocations of the two binaries over `payment-api` (both scopes, text and JSON)
  are byte-identical; the only field that moved anywhere is `status --json`'s
  `go_heap_alloc_bytes`, 44.9 MB → 3.1 MB.
- **P3 — Attack the serial fraction (P-RC3).** Parallelize independent post-parse work (per-file
  facts already arrive in slots — build symbol/text/spring/config shards concurrently and merge);
  pre-size and parallelize sort where order-independent. Target serial < 8% ⇒ ~9× on 12 cores,
  but the real goal is lower wall at GOMAXPROCS=4 (typical ultrabook) and lower CPU at all sizes.
- **P4 — Batched queries in one process (P-RC5, still stateless). ✅ approved as no-state-legal,
  shipped 2026-10-28 as `cxj batch`.** One process opens once, answers many stdin queries, persists
  nothing, exits. JSONL in (`{"cmd":"find","arg":"Order"}` per line), JSONL out in request order,
  plus a summary line whose `open_ms` beside `wall_ms` is the amortization made visible. Reading
  stdin *before* opening lets the batch union `CapsFor` of its members — the P2 trick per batch.
  Scope is one batch-level decision (the snapshot cannot be scoped per line); it defaults to the
  breadth of the widest member so batch answers never silently differ from standalone — and they
  do not: every batch answer is byte-identical to the command's own `--json`.
  - **Exit — met at 2.2× the bar.** 20 mixed lookups (show/outline/trail/callers/callees/
    references/endpoints/keys/config/status) on `payment-api`: **150–160 ms wall** vs the ≤ 350 ms
    target and ≈ 3 s for twenty separate invocations. The summary's split — `open_ms` 148 of
    `wall_ms` 160 — says 92 % of a batch is now the thing only an Open can do; the queries
    themselves cost 12 ms total, which is P7 paying P4 back.
- **P5 — Cheap algorithmic wins (P-RC4). ✅ shipped, and the plan's prescription was wrong.**
  The phase was written as "per-file newline offset table (replace `lineAt` linear scans)". The
  better fix was to stop computing line numbers at all: tree-sitter already knows the row every
  node starts on, so `tshelp.Line(n)` reads it back for the cost of one call. A newline index
  would have been a new data structure doing work the parser had already done.
  - Three private `lineAt` copies (java, spring, config) — the A-RC10 shape again — became one
    helper. They did not even agree with each other: java's decoded UTF‑8 rune by rune while
    counting newlines (byte-scanning is safe: `0x0A` is never a UTF‑8 continuation byte), the
    other two byte-scanned. That made the slowest copy of the three also the wrongest-looking one.
  - **Premise pinned, not assumed.** Deleting a line-number implementation is only safe if a
    parser row *is* the line number, so `internal/java/rowparity_test.go` compares the two over
    every node of every file — **641,176 positions across 849 real sources, plus start and end
    rows on CRLF files, multi-byte identifiers, text blocks, a BOM and a file with no final
    newline**. The test's oracle is a deliberately dumb newline counter that lives in the test
    file, so it still proves something after the implementation it guards is deleted.
  - **Exit — met at 2.5× the bar.** "cpu for a full 849-file Open drops ≥ 25% at `GOMAXPROCS=1`":
    measured **2921 ms → 1093 ms = 2.67× (−63 %)**, single core, on `payment-api`, with
    allocations identical to the object (1,869,699 vs 1,869,698), so it is provably less work and
    not a GC or scheduling artifact.
  - Not shipped from this phase's text: "reuse the Tree-sitter tree between definition-walk and
    `walkInvocations`". They already share one `Extract` parse and one traversal, so there was
    nothing to reuse; `walkInvocations` walks the same tree, it does not re-parse it.

### 3.3 Performance budget (CI-enforced ceilings, not aspirational)
On the pinned repo, at `GOMAXPROCS=8`, warm cache, p95 of 10 runs:

| Command class | wall ceiling | cpu ceiling |
|---|---|---|
| process startup (`version`) | < 20 ms | < 20 ms |
| scoped command (`show`/`outline`/`endpoints`/`keys`) | < 200 ms | < 1.0 s |
| navigational (`find`/`trail`/`callers`/`callees`) | < 300 ms | < 1.5 s |
| batch of 20 lookups (one Open) | < 400 ms | < 2.0 s |
| peak RSS (any command) | < 200 MB | — |

Rationale: an agent doing ~30 lookups/task should spend **well under a second total**, not
tens of seconds. These ceilings are set *after* P1 so they are achievable, and are checked in
CI as a hard gate.

#### 3.3.1 What CI actually gates (found the hard way)

The first CI run of the P2 push failed the `perf` job while every test passed, and the failure was
undiagnosable from outside: **Actions job logs and artifacts both answer 403 without a token.**
Two separate fixes, one for the gate and one for the blindness.

The gate failed on `wall_ms`, and the cause is the machine. Reproducing the only part of CI's
condition that can be reproduced locally — same machine, same binary, same baseline, just
`GOMAXPROCS=4` — every `cpu_ms` came in **under** the tight 1.25 local tolerance while **every**
`wall_ms` blew through it:

| Case | baseline wall | @4 cores | ratio | baseline cpu | cpu @4 cores | ratio |
|---|---|---|---|---|---|---|
| `brief` | 150 ms | 210 ms | 1.40× | 1130 ms | 690 ms | **0.61×** |
| `trail --scope all` | 190 ms | 270 ms | 1.42× | 1510 ms | 930 ms | **0.62×** |
| `find --scope all` | 150 ms | 190 ms | 1.27× | 940 ms | 580 ms | **0.62×** |
| `trail` main scope | 100 ms | 130 ms | 1.30× | 640 ms | 380 ms | **0.59×** |

The two metrics disagree because they measure different things. `cpu_ms` is work summed over
cores: it barely moves with core count, and here it went *down* with fewer cores because the
sync and spin overhead of 12 workers disappears. `wall_ms` is latency, which scales with core
count, with whoever else is on the runner, and with a cold page cache loading a freshly built
binary. CI runs `macos-14` (8-core M1); the baseline belongs to an M3 Pro with 12 cores. A
shared runner cannot judge latency against a laptop's numbers, at any tolerance that still
detects real damage.

So the enforcement is split by metric, not by convenience:

| Metric | Locally | On CI | Why |
|---|---|---|---|
| `cpu_ms` | gated (×1.25) | **gated** (×2.0) | total work; insensitive to cores |
| `rss_mb` | gated (250 MB ceiling) | **gated** | absolute, machine-independent |
| `wall_ms` | **gated** (×1.25) | reported, not gated | belongs to the machine that captured it |

`--gate-wall` / `--no-gate-wall` (or `CXJ_PERF_GATE_WALL=1`) override it either way, and the
banner states which mode ran so a green run cannot be misread as a wall-time approval.

**The accepted cost:** a change that adds no work but serializes it — same `cpu_ms`, worse
latency — passes CI and only fails on a developer machine. That is the trade for a gate that
stays green when nothing is wrong; a wall gate on shared hardware fails often enough that people
start ignoring it, which is worse than a hole.

**The blindness fix:** the gate can write a markdown verdict (`--markdown <path>`) and the `perf`
job publishes it as a commit comment and in the step summary, with `if: always()`. A commit
comment is readable through the public API, so "the perf job is red" became an actionable
sentence without anyone granting repository access. The comment step is `continue-on-error`, so a
permissions problem cannot mask the gate result, and the recorded exit code is re-raised by a
final step so the job still fails exactly when the gate does.

### 3.4 Battery/thermals note (this is a laptop tool)
Because cores past ~4 add CPU without wall-time benefit, prefer to cap effective workers at the
**performance-core count** (detect via `hw.perflevel0.logicalcpu` on macOS) rather than
`GOMAXPROCS=ncpu`, to avoid scheduling onto efficiency cores that add heat for no latency. Make
this the default worker count, overridable by `--workers`/`GOMAXPROCS`.

### 3.5 P0+P1+P2+P5 results (measured after shipping, same harness)

`scripts/perf-bench.sh` median of 5, Apple M3 Pro, warm cache. `payment-api` (561 main / 849
total java) in report-only mode:

| Command | Before (full Open) | After (main scope) | Δ wall | Δ cpu | Δ RSS |
|---|---|---|---|---|---|
| `find` / `trail` / `callees` / `endpoints` / `keys` | 620 ms / 4.52 s / 93 MB | **170 ms / 1.00 s / 34 MB** | **3.6×** | **4.5×** | 2.7× |
| same with `--scope all` (old path, unchanged) | 620 ms / 4.52 s | 610 ms / 4.53 s | — | — | — |
| `brief` (BriefOnly, full scope by design) | 560 ms / 3.6 s | 560 ms / 3.68 s | — | — | — |
| process startup | ~0 ms | 0 ms | — | — | — |

Two things worth reading off this table. First, `--scope all` reproduces the old cost to within
noise, which proves scope is the *only* variable — the speedup isn't a measurement artifact.
Second, every navigational command now sits inside the §3.3 budget (navigational < 300 ms,
scoped < 200 ms, RSS < 200 MB), so the ceilings are live constraints rather than hopes.

The committed **fixture** baseline (the gated numbers) is 150-160 ms wall / ~1.2 s cpu for
main-scoped commands and 340 ms / ~3.1 s for `--scope all`, i.e. the fixture reproduces the
real repo's shape. In-process Go benchmarks confirm the scaling story from §3 held at the
fixture size — full Open 1.63 s @1 core, 0.49 s @4, 0.35 s @8 (**4.7× on 8 cores**, so the
~19 % serial fraction is the ceiling), and main-scope allocation drops 141 MB → 64 MB, which is
the garbage P5 attacks.

**P2 (sub-index selection).** Gated fixture numbers, before → after, median of 5:

| Fixture case | wall | cpu | RSS |
|---|---|---|---|
| `keys` | 150 → **70 ms** | 1190 → **320 ms** | 42 → **20 MB** |
| `endpoints` | 150 → **70 ms** | 1190 → **350 ms** | 40 → **20 MB** |
| `find` main scope | 160 → **110 ms** | 1210 → **650 ms** | 43 → 40 MB |
| `callees` main scope | 150 → **110 ms** | 1190 → **670 ms** | 41 → 40 MB |
| `trail` main scope | 160 → **130 ms** | 1200 → **920 ms** | 42 → 41 MB |
| `show` / `outline` (new cases) | — → **100 ms** | — → **640 ms** | — → 40/37 MB |
| `trail --scope all` | 340 → **270 ms** | 3130 → **2430 ms** | 84 → 76 MB |
| `brief` (unchanged by design) | 230 ms | 2010 → 1990 ms | 44 → 45 MB |

On `payment-api` (min of 6, alternating runs so thermals hit both binaries equally):

| Command | ref wall/cpu | P2 wall/cpu | cpu win |
|---|---|---|---|
| `keys` | 170 ms / 940 ms | 100 ms / **300 ms** | **3.1×** |
| `endpoints` | 160 ms / 929 ms | 100 ms / **310 ms** | **3.0×** |
| `beans` | 170 ms / 909 ms | 100 ms / **310 ms** | **2.9×** |
| `show` / `outline` / `callees` | 160 ms / 929 ms | 140 ms / **560 ms** | **1.7×** |
| `find` (scope all) | 590 ms / 4420 ms | 530 ms / **3420 ms** | 1.3× |
| `trail` | 170 ms / 940 ms | 160 ms / **770 ms** | 1.2× |
| `brief` | 520 ms / 3560 ms | 520 ms / 3590 ms | — |
| `status` | 580 ms / 4390 ms | **80 ms / 69 ms** | **63×** |

Read three things off these. `trail` only moves 1.2× because it genuinely needs the Java parse
*and* the spring parse; capability selection cannot help a command that reads everything, and the
remaining cost there is P3/P5's. Wall time now floors out at ~70-100 ms on the trimmed commands no
matter which caps are off, because that is the `filepath.WalkDir` cost, which no capability gates
— the last 70 ms of every command is P3. And `status` is the whole idea in one line: it declares
no capabilities at all, since its answer is the scan's counters, so it went from parsing 849 Java
files three times over to 69 ms of walking.

`brief` staying flat is the honest control. P2 gives it exactly the work it used to get, and it is
the case that caught the one regression this phase introduced (an unconditionally-built symbol
index, +6 % cpu and +39 MB RSS on the tool's most common first call), because in-process
`BenchmarkBrief` allocates the identical 1,361,787 objects before and after.

**P5 (line numbers read from the parser).** Gated fixture numbers, before → after, median of 5:

| Fixture case | wall | cpu |
|---|---|---|
| `brief` | 230 → **150 ms** | 1990 → **1130 ms** (1.76×) |
| `find --scope all` | 220 → **150 ms** | 1740 → **940 ms** (1.85×) |
| `find` main scope | 110 → **90 ms** | 650 → **410 ms** (1.59×) |
| `show` / `outline` | 100 → **80 ms** | 640 → **400 ms** (1.60×) |
| `callees` main scope | 110 → **80 ms** | 670 → **390 ms** (1.72×) |
| `trail` main scope | 130 → **100 ms** | 920 → **640 ms** (1.44×) |
| `trail --scope all` | 270 → **190 ms** | 2430 → **1510 ms** (1.61×) |
| `keys` / `endpoints` | 70 → **60 ms** | 320 → **270**, 350 → **290 ms** |

RSS did not move (39–45 MB): this phase removes scanning, not allocation, and the allocation
counts confirm it — `BenchmarkOpen` allocates 1,610,018 objects before and 1,610,021 after.

### 3.6 What the deterministic fixture cannot see (found in P5)

The same change measured **1.85× on the fixture and 3.9× on the real repository**, and the reason
is worth writing down because it applies to every future phase.

`lineAt` was **8.0 % of a cold Open on the fixture but 36.5 % on `payment-api`** — profiled the
same way, `unicode/utf8.DecodeRune` visible underneath it in both cases (21 % of total CPU on the
real repo, entirely inside `lineAt`). The fixture has the same *file count* as `payment-api`
(850 vs 849 java files, which is what it was sized to) but much smaller *files*, and a
from-byte-zero rescan is quadratic in file size. A fixture matched to a real repo by file count
therefore under-reports any superlinear cost by roughly the ratio of average file sizes.

Two consequences:

- **Fixture numbers are a floor, not an estimate, for algorithmic fixes.** The gated numbers above
  are the pessimistic ones; agents on real Spring codebases see more.
- **Report-only real-repo runs are not optional decoration.** Every P-phase number in this
  document is gated on the fixture, but the fixture alone would have rated P5 a 1.6× phase instead
  of a 2.7× one. Keep running both.

The clean number for the phase sidesteps the whole issue: at `GOMAXPROCS=1` over `payment-api`, a
full Open went 2921 ms → 1093 ms. One core means no parallelism, no thermal drift and no scheduler
in the measurement, so it is the amount of work removed and nothing else.

Real-repo CLI numbers (min of 6, alternating runs so thermals hit both binaries equally):

| Command | ref wall/cpu | P5 wall/cpu | cpu win |
|---|---|---|---|
| `find --scope all` | 520 ms / 3389 ms | 180 ms / **860 ms** | **3.9×** |
| `find` main scope | 520 ms / 3340 ms | 180 ms / **880 ms** | **3.8×** |
| `brief` | 520 ms / 3570 ms | 170 ms / **1020 ms** | **3.5×** |
| `trail --scope all` | 560 ms / 3889 ms | 210 ms / **1370 ms** | **2.8×** |
| `show` / `outline` / `callees` | 140 ms / 550 ms | 100 ms / **310–330 ms** | **1.7×** |
| `trail` | 150 ms / 740 ms | 120 ms / **500 ms** | 1.5× |
| `keys` / `endpoints` / `beans` / `status` | unchanged | unchanged | — |

`keys`, `endpoints`, `beans` and `status` are flat, and that is the control: P5 lives in the Java,
Spring and binding extractors, and P2 already proved those four commands do not run the Java
extractor. A change that makes everything faster is usually making the measurement worse.

Correctness: 28 invocations over `payment-api` (both scopes, text and JSON) byte-identical to the
pre-P5 binary, which for this phase specifically means **every line number in every answer
matched** — including `map`, which prints 1086 lines of `path:line:symbol`. One field did differ:
`status --json`'s `go_heap_alloc_bytes`, by 7 %. That field is noise, not a result — the *same*
binary varies 3.08–3.29 MB run to run — and the A/B harness that produced this line (an ad-hoc
script, not a repo artifact) should zero it out instead of reporting it as a near-miss. If that
harness is ever promoted into `scripts/`, that is the first thing to fix.

### 3.7 The other half of the bill: what one lookup costs (audit, before P4)

Track P has measured `Open` because statelessness makes it unavoidable — every command pays it.
That also means nobody had measured the *query* half, which is the half that decides whether P4
(batch mode) is worth building. `internal/engine/query_bench_test.go` does: one `Open` outside the
timed region (`CapsAll`, main scope), then individual lookups and a 20-lookup batch.

Per lookup, median of 10, Apple M3 Pro:

| Lookup | fixture — 8260 defs / 10360 edges | `payment-api` — 2593 defs / 5963 edges |
|---|---|---|
| `find` (member's simple name) | 9.1 ms · 7.0 MB · 155k allocs | **4.7 ms · 3.1 MB · 49k allocs** |
| `find` `Controller` | 10.4 ms · 8.3 MB · 222k | **5.2 ms · 3.6 MB · 74k** |
| `find` `Service` | 10.9 ms · 8.9 MB · 240k | **5.1 ms · 3.4 MB · 74k** |
| `trail` | 1.5 ms · 1.8 MB · 36k | 0.78 ms · 1.4 MB · 14k |
| `callers` | 0.58 ms · 410 kB · 8.5k | 0.29 ms · 255 kB · 2.6k |
| `callees` | 0.53 ms · 894 kB · 19k | 0.27 ms · 798 kB · 8.3k |
| `map` | 0.32 ms · 666 kB | 0.21 ms · 369 kB |
| resolve a name (`LookupAll`) | 0.13 ms · 117 kB · **14 allocs** | 0.043 ms · 7 kB · 10 |

**A `find` costs 4.7 ms and 3.6 MB to return 20 lines.** That is 3.3 % of a full Open in time and
5 % of its garbage, per query, and it allocates 74,000 objects to print twenty. Two structural
reasons, both read from the source and one visible in the profile:

- **There is no index.** `index.Build` concatenates facts into flat slices and keeps no maps, so
  every query is a full pass over every definition and edge in the workspace. `callers` is the
  worst shape: `References` scans defs for definition sites, defs again for overrides, edges for
  call sites, and `implementors()` walks the type hierarchy to make the override pass possible —
  four passes per lookup, deduplicated through a freshly allocated map.
- **The ranking ladder runs in full on every definition.** `matchDef` builds up to four candidate
  strings per def by concatenation (`Owner+"."+Name`), calls `strings.ToLower` roughly ten times
  — including `strings.ToLower(q)`, recomputed per def — and then up to four Levenshtein passes in
  `fuzzyQualified`. `strings.ToLower` is the top non-runtime cost in the profile (10 % of process
  CPU). This is what the 74k allocations are: candidates and lowercase copies for defs that
  matched nothing.

The batch number is the one that changes the plan. `Open` + 20 mixed lookups on `payment-api` is
**201 ms against 141 ms for the `Open` alone**, so the lookups add 43 % of an `Open`; on the
fixture it is 237 ms against 125 ms, i.e. +89 %. P4's exit criterion ("20 mixed lookups in one
batch ≈ one Open, ≤ 0.35 s main-scoped") is therefore **already reachable** — the batch is ~200 ms
in-process plus ~10 ms startup — but a third of its cost is now the queries, and four of its
twenty lookups are `find`, which is the expensive one.

The fixture inverts here, which is worth noting next to §3.6. Sized by *file* count, it carries
**3.2× more definitions than `payment-api`** (8260 vs 2593: 9.7 per file against 4.6), so every
per-def cost looks worse on the fixture. Query-side fixture numbers are pessimistic, where
`lineAt`-style Open-side numbers were optimistic. Same root cause, opposite sign: file count is
not a workload model.

Two smaller finds from the same read:

- `TextIndex.Search` lowercases **every indexed text line on every query** (`strings.ToLower`
  inside the loop over `t.lines`). Harmless at `payment-api`'s ~7k resource lines, and it grows
  with resource files rather than code.
- `LookupAll`, `map`, `callees` and `callers` are all sub-millisecond. The problem is specifically
  `find`, and specifically `matchDef`'s cost for the ~99 % of definitions that will not match.

**Recommended: P7 (query cost) before P4 (batch mode).** P4's premise is that a lookup is cheap
and an `Open` is not; that premise is currently only half true, and P4 built on the flat index
would ship a batch command whose first four lookups cost more than everything after them. P7 is
also the same proof shape as P2 — identical answers, less work — with the added constraint that
the A6/A7 tier ranking is user-visible, so `exact > prefix > fuzzy > substring/hump` must come out
byte-identical, not "similarly ranked".

Sketch of P7, none of it state, all of it built per `Open` and dropped at exit:

- Compare without allocating: the package already has `foldByte`, so fold-aware `HasPrefix` /
  `HasSuffix` / `Index` replace the ~ten `strings.ToLower` copies per def; hoist the folded query
  out of the loop entirely.
- Prune before ranking: skip `fuzzyQualified` when the length difference alone exceeds
  `maxFuzzyDistance(q)`; do not build the four candidates until a cheap test says the def is a
  contender.
- Give exact-name lookups a map (`name → defs`) built at `Open`; `LookupAll` is already the cheap
  case, which is the argument that a map is affordable.
- Bound the hit lists before sorting: `Search` collects every match in the workspace and sorts it,
  then truncates to 20.

**Exit:** `find` on `payment-api` under 1 ms with allocations under 5000, ranking byte-identical to
today across a fixed query battery (the tier table in `ir`, exercised at both scopes), and the
20-lookup batch under 160 ms against its 141 ms `Open`.

One instrument note: `b.ResetTimer()` **deletes user-reported metrics** — reporting corpus sizes
before it loses them silently, and the benchmark lines come out without their denominators and
without complaining. `queryBench.reportSizes` resets first, and says why.

### 3.7.1 P7 results — what shipped and what the exit criteria became

All four sketch items shipped except the `name → defs` map, which the measurement made
unnecessary: `LookupAll` was already the 40 µs / 10-alloc case, and it stays that way.

- **`internal/index/match.go`.** The per-query `matchPlan` (folded query, split segments, distance
  bound — computed once per `Search`, not per definition) and a byte-folding matcher for the ASCII
  case. `matchDefLegacy` is the previous implementation kept verbatim in production: it is what
  every non-ASCII definition still runs, because byte folding equals `strings.ToLower` only while
  both sides are ASCII (ToLower expands U+0130 to `i` plus a combining dot — an ASCII byte that
  was never in the input). Which definitions are ASCII is decided once at `Build` as a bit per
  definition. The four spellings of a definition are compared as un-joined halves, so no
  per-definition concatenation, splitting, or lowering survives on the hot path. For a one-word
  query the edit distance is computed against each *distinct* trailing segment once, not once per
  spelling, and `foldIndex` skips candidate starts with assembly `IndexByte`.
- **Text lines are lowered once at `Build`.** `TextIndex.Search` used to call `ToLower` on every
  line on every query; it now calls `strings.Contains` on the stored lowercase, which is exactly
  what the old expression computed — same bytes, same answer, non-ASCII included. Cost moved from
  per-query to a single pass per `Open` that barely registers in the gate.
- **Hit lists are bounded during the scan, not after it** (`topHits`): insertion at the first
  position strictly beaten, ties landing behind ties, which reproduces
  collect-everything + `sort.SliceStable` + truncate exactly.

**How "identical answers" is proven**, rather than asserted:

| Test | What it pins |
|---|---|
| `TestMatchDefMatchesLegacy` | differential fast-vs-legacy verdict per definition, ~630k comparisons in CI (golden + synthetic fixture), ~1.06M with `CXJ_JAVA_DIR` pointing at a real repository |
| `TestSearchPinnedRanking` | absolute expected hits/tiers/scores written into the test as literals, generated once from the pre-P7 matcher and hand-checked row by row — the guard against both matchers drifting together |
| `TestSearchCappedMatchesFullSort` | the bounded top-k against a full collect-sort-truncate reference, limits 1/3/20, tie order included |
| `TestNonASCIIDefinitionsTakeTheLegacyPath` | the ASCII bit flags match ground truth, and non-ASCII corpora answer exactly what the legacy matcher answers |
| `TestFoldIndexEquivalence` | 4,000 seeded pseudo-random strings against the `ToLower`+`Index` definition — it caught a real window bug in the first `IndexByte`-skipping version |

**Exit criteria, measured after (was → now, `payment-api`, 2,593 defs):**

| Metric | Before | After | Criterion |
|---|---|---|---|
| `find` wall | 4.7–5.2 ms | **0.52–0.56 ms** | < 1 ms ✅ |
| `find` allocations | 49k–74k | **13–21** | < 5,000 ✅ |
| `find` bytes | 3.1–3.6 MB | 2.7–19 kB | — |
| 20-lookup batch | 201 ms | **159.0 ms** | < 160 ms ✅ (Open alone: 141 ms) |

Fixture (8,260 defs): 9.1–10.9 ms → 1.2–1.6 ms per `find`, 13–15 allocs; batch 237 → 166 ms.
Correctness: 31 CLI invocations over `payment-api` (both scopes, text and JSON, `find`/`trail`/
`show`/`map`/`callers`/`callees`/`references` plus two non-ASCII queries) byte-identical to the
`4e45a80` binary. The perf gate stayed green **without** re-capturing the baseline — end-to-end CLI
numbers are Open-dominated, so a query that got 9× cheaper moves `find`'s cpu_ms by 10 ms. That gap
between the in-process 9× and the gated 2 % is the point of P4: the batch pays the query cost N
times, and now it can afford to.

`trail` (0.75 ms / 14k allocs) and `callees` (8.3k allocs) keep their old costs — untouched here,
and the next obvious tenants of the same lease.

---

## 4. Track A — navigation accuracy

### 4.1 Measured findings (honest restatement)
Earlier I called `trail` output "inverted." It is not. Measured on `payment-api` with the
shipped binary *(this table is the pre-Track-A baseline; §4.4 carries the after column)*:

| Command | Measured | Correct | Cause | Verdict |
|---|---|---|---|---|
| `callers …PaymentController.pay` | 106 | ~1 | call-sites matched by simple name `pay` repo-wide | bug |
| `callees …PayHandler.handle` | 292 / 23 files | 1 file body | `From` matched by simple name `handle` repo-wide | bug |
| `trail …PaymentController.pay` (http) | 5 routes, none on it | 1 | `Contains(sym,"pay")` matches package `paymentapi` | bug |
| `find createPayment` | only test helpers | (term absent in main) | term genuinely not in `src/main` | **artifact, not a bug** |
| `find Controller` (also `Handler`, `Repository`) | **0** | every `*Controller` class | Java is never substring-searched (A-RC7) | bug |
| `find Handler` | 31, of which **29 are private test *fields* named `handler`**, **0 are handler classes** | the handler classes | any definition's simple name scores equal regardless of kind (A-RC8) | bug |
| `callers …PaymentController.pay` with `--scope all` | 106 | ~1 | same A-RC1 collisions, most of them in test files | bug, masked by P1 |

The `find Controller` row is the one that bites agents hardest: the natural first query in an
unfamiliar service is a suffix of a type name (`find Controller`, `find Handler`), and it
returns *nothing at all* — not a bad ranking, an empty result. Empty reads as "this
concept does not exist here", which is worse than a wrong answer. Its neighbour `find Handler`
is worse in a different way: 31 confident results, all of them private test fields.

**Caveat on the P1 work: it masked an accuracy bug without fixing it.** `callers
…PaymentController.pay` measured 106 before P1 and **7** after, because the default main scope
drops test files and ~99 of those collisions lived in tests. The A-RC1 root cause is fully
intact — `--scope all` still returns 106. Perf scoping made the *evidence* of an accuracy bug
disappear, which is the opposite of what a faster tool should do. So A1/A2 must be verified at
`--scope all`, not only at the new faster default, or the fix can pass while remaining broken.
A1, A2, A6 and A7 were all checked that way; `callers` at 106 is the row still open, and it is
open because it needs A3 rather than a stricter string.

### 4.2 Root causes (code-anchored)
- **A-RC1 simple-name edge matching** — `index.References` via `callMatches` and
  `index.Callees` via `fromMatches` (internal/index/symbol.go) match on the last dotted
  segment. Common names (`pay`, `handle`) collide repo-wide.
- **A-RC2 substring fallbacks** — `trail.symbolMatch` `Contains`; `spring.Context`
  `Contains(e.Symbol,…)`; `index.TestTypes` `Contains(d.Qualified,…)`; `spring.InjectionsFor`
  bare `HasPrefix`. `pay` is inside `paymentapi`, so they match almost everything.
- **A-RC3 silent ambiguity** — `index.Lookup` returns `named[0]`; `resolveSymbolName` takes the
  first hit. A bare `pay`/`handle` answers about the wrong symbol.
- **A-RC4 no receiver typing** — `java.extractInvocation` stores `To = recv + "." + name` with
  `recv` a raw variable *name*, never a type. So `payService.pay` can't be told apart from any
  other `.pay`. This is why `callers <method>` is approximate.
- **A-RC5 no source tier in `find`** — `engine.mergeLexical` has no main-vs-test ordering.
- **A-RC6 no coverage** — only `brief` has tests; nothing guards `trail/ref/callers/callees`.
- **A-RC7 no substring search over Java at all** — two gaps compose into an empty result:
  `index.BuildText` indexes **non-Java files only** (internal/index/text.go:23), so the text
  side of `find` never sees Java source; and `index.matchDef` (internal/index/symbol.go:371)
  offers only exact, `.name` suffix, prefix and bounded edit distance — a suffix of a *simple*
  name (`Controller` in `PaymentController`) is none of those, and the fuzzy distance (7) blows
  the budget (1). So `find Controller` cannot match anything.
- **A-RC8 no kind tier in `find`** — `matchDef` scores the simple name of *any* definition
  (type, method, field) with equal weight, so for the query `Handler` the ~29 private fields
  named `handler` in test classes score MatchExact (rank 0) and **every handler class in the
  repo is outranked**. Distinct from A-RC5: that one is main-vs-test ordering, this is
  type-vs-member ordering. Fix is a tier in `matchDef`/`mergeLexical` (types before members for a
  capitalized query), not more fuzzy matching.
- **A-RC9 `trail` over- and under-reports at once** — the http section on `…PaymentController.pay`
  shows 5 routes owned by `BackofficePaymentController`/`CallbackController` (matched via
  `Contains(endpointSymbol, "pay")`, and `paymentapi` contains `pay`) while **omitting the route
  the method actually has**. The substring match makes every endpoint in the repo a candidate, so
  the `trailCap` truncation then discards the correct one. Both halves come from one line, and
  the cap turns a wrong answer into a confidently wrong one.
- **A-RC10 two private copies of the match-tier table** (found while landing A6) —
   `index.matchRank` ranked symbol hits and `engine.rankMatch` re-ranked the merged `find`
   output, and the two agreed only by habit. Adding `substring` to one side put every new-tier
   hit at the default rank on the CLI, so a class named `PayHandler` sorted *below a line of
   `demo.html`* on the real repo. Any ranking the user can see must live in `ir` once
   (`RankMatch`/`LexicalLess`/`WantsTypeName`); a per-package copy is a latent bug that only
   surfaces when someone extends the other one. Same shape, same discovery, found again in P5:
   three private `lineAt` copies across the extractors, disagreeing with each other, one of them a
   36 %-of-Open performance bug. `internal/tshelp` is now the single home for that helper too.

### 4.3 Phases (biggest, cheapest win first; red tests before behavior change)

Status: **A0 ✅ `b145fad` · A1 ✅ `d552511` · A2 ✅ `a897b3b` · A6+A7 ✅ `480505a`**
· A4, A3, A5 open. A5 is half-shipped: `RefReply.Ambiguous` exists and prints in both text and
compact JSON (A2); the `(+K more)` cap marker and certainty-in-verbose are still open.

- **A0 — Fixtures + goldens (prerequisite) ✅.** Inline collision-shaped Java fixtures fed
  through `java.Extractor.Extract(path, src)` (already takes bytes → no repo-wide indexing
  needed), in `internal/index/golden_test.go` and `internal/engine/trail_golden_test.go`.
  Encode: `handle` on three owners; `pay` both as a method and as a substring of the package
  `paymentapi`; two types where one name prefixes the other; a field named `handler` beside
  `*Handler` types; a per-type test class plus a `PayoutTest` prefix collision.
  **Deviated from the plan on purpose:** goldens landed *green* (they lock behavior that already
  holds, which is the regression cover later phases need); each later phase lands its own
  negative test red-then-green in the same commit. A0 also pins that `Edge.From` is the
  fully-qualified enclosing method — A2’s callee scoping is meaningless if that changes.
- **A1 — Drop substring fallbacks (A-RC2) ✅.** Removed the `Contains`/bare-`HasPrefix`
  lines in `trail.symbolMatch`, `index.TestTypes`, `spring.Context`, `spring.InjectionsFor`;
  kept exact plus dot-boundary prefix/suffix. `trail …PaymentController.pay` went from five
  wrong routes and a missing own route to exactly `POST /v1/payments`; `trail
  <PaymentController>` went from five foreign routes to its own five. Two over-corrections
  surfaced on the real repo rather than in unit tests: an affix-only test-name rule dropped
  `PaymentControllerValidationTest` (reporting a tested class as untested), and a bare prefix
  still let `Pay` match `PayoutTest`. Both fixed and pinned as test cases.
- **A2 — Owner-scope callees + surface ambiguity (A-RC1 callee side, A-RC3) ✅.**
  `index.LookupAll`, `engine.resolveOwners` (resolve to a qualified owner *before* asking the
  index — that is what keeps a precise query precise, since the matchers only fall back to
  simple names for genuinely unqualified input), strict `fromMatches`, and `RefReply.Ambiguous`
  listing the **full** candidate set including the symbol answered about.
  `callees …PayHandler.handle`: 292 results across 20 files → 31 results in 1 file. `handle`
  really is owned by 20 methods on `payment-api`, and the flag says so (names capped at 5, the
  count always true). `callers` untouched by design.
- **A4 — `find` source tier (A-RC5) ✅.** One more key in `ir.LexicalLess`: main before test,
  above match-kind, stable otherwise — framed as ranking, not correctness. It sits *below*
  A7's wantsType penalty on purpose (a wrong-kind answer is worse than a right-kind answer
  from the wrong tree) and *above* match rank, which is what "main before test, above
  match-kind" means when it bites: on `payment-api`, `find Controller` now leads with
  `MerchantPaymentController` (main, score 0.244) over `PaymentControllerValidationTest`
  (test, 0.256) — lower score, higher rank. `find pay` moves all eight main hits above all
  four test hits. The one honest cost lives at the tail of deep pages: at
  `--scope all --limit 50`, two `demo.html` text rows (main, MatchText) rise above the last
  two test-class rows, so the classes row reads 48/50 — the source tier is one key, not a
  symbol-over-text rule, and page 1 is untouched (still 5/5 classes).
  `ir.IsTestPath` becomes the single law of the judgment: `engine.isTestPath` and
  `scan.isTestPath` both delegate to it (the two spellings had quietly agreed only by luck).
  This is the battery's first **intended** divergence: 3/31 `find` rows reorder, all pure
  main-first moves, everything else byte-identical.
- **A3 — Receiver typing (A-RC4, A-RC1 caller side; largest, land last).** Record declared
  types for fields/params (+ simple locals) in extraction; build a `varName→Type` scope map;
  when a call receiver is a known name, set edge `To` to the receiver *type* (still
  `candidate`). Then `callers C.pay` matches typed edges resolving to `C`/subtypes, falling back
  to name candidates (labeled `candidate`) only when the type is unknown. **Exit:**
  `callers …PaymentController.pay` drops 106 → real caller set.
- **A5 — Honesty in presentation (A-RC3 visibility) ✅ (shipped in halves).** The ambiguity
  half shipped with A2 (`ambiguous N: qualify the query (…)` in text, `ambiguous_total` in
  JSON); the certainty half ships here in `trail --verbose`, which now appends each hit's
  certainty the way `references --verbose` has. The third — the trail-cap marker — is the
  one the audit found missing: `trailCap` truncated silently, so five callers and 106
  callers printed identically. Every `trail` section now counts what it saw before capping:
  `TrailReply.Totals` mirrors the sections in JSON, text renders `callers … (+101 more)`
  (the label rides because sections share one stream), and `ask`'s digest carries the same
  marker. On `payment-api`: `trail …PaymentController.pay` → `(+2 more)`, at `--scope all`
  → `(+101 more)` — the 106-caller window admits being a window; A3 still owns shrinking it.
- **A6 — Substring/camel-hump search over Java (A-RC7) ✅.** `MatchSubstring` tier in
  `matchDef`, ranked below exact/prefix/fuzzy so it can only fill gaps: case-insensitive
  substring on the *simple* name — never the qualified name, which is A-RC2’s lesson applied
  to a brand-new tier — plus camel-hump at one letter per hump. The example in the original
  draft, `PHCtrl` → `PaymentHandlerController`, is **not implementable**: under the usual
  hump-prefix rule, “ctrl” is not a prefix of “controller” either. `PPC` →
  `prePaymentComplete` is what the shipped rule gives. Plain substrings need 3 letters (two
  letters mid-word match a slice of any repo); hump queries do not. Java source was *not* added
  to the text index, so the §3 budget holds. `find Controller`: 0 → the controller set,
  main-first.
- **A7 — Kind tier in `find` (A-RC8) ✅, and it needed a second rule.** Types-before-members
  *within* a tier cannot fix `find Handler`: the private field `handler` claims `MatchExact` for
  that query by case-insensitive equality while no class is named exactly `Handler`, so it sits
  in a strictly better tier no within-tier preference can reach. The shipped rule also sinks
  members whose own name starts lowercase when the query is capitalized and unqualified
  (`ir.WantsTypeName`). Still a sink, not a filter: `find handler` leads with the field.
  `find Handler --scope all --limit 50`: 0 classes → **50/50 classes**, no test fields.

### 4.4 Benchmark table (Track A exit criteria, from `payment-api`)
| Metric | Before | Target | As shipped |
|---|---|---|---|
| `callers …PaymentController.pay` | 106 | single digit | 106 — needs A3 (untouched by A1/A2 on purpose) |
| `callees …PayHandler.handle` | 292 | ≈ method-body calls | **31** ✅ |
| distinct files in that callees | 23 | 1 | **1** ✅ |
| `trail …PaymentController.pay` http rows | 5 (all wrong) | 1 | **1, its own** ✅ |
| `find Controller` result count | 0 | main controller set, main-first | **5 controllers, all main** ✅ (A4 pushed the validation test off the page) |
| `find Handler` classes on first page | 0 | most of the first page | **5/5**, and 48/50 at `--scope all --limit 50` ✅ (A4: two main-tree `demo.html` text rows now rank above the last two test classes — see A4) |
| `callers …PaymentController.pay` at `--scope all` | 106 | single digit | 106 — A3 |

Every row is checked at `--scope all` as well as at the main-scope default: a narrowed default
can make an unfixed matcher look fixed, and `callers` is the honest counterexample still open.

---

## 5. Track B — the Context7 features (stateless)

Both read the user's build files and their own local artifact cache, parse on demand, exit.
Neither stores anything. Neither uses the network. New subcommands live in `cmd/cxj/main.go`
(dispatch + `usage`), delegate to new `internal/engine` files (reuse `outlineFacts`,
`ir` formatting, `Search`), and must **not** route through `engine.Open`/`scan.Scan` (those are
root-bounded and skip binaries/JARs by design — correct, since JARs must not enter the symbol
graph). Dependency files outside the repo are opened directly.

### 5.1 Feature 1 — `cxj deps` : dependency & version ground truth
"Which version am I actually on, and where did it come from?"

Sources (local, on demand): build files under `--root` — `pom.xml` (`<parent>`,
`<dependencyManagement>`, `<properties>`, BOM `import`), `build.gradle(.kts)`
(`platform(...)`, `implementation "g:a:v"`), `gradle/libs.versions.toml`. Cross-check presence
against the user's local cache (`~/.m2/repository`, `~/.gradle/caches/modules-2`) via
directory listing only, path overridable by `--m2`/`--gradle`/`--cache` (also for tests).

Reuse & extend `internal/engine/versions.go` (already regex-parses pom/gradle and picks the
shallowest module); it becomes the parser seed for a `deps.go`.

Honest tiers, mirroring the existing certainty ladder:
- `declared` — direct in the project build file (with `file:line`).
- `managed` — version set by a BOM/parent/catalog, not directly declared.
- `cached` — a jar exists locally (evidence of resolution); multiple versions → `ambiguous`.
- `unknown` — cannot be determined without running the build. cxj does **not** reimplement
  Maven/Gradle resolution; it says so rather than guessing.

Surface:
- `cxj deps` — compact table `group:artifact  version  tier  file:line`, capped, headline
  frameworks (boot, core) first.
- `cxj deps <substring|g:a>` — filter; `--tier managed` etc.; `--json`.
- Lockfile-aware: if a resolved lockfile exists (Gradle `dependencies.lock`,
  `gradle/dependency-versions.toml`, npm-style) read versions straight from it and mark
  `cached/resolved`.

**Exit + tests:** fixtures for (a) pom with `<parent>` BOM + `<properties>` substitution,
(b) gradle with a version catalog, (c) a fake cache dir under `testdata`; assert property
substitution, `managed` vs `declared`, `cached` cross-check, and `unknown` when unresolvable.
Offline. `--root` and cache overrides honored.
**Why it wins:** Context7's headline is "version-specific," but you *tell it* the version and
trust their crawl; cxj derives it from your build + cache, reproducibly, offline — and `api`
consumes it.
**No-state:** reads build files + a user-owned cache read-only; writes nothing.

#### 5.1.1 B1 results — what shipped

All of it, with the tier ladder widened to five rungs once the lockfile earned its own:
`declared / resolved / managed / cached / unknown`. `deps` opens **no engine snapshot** — a repo's
dozen build files do not justify an 850-file scan — and declares `"deps": 0` in `commandCaps`
as the stated intent §7 asked for. Everything lives in `internal/engine/deps.go` (~700 lines),
`internal/ir/deps.go`, one printer method, and `runDeps`.

Real-repo work on `payment-api` changed the design twice:

- Gradle's `$var` versions are resolved from `gradle.properties` **and from `name = "value"`
  lines in the same build file** — the `springCloudVersion = "2025.1.2"` spelling is how BOMs
  are actually written. An unresolvable placeholder stays `unknown` showing the raw expression:
  a row that shows `$fooVersion` is diagnosable, a blank one is not.
- A cache bump replaces the now-false "no version found" note instead of stacking next to it,
  and dynamic lockfile pins (`5.+`) are reported as unknown-with-a-pin-note, never as versions.

Lockfiles answer transitives only under a filter (`deps jackson` reaches `dependencies.lock`
for what no build file declares); unfiltered they would bury the rows the user maintains.
Cache evidence is directory listing plus one look for a `.jar`, never a write.

Measured on `payment-api` (real `~/.m2` and Gradle caches): full answer in **12 ms wall, ~0 ms
cpu**. `TestDeps*` covers the plan's exit list — property substitution, declared vs managed,
cached cross-check both layouts, ambiguous → refuses to pick, unresolved → unknown, lockfile
outranks the build file while citing it — and the phase is **purely additive**: the 31-command
identity battery against `4e45a80` stayed byte-identical and the perf gate stayed green with
the baseline untouched.

Not built (documented gaps, honest tier carries them): remote BOM contents (needs network),
Gradle settings-level catalog declarations, npm lockfiles.

### 5.2 Feature 2 — `cxj api DEP [Type|member]` : your-version API + your usage
"What's the real API of this dependency **for my version**, and how does my repo already use
it?" — the Context7-killer, from disk.

Mechanism (no new parser, no state):
1. Resolve `DEP` → coordinates + version via `deps` (§5.1). Ambiguous version → list, don't guess.
2. Tier 1 (rich): open the resolved **`-sources.jar`** with stdlib `archive/zip`; select the
   `.java` entry(ies) for `Type`; run the **existing** `java.Extractor.Extract(path, src)` →
   `ir.FileFacts`; render public types + method signatures (+ first doc-comment line) via the
   **existing** compact outline path. Filtered to `member` if given; capped.
3. Tier 2 (always-on, minimal): a small **class-file reader** — parse the binary `.class`
   constant pool + `methods`/`fields` tables + access flags to list public members. Bounded,
   stdlib, no bytecode execution, no deps. Works when sources aren't cached. Signatures have
   types (names may be absent). This makes `api` reliable, so it is the MVP core.
4. Tier 3 (workspace): if `Type` is used in the repo, append a few **real call sites from your
   code** via `References`/`Trail` ("you already call `ObjectMapper.readValue` at file:line").
   Pure workspace, zero state — matches house style, cuts hallucination.
5. Graceful degradation: no sources jar → class-file tier; no jar at all → print resolved
   coordinates + one line "enable sources (`mvn dependency:sources` / Gradle) then re-run" and
   still show workspace usage. Type not found → nearest source-file names via `Search`.

Surface: `cxj api spring-web` (headline types), `cxj api spring-web RestClient` (its public
methods), `cxj api jackson-databind ObjectMapper.readValue` (matching overloads), `--json`,
`--from-repo-only`, `--sources-only`.

**Exit + tests:** a test "cache" built at test time with `archive/zip` from inline `.java`
(a sources jar) and a hand-authored `.class` fixture; assert `api` returns the public
signatures for a known type at the pinned version, the class-file fallback path, and the
workspace-usage augmentation. No network. Reuses `deps` resolution.
**Why it wins:** Context7 Search returns curated *doc snippets* for a *indexed* version; cxj
returns the *public surface of the exact artifact this build resolved* (jar/`-sources.jar`),
plus *your* call sites — offline, deterministic, no rate limit, source never leaves the
machine. Context7's `codeTokens` budgeting is a good idea; cxj matches it by capping signatures
and supporting `--compact` without any hosted round-trip.
**No-state:** opens a zip/`class`, parses, formats, exits; writes nothing.

#### 5.2.1 B2 results — what shipped

All five mechanism steps, in `internal/engine/api.go` + `classfile.go` + `internal/ir/api.go` +
one printer method + `runApi`. `api` resolves through `Deps` unchanged — coordinates arrive with
their tier and build-file provenance, and the two honest failure modes keep their shape: several
coordinates match → list them, one artifact at two cached versions → list them; the surface read
never picks a winner. The usage tier still answers when the version does not resolve
(`api jackson-databind ObjectMapper` on `payment-api` says "no version established (ambiguous:
2.15.2, 2.21.4)" *and* shows the workspace's `ObjectMapper` sites — half an honest answer beats a
confident wrong one).

The class reader (§5.2 tier 2, the MVP core) is a stdlib constant-pool + member-table parse with
every table count bounded against the bytes that remain: corrupt input is an error string, never
a panic or a partial member list. Its fixture is hand-authored `.class` bytes, not compiler
output — the reader is tested against the spec. The sources tier runs the entries through the
same `java.Extractor` as workspace code and reads each item's doc-comment first line off the
source; both tiers render signatures with types and without parameter names, because descriptors
never carried names and sharing one honest shape beats a per-tier dialect.

`api` is two halves and the cost model says so: the artifact half opens no snapshot (deps
resolution + bounded zip reads), the usage half is exactly a references query —
`"api": CapsSymbols` is the ceiling, and `--sources-only` skips the Open entirely. The flags read
as the plan named them: `--sources-only` = jar question only, `--from-repo-only` = workspace
question only; together they are a usage error because they cancel each other.

Real usage on `payment-api` (its cache had been IDE-seeded, so the richest tier was live):

    $ cxj api commons-lang3 StringUtils.isBlank --scope all   # 228 ms wall
    org.apache.commons:commons-lang3 3.18.0  declared  build.gradle:112
    public surface from sources jar: commons-lang3-3.18.0-sources.jar
      class org.apache.commons.lang3.StringUtils
        isBlank(CharSequence)  # Tests if a CharSequence is empty …
    your call sites (6)
      src/main/java/…/Payment.java:595  use StringUtils.isBlank  [candidate]

That row — the signature of the version this build compiles against, its own javadoc, and the
six places this repo calls it, offline — is the Context7 comparison in one screen.

The engine tests build the plan's exit fixture at test time: a sources jar zipped from inline
`.java` and a class jar carrying the hand-authored `.class` bytes, laid out in both cache shapes;
they assert the plan's list — sources-tier signatures + docs + private members excluded, class
tier from the gradle `files-2.1` layout, member filtering, headline mode that refuses to parse
hundreds of entries, ambiguous lists, no-jar guidance, and the usage tier. Purely additive again:
31/31 identity, gate green, baseline untouched.

One CLI bug the smoke caught: `--sources-only` had to be added to `SplitArgs`' boolean-flag list
— unlisted flags are assumed to take a value, so the flag ate its own `--scope`. The house rule
this implies (new booleans go in two places) is now enforced by the flag list's comment-free
simplicity rather than by luck.

Not built (documented gaps): parameter names for the class tier (optional attributes; the plan
accepted their absence), inner-class *members* under a top-level query beyond the matched
entries' own files, jar signatures/module descriptors, and transitive-only deps (no build-file
declaration means no coordinate — the same honesty boundary as `deps`).

---

## 6. Success metrics (competitive, measurable)
- **Cold-path latency (primary):** at `GOMAXPROCS=8` on the pinned repo, meet the §3.3 budget
  (`find`/`trail` p95 < 300 ms; scoped < 200 ms; 20-lookup batch < 400 ms). Checked in CI.
- **CPU efficiency:** full main-scoped Open cpu < 1.5 s and dropping vs the 4.4s baseline; P5
  cuts ≥ 25% single-core CPU.
- **Agent-turn total:** ~30 lookups end-to-end well under 1 s (batched), vs the current
  ~30 × 0.63 s ≈ 19 s.
- Time-to-first-fact for a dependency-API question, offline: cxj (local read) vs a Context7
  round-trip (network + service latency). Target: cxj < 250 ms warm, no network.
- Token cost per API answer: cxj compact signatures vs Context7 Search's returned snippets
  (they report `codeTokens`); target ≥ 5× smaller *and* version-exact to the compiled artifact.
- Version-accuracy: % of `deps` rows matching `mvn dependency:list`/Gradle `dependencies` on
  `payment-api`. Target ≥ 95% for direct deps.
- Offline coverage: % of direct deps with a class or sources jar present in the local cache.
- Hallucination proxy (Track A + B): N sampled API/graph facts, correctness vs the actual
  jar/graph.
- Track A regression table (§4.4) stays green in CI.

## 7. Rollout, effort, non-goals
**Status (2026-10-28): P0, P1, P2, P4, P5, P7 shipped and gated; Track A's cheap half (A0, A1, A2,
A4, A5, A6, A7) and Track B's `deps`/`api` pair (§5.1.1, §5.2.1) are shipped.** Everything below is
the remaining queue. A4's main-first `find` reorder and A5's trail-cap markers were the battery's
first intended divergences; the identity baseline moves forward with each such phase, so from here
on any diff is a bug again. A6 moved
forward because it is the
cheapest user-visible accuracy win and touches none of the graph code — it paid off, and A7 came
with it. P5 jumped the queue ahead of P3 because it was the only remaining phase whose fix was a
deletion rather than a redesign, and it turned out to be the largest single-phase win so far (§3.6).
P7 then jumped ahead of B1 for the reason §3.7 measured: it made a `find` 9× cheaper (§3.7.1) and
is the prerequisite that lets P4's batch mode actually afford twenty lookups.

Suggested PR order: `P0 ✅ → P1 ✅ → A0 ✅ → A1 ✅ → A2 ✅ → A6/A7 ✅ → P2 ✅ → P5 ✅ →
P7 ✅ → B1(deps) ✅ → P4(batch mode) ✅ → B2(api) ✅ → A4 ✅ → A5 ✅ → P3 → A3 → docs/bench`.
(A0/A1/A2/A6 make `find`/`trail` trustworthy before `deps`/`api` build on top; B tests reuse
A0's fixture harness and P0's `benchfixture`; `api`/`deps` land on the fast, scope-aware Open
path from P1/P2 so they start already-fast — P2 extends that to *which* indexes they pay for, so
`deps` should declare its own row in `commandCaps` rather than take `CapsAll`.)
Sizing: P0 S ✅ · P1 S ✅ · A0 S ✅ · A1 S ✅ ·
A2 S–M ✅ (came in S) · A6 S ✅ · A7 S ✅ (landed with A6, but needed a second rule) ·
P2 M ✅ (the code was S; the 44-case identity proof and the drift guard were the work) ·
P5 S ✅ (the swap was S; the 641k-position parity test was the work) ·
B1 M · P4 M · B2 M · A4 S · A5 S (half shipped via A2) · P3 M · A3 M · docs S.

**One thing P5 changes about the queue.** The fixture matched `payment-api` by file count and still
under-reported a superlinear cost by 4.5× (§3.6), so any remaining per-file or per-query loop that
is not linear is currently invisible in the gated numbers. Before P3, audit for the same shape:
`lineAt`-style rescans, and linear scans over the whole bean/injection/symbol tables per query
(`spring.Index.Beans`, `resolveOwners`, `mergeLexical`'s stable sort). A `--scope all` query on a
large repository is the corpus that would catch these, which argues for one more report-only bench
case at real file sizes rather than a bigger fixture.

**Audit done (§3.7), and it reorders the queue.** `internal/engine/query_bench_test.go` measures
lookups with `Open` outside the timed region. The scan-shaped costs are all on the query side:
`index.Build` keeps no maps, so every lookup is a full pass over defs and edges, and `matchDef`
runs its whole ladder — ~10 `strings.ToLower` calls and up to four Levenshtein passes — against
every definition. A `find` costs **4.7 ms and 3.6 MB to print 20 lines**; a 20-lookup batch costs
201 ms against a 141 ms `Open`. That is why **P7 (query cost) now precedes P4 (batch mode)**: P4's
premise is that a lookup is cheap and an `Open` is not, and only half of that is true today.

Cross-cutting: shared compact formatting + certainty tiers in `internal/ir`; new usage strings;
`SKILL.md`/README examples for `deps`/`api`, the `--scope` flag, and "pass qualified symbols to
callers/callees"; CI runs `go test ./...` **and the `perf` gate job** (`.github/workflows/build.yml`,
gate policy in §3.3.1);
`scripts/perf-bench.sh` (hard gate) is in place — still to add: `scripts/accuracy-benchmark.sh`
+ `scripts/context7-bench.sh`.

Explicit non-goals (they would break the no-state law or the architecture):
- **A persisted/cached index or on-disk warm cache as the way to get fast** — speed comes from
  less work + parallelism + batch-in-one-process, never from state cxj would carry across runs.
- Crawling docs or hosting any corpus; running an MCP server or daemon.
- A persistent/embedded index or vector store; embeddings on the hot path.
- Network by default (reading the local cache is not the network).
- Full Maven/Gradle transitive resolution or Spring-runtime bean/proxy graphs.
- Overload-accurate dispatch; `api` signatures are best-effort public surface.
