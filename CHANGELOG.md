# Changelog

All notable changes to `cxj`. The format is loosely based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this project
adheres to semantic versioning, and every entry is backed by a commit you
can read.

## [0.3.2] - 2026-10-29

### Added
- Release assets for all four supported platforms on every tag
  (`cxj-macos-arm64`, `cxj-macos-x86_64`, `cxj-linux-x86_64`,
  `cxj-linux-arm64` .zip), matching the asset names `cxj update` already
  resolves. Each asset is smoke-tested on its own runner before upload.
- Homebrew tap (`brew install umuterturk/tap/cxj`) and npm wrapper
  (`npx @umutert/cxj`), both installing the same release binaries.

### Changed
- MIT license; module path is the real `github.com/umuterturk/cxj`, so
  `go install` works.

## [0.3.1] - 2026-10-28

### Fixed
- **`trail` no longer lends other classes' HTTP routes to same-named
  methods (A8).** `trail` on an outbound client method with no route (e.g.
  `CardPaymentClient.pay`) showed `POST /v1/payments` with certainty
  `exact`: a leftover simple-name suffix match in the trail's http matcher,
  the same root as the `callers` bug fixed in 0.3.0. Routes are now
  owner-exact — a method's card shows its own route, a class's card shows
  its own routes, and a name collision gets an `ambiguous N:` warning
  instead of a guess. Found by dogfooding the 0.3.0 binary itself; fixed
  test-first with a collision fixture and a new accuracy-bench tripwire.

## [0.3.0] - 2026-10-27

The accuracy-and-performance release: a full internal audit (`docs/FIX_PLAN.md`)
measured every command against grep ground truth on real Spring repos, and
each finding shipped as its own fix with a golden that fails on the old
binary.

### Added
- **`api <dependency> [Class]`** — your dependency's real API surface plus
  your own call sites, read from disk (jar/sources via the local build
  cache), with version provenance. (B2)
- **`batch`** — NDJSON in, results out: twenty lookups for the price of
  one `Open` (measured 17× amortization on a 1,354-file repo). (P4)
- **`deps` version ground truth** — declared/derived/cached tiers from the
  build files, BOMs, and the local Gradle/Maven cache; `deps` now says
  *how* it knows a version. (B1)
- **`find` search quality** — substring and camel-hump matching
  (`fPrOrd` finds `FindPaymentRequestOrder`) and type-first ranking with
  a main-before-test source tier. (A4, A6, A7)
- **Honest caps** — capped `trail`/`callers` sections say what they cut;
  `--verbose` shows per-row certainty. (A5)
- **Collision-shaped golden tests** for the whole navigation contract, so
  every accuracy fix arrives with a test that fails on the buggy binary. (A0)
- **Performance gate** — a benchmark harness with CI-enforced CPU/RSS
  budgets; numbers are published either way. (P0)

### Fixed
- **`callers` matches by the receiver's declared type, not its name
  suffix (A3).** The old suffix heuristic returned up to 106 false
  positives on a real repo; untypeable chains (builders, generic
  dispatch, Mockito `verify(...)`) are now labeled `candidate` instead of
  silently trusted or silently dropped.
- **`trail` dropped substring fallbacks that matched almost everything
  (A1)**, and `callees` is scoped to its owner with an explicit
  `ambiguous` answer when a query has more than one plausible symbol (A2).

### Changed
- **Cold-start cost roughly halved on large repos.** Scan was the serial
  fraction, not the index builds (P3); commands build only the sub-indexes
  they read (P2); line numbers come from the parser instead of a second
  file rescan (P5); lookups cost what they return (P7); navigational
  commands skip `src/test` unless asked (P1).
- README and the bundled agent skill rewritten for the shipped tool,
  including the ambiguity rule agents must follow when output says
  `ambiguous`.

## [0.2.x] - 2026-10-25

### Changed
- Iteration on the 0.1 navigation commands and install flow
  (`cxj install -g` writing the agent skill to `~/.agents/skills`).

## [0.1.0] - 2026-10-24

### Added
- First public shape: stateless Java/Spring code-intelligence CLI —
  `brief`, `find`, `endpoints`, `trail`, `callers`, `callees`,
  `references`, `show`, `outline`, `keys`, `beans`, `deps`, `map`,
  `context`, `status`, `batch`, `update` — with `--scope`, `--json`,
  certainty labels, and macOS arm64 releases.

[0.3.2]: https://github.com/umuterturk/cxj/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/umuterturk/cxj/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/umuterturk/cxj/compare/v0.2.3...v0.3.0
[0.2.x]: https://github.com/umuterturk/cxj/compare/v0.1.0...v0.2.3
[0.1.0]: https://github.com/umuterturk/cxj/releases/tag/v0.1.0
