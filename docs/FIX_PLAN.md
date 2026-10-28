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
  deferred.** Ruled in-law by the maintainer on 2026-10-28: one process that opens once, answers
  many stdin queries, persists nothing, exits. Not built yet (out of the P0+P1 scope).
  `cxj query -` /
  `--batch` / `--json-batch`: read many operations over stdin, `Open` **once**, answer all, exit.
  This is the agent-facing win: dozens of lookups for the price of one Open, with zero persisted
  state. **Exit:** 20 mixed lookups in one batch ≈ one Open (≤ 0.35s main-scoped), not 20×0.15s.
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
- **A4 — `find` source tier (A-RC5).** `mergeLexical` orders `main` before `test` (reuse
  `engine.isTestPath`), above match-kind, stable otherwise. Framed as ranking, not correctness.
  After A7 the shared comparator owns this ordering, so A4 is one more key in `ir.LexicalLess`
  rather than a new sort site.
- **A3 — Receiver typing (A-RC4, A-RC1 caller side; largest, land last).** Record declared
  types for fields/params (+ simple locals) in extraction; build a `varName→Type` scope map;
  when a call receiver is a known name, set edge `To` to the receiver *type* (still
  `candidate`). Then `callers C.pay` matches typed edges resolving to `C`/subtypes, falling back
  to name candidates (labeled `candidate`) only when the type is unknown. **Exit:**
  `callers …PaymentController.pay` drops 106 → real caller set.
- **A5 — Honesty in presentation (A-RC3 visibility).** Print `resolved <t> (ambiguous: N)`;
  emit `… (+K more)` when a `trailCap` section is cut; show certainty in `--verbose`.
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
| `find Controller` result count | 0 | main controller set, main-first | **5 controllers, main-first** ✅ |
| `find Handler` classes on first page | 0 | most of the first page | **5/5**, and 50/50 at `--scope all --limit 50` ✅ |
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
**Status (2026-10-28): P0, P1, P2, P5 shipped and gated; Track A's cheap half (A0, A1, A2, A6, A7)
is shipped.** Everything below is the remaining queue. A6 moved forward because it is the cheapest
user-visible accuracy win and touches none of the graph code — it paid off, and A7 came with it.
P5 jumped the queue ahead of P3 because it was the only remaining phase whose fix was a deletion
rather than a redesign, and it turned out to be the largest single-phase win so far (§3.6).

Suggested PR order: `P0 ✅ → P1 ✅ → A0 ✅ → A1 ✅ → A2 ✅ → A6/A7 ✅ → P2 ✅ → P5 ✅ → B1(deps) →
P4 → B2(api) → A4 → A5 → P3 → A3 → docs/bench`.
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

Cross-cutting: shared compact formatting + certainty tiers in `internal/ir`; new usage strings;
`SKILL.md`/README examples for `deps`/`api`, the `--scope` flag, and "pass qualified symbols to
callers/callees"; CI runs `go test ./...` **and the `perf` gate job** (`.github/workflows/build.yml`);
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
