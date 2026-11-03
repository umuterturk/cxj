# cxj — distribution + marketing plan (marketplaces, launch, reach)

One goal: get cxj in front of the people who wire agents into Java/Spring work, through the
channels where agent tools are actually installed in late 2026. Same law as FIX_PLAN: **cxj
has no state.** Distribution must not break it — no daemon, no persistent service, no hosted
account required to use the tool. An install is a binary plus a skill file; an uninstall is
`rm`. Every marketplace we join must be joinable without violating that (a listing is not
state; a daemon is).

Positioning line used everywhere below, verbatim:

> **Persistent-graph tools are a memory; cxj is a measuring instrument.** Every answer comes
> from a fresh read of your source, so no answer can be stale, and every edge says whether it
> is proven or inferred. The harness is the natural-language layer — cxj never was a model
> and removed the one it had (v0.4.0).

Grounded in `/Users/umut.erturk/mycode/cxj` at `13ac6ae` (v0.4.0 shipped: 4 release assets,
tap at 0.4.0, npm wrapper staged at 0.4.0) and marketplace research verified **2026-11-03**.

---

## 0. The channel landscape (verified 2026-11-03)

| Channel | What it is | Entry | Cost / gate |
|---|---|---|---|
| skills.sh (Vercel) | Cross-agent SKILL.md installer: `npx skills add owner/repo`; ~490k skills across marketplaces as of Mar 2026 | Repo exposes an installable skill; request index | Free |
| Claude Code plugin marketplaces | `.claude-plugin/marketplace.json` in a repo → `claude plugin marketplace add umuterturk/cxj` | Own repo, no review | Free |
| Anthropic's Claude directory | claude.com/marketplace/plugins; syncs into Claude Code sessions; install counts visible | Portal `claude.ai/directory/manage` (opened 2026-09-25) | Paid claude.ai plan + review |
| ClaudePluginHub | Auto-generated hub; listing page renders the repo README | Claim with the repo-owning GitHub account | Free |
| Official MCP Registry | registry.modelcontextprotocol.io, ~53.5k servers; **Glama / mcp.directory / PulseMCP auto-import from it** | `mcp-publisher` CLI, namespace `io.github.umuterturk` | Free; needs an actual MCP server (the T3 shim) |
| Glama | Largest MCP index (~110k), quality grades, claimable badge | Auto-index + claim | Free |
| mcp.so | Reviewed community directory | Premium tier is the found path | $39 |
| Smithery | Directory only — proxy/managed layer **shut down 2026-10-31** | skip | — |
| OpenCode plugins | A plugin is an npm package listed in `opencode.json` `"plugin": [...]` | `npm publish` | npm account |
| Homebrew | Public tap already live (`umuterturk/tap/cxj` at 0.4.0) | done | — |
| Launch venues | Show HN, r/ClaudeAI, r/java, r/ExperiencedDevs, Java Weekly, Baeldung weekly, SustainIt, The Changelog, Console.dev/TLDR "made with" slots | one coordinated moment | your accounts |

Competitor for mindshare: **codebase-memory-mcp (DeusData, ~41k stars)** — persistent SQLite
knowledge graph, 17 MCP tools, daemon + watchers, arXiv paper claiming 10× token / 2.1×
tool-call savings vs file-by-file exploration. Their own README spends pages on when the
graph might be **stale** (unwatched non-git roots, `watcher_enabled` read only at daemon
start, "no recorded gap ≠ complete"). That is our best ad: quote it, don't strawman it.

---

## 1. P0 — prerequisites before any listing or launch

Nothing gets promoted until these are green; every marketplace comment section asks the same
three questions (trust, uninstall, proof), and we answer them pre-emptively.

- [x] **P0.1 Trust section in README** (agent). Checksums table for the four release zips +
  one-line verification command (`shasum -a 256` / `sha256sum`); Gatekeeper note for the
  unsigned macOS zip (`xattr -d com.apple.quarantine cxj`, brew/npm paths unaffected); a
  "how cxj binaries are built" sentence (public CI, four runners, `file` header smoke).
  CBM defuses the curl-bash objection with a VirusTotal badge; our honest equivalent is
  published digests + reproducible-looking CI + "the installer is `brew install`".
  *Done 2026-11-03: digests recomputed from live v0.4.0 assets before publishing the table.*
- [x] **P0.2 Uninstall section in README** (agent): `brew uninstall umuterturk/tap/cxj`,
  `npm rm -g @umutert/cxj`, `rm $(which cxj)` + skill dirs. Two lines, big trust.
  *Done 2026-11-03.*
- [x] **P0.3 Repo polish** (agent): GitHub About text + topics check + social preview image;
  CONTRIBUTING.md; issue templates (bug form that demands `cxj version`, repo shape, exact
  command — "measured reproductions get fixed fastest" made operational).
  *Done 2026-11-03: About + 11 topics (added `claude-code`) via API; CONTRIBUTING +
  structured bug/feature templates with placeholders verified against real `cxj status`
  output. One user minute remains: the social preview has no upload API — drag
  `docs/assets/social-preview.png` into Settings → General.*
- [ ] **P0.4 The number** (agent designs, runs on payment-api/enigma-card-api/pos):
  token-comparison experiment — same 10 orientation questions answered by (a) an agent with
  only file-reading and (b) an agent with the cxj skill; record tokens + tool calls. Publish
  as `docs/MEASURING.md` with the raw transcript and a "run it yourself" script. CBM set the
  expectation that you show measurements; this is our table-version of their arXiv.
- [ ] **P0.5 Demo GIF** (user films, agent scripts the session): `brief → trail → show` on a
  real repo, ms timer visible, ≤20 s. README top + marketplace listings.
- [ ] **P0.6 npm republish 0.4.0** (user, 2FA): `cd npm && npm publish` (wrapper is already
  committed at 0.4.0 in `c2fccfb`).
- [ ] **P0.7 Machine hygiene** (user): delete ask-era `~/bin/cxj` (shadows brew binary in
  PATH) and `/opt/homebrew/bin/cxj.pre-brew-backup`.
- [x] **P0.8 Known gap: `api webflux`** — **disproven, record corrected** (agent,
  `36e3861`). Reproduced against pos-integration-api: the substring filter reaches
  `spring-boot-starter-webflux` at both layers; the honest answer is "no version
  established" because the BOM-managed starter has no declared version and no jar
  in either local cache (`spring-webflux` 7.0.8 is cached; the starter is not).
  Pinned with `TestApiStarterSubstringMatch` so the match can never rot into the
  "no dependency matches" lie.

Definition of done: fresh-eyes read of README answers trust / uninstall / proof without
leaving the page; every badge and command verified live.

---

## 2. Tier 1 — native fits (skill + plugin formats; no new runtime)

cxj already ships exactly what these marketplaces distribute: a SKILL.md. Tier 1 is
manifests and layout, zero engine code.

### 2.1 Claude Code plugin + our own marketplace (anchor — do first)

- [x] **T1.1** Repo layout: `.claude-plugin/marketplace.json` (marketplace `cxj`, one plugin
  `cxj`), plugin dir with `.claude-plugin/plugin.json` + `skills/cxj/SKILL.md`. The plugin's
  skill content is the same file the binary embeds. *Done 2026-11-03 (`58a799f`), plus
  `.claude-plugin/icon.png` — square, generated; the directory freezes the icon at first
  portal submission, so it had to exist before T1.4.*
- [x] **T1.2** Single source of truth: `internal/skill/SKILL.md` stays canonical;
  `scripts/sync-skill.sh` copies it to the plugin skill dir; **CI drift check** fails if the
  copies differ (house pattern: contracts enforced in CI, not by memory). *Done — the check
  also compares manifest versions against the newest CHANGELOG release, and caught the
  0.3.2/0.4.0 fold-forward on its first run.*
- [x] **T1.3** `claude plugin validate .` locally (and in CI if the CLI installs headless);
  README install section gains: `claude plugin marketplace add umuterturk/cxj` →
  `/plugin install cxj@cxj`. *Done: both manifests pass `--strict` with zero warnings via
  Claude Code 2.1.317 (npx), and the full loop was proven end-to-end locally —
  `marketplace add` → `install cxj@cxj` → `list` (v0.4.0) → `details`: Skills (1) cxj.
  CI runs the cheap drift check; the authoritative validate stays a release-day step.
  The validator itself recommends the one-step form `/plugin install cxj --marketplace
  umuterturk/cxj` (Claude Code ≥ 2.1.275), which is the README's lead line.*
- Accept: plugin installs in Claude Code and the skill drives a correct `brief → trail`
  session on a scratch Spring repo with no manual setup beyond the two commands.

### 2.2 Anthropic's official directory (account-gated)

- [ ] **T1.4** (user) Submit the T1.1 plugin via `claude.ai/directory/manage` from your paid
  claude.ai plan. Notes: names are immutable once accepted; the official
  `claude-plugins-official` repo is partner-only, the portal is the real door. Track review
  state in the portal; usage analytics appear once live.

### 2.3 skills.sh (biggest cross-agent funnel)

- [x] **T1.5** Top-level `skills/cxj/SKILL.md` (synced, same drift check as T1.2) so
  `npx skills add umuterturk/cxj` resolves; the skill's frontmatter description must earn the
  click in a 490k-skill directory. *Done 2026-11-03: the install command was run live against
  the pushed repo — one skill installed to `.agents/skills/cxj`, byte-identical to the
  canonical copy. The canonical description now leads with "Stateless Java/Spring code
  intelligence".*
- [ ] **T1.6** (user) skills.sh index request / GitHub login to claim; README badge + the
  one-liner. *The one-liner is in the README and works today (proven); the site listing +
  verified badge are the remaining user step.*
- [x] **T1.7** agentskills.io standard compliance pass (frontmatter fields exactly per spec;
  it's what Codex/Copilot/OpenCode read too). *Verified: `name` matches the directory,
  `description` within limits; skills.sh and Claude Code both parsed and loaded it.*

### 2.4 Hubs + aggregators (free listings, one afternoon)

- [ ] **T1.8** (user claims, agent prepares) ClaudePluginHub claim with the repo-owning
  GitHub account — its page renders the README, so P0.1/P0.2 must be done first.
- [ ] **T1.9** PRs: community `marketplace.json` collections, `awesome-claude-code`,
  awesome agent-skills lists, `awesome-java` (tool section), `awesome-opencode`. Each PR:
  2-line description using the positioning line, no marketing voice. *Lines to paste
  (per-room rewrite still required):*
  > **[cxj](https://github.com/umuterturk/cxj)** — stateless Java/Spring code intelligence
  > for coding agents. Cold-open `brief`/`trail`/`endpoints`/`deps` from a fresh read of
  > your source: no daemon, no index, cannot serve stale. Ships as an agent skill + Claude
  > Code plugin; `claude plugin install cxj --marketplace umuterturk/cxj`.
- [x] **T1.10** README "Where to find cxj" list (brew, npm, zips, skills.sh, Claude
  marketplace, OpenCode when T2.1 lands) — discovery page for people who arrive late.
  *Done 2026-11-03: the Install table is the list (six channels); OpenCode row waits on T2.1.*

---

## 3. Tier 2 — small new artifacts, high leverage

### 3.1 OpenCode plugin (npm)

- [ ] **T2.1** `opencode-cxj` npm package (own scope: `@umutert/opencode-cxj` or bare name —
  decide at publish): a plugin whose `setup` ensures the cxj skill is registered (and offers
  to run `cxj install -g` / brew install hint if the binary is absent — plugin checks, never
  silently downloads). Users add `"plugin": ["opencode-cxj"]` to `opencode.json`.
- [ ] **T2.2** PR/submit to OpenCode community plugin lists; announce in the OpenCode
  Discord (their ecosystem is the hungriest for Java tooling; CBM's discussion #2025 —
  agents refusing MCP tools — is the cautionary tale to cite, without naming them).
- Note: this is the only npm publish that stays routine-manual (2FA); automation stays a
  goal behind repo secrets (T4.5).

### 3.2 Nix + pkgx (installer-culture credibility)

- [ ] **T2.3** `flake.nix` in the cxj repo building from source (go_1_23, tree-sitter cgo
  handling explicit). CBM ships Nix/Scoop/AUR; we don't do Windows, and the flake is what
  makes "macOS + Linux only" read as a decision rather than a gap.
- [ ] **T2.4** pkgx dev reference (or explicit note why not). Scoop/AUR = Windows-only /
  out of scope; stated once in README's platform line, not apologized for repeatedly.

---

## 4. Tier 3 — the MCP compat shim (un-deferred: this is where raw discovery traffic lives)

Design constraint first: **the no-state law survives.** `cxj mcp` is a stdio server that
implements `initialize` / `tools/list` / `tools/call`, and every `tools/call` runs the same
one-shot `Open → query → exit` engine in-process. No daemon, no index persistence, no
watcher, no `resources/list` fiction — return **empty lists, not Method-not-found** (the
Smithery scanner error documented in the field reports). Tools are the commands: `brief`,
`find`, `trail`, `show`, `outline`, `callers`, `callees`, `endpoints`, `beans`, `config`,
`keys`, `deps`, `api`. Output is the same JSON the CLI emits. README labels the shim
honestly: *compat transport for MCP-only harnesses; the skill + CLI stays the recommended
path.*

- [ ] **T3.1** `internal/mcp/` + `cxj mcp` subcommand; JSON-RPC stdio; golden tests per tool
  (reuse batch's JSONL fixtures — one engine, three transports).
- [ ] **T3.2** `server.json` per registry spec; publish via `mcp-publisher` under
  `io.github.umuterturk/cxj` (GitHub-verified namespace; org namespaces need OIDC/token —
  personal namespace is enough). Versions are immutable: every description tweak = new
  version, so write them carefully once.
- [ ] **T3.3** Claim the auto-imports: Glama (+ verified badge), PulseMCP, mcp.directory
  (email). mcp.so: decide on the $39 listing after the launch spike, not before.
- [ ] **T3.4** Shipped with a release: `cxj mcp` lands as its own minor (v0.5.0), with the
  perf gate extended: `cxj mcp --once '<initialize+tools/list>'` must cold-start within the
  same budget as `batch`.
- Accept: an MCP-only client (Cursor or Claude Code without skills) can answer "who calls
  PaymentController.pay" with one tool call and certainty labels intact.

---

## 5. Launch moment (after P0 + Tier 1 are green; all venues in one week)

Order inside the week: blog post → Show HN → Reddit → Java newsletters. Same GIF, same
number (P0.4), same positioning line everywhere.

- [ ] **L.1 Blog post** on a personal domain (cross-post dev.to): *"Your agent doesn't need
  a codebase memory"* — the no-state argument, the CBM-freshness-quote move, the payment-api
  measurement table, ending with the install one-liners. Bare repo links underperform on HN;
  an argument outperforms a tool announcement.
- [ ] **L.2 Show HN** (user posts): "Show HN: cxj – stateless Java code-intelligence for
  coding agents (I removed the AI from it)". Tue–Thu ~07:00 PT. Comment-plan: have the P0.1
  trust answer, the P0.4 number, and the "why not MCP-first" answer pre-drafted; the CBM
  comparison comment gets posted by the user, not the author account, if a moderator flags
  self-praise (standard practice: author answers facts, comparisons come from the crowd).
- [ ] **L.3 Reddit**: r/ClaudeAI + r/ExperiencedDevs ("my agent re-reads 40 files per
  question" answer with the batch timing row), r/java (tool announcement + the webflux/bean
  depth story). No crossposting spam; each post rewritten for its room.
- [ ] **L.4 Java channels**: Java Weekly + Baeldung weekly (tool submission), SustainIt
  (community-submitted tools; Spring angle), The Changelog pitch (the "removed the AI from
  the AI tooling" hook is the episode title), Console.dev + TLDR made-with slots.
- [ ] **L.5 X/LinkedIn clip**: the GIF + one sentence. Scheduled same day as HN.
- [ ] **L.6 Ask HN (optional, follow-up week)**: "Ask HN: how stale is your agent's code
  index?" — the question is the argument; answers become the ammunition for the next post.

---

## 6. Tier 4 — ops + long tail (keeps the machine from rotting between releases)

- [ ] **T4.1 Tap bump automation** (uses `/Users/umut.erturk/mycode/homebrew-tap`): cxj's
  release workflow computes digests and dispatches a `repository_dispatch` to the tap repo,
  whose workflow edits `Formula/cxj.rb`, runs `brew audit --strict`, commits, pushes —
  deleting the manual digest dance (this release did it by hand: four `shasum`s + heredoc).
  Tap repo needs a `HOMEBREW_*`-free self-hosted-free setup: GITHUB_TOKEN + a trusted tap
  (already trusted on the maintainer machine; CI just audits).
- [ ] **T4.2 npm publish automation** behind repo secrets (NPM_TOKEN with 2FA override for
  publish is not allowed on most accounts — verify; if blocked, keep manual and automate
  everything up to the `npm publish` line, which prints in the release job log).
- [ ] **T4.3 Listing-drift check**: weekly CI cron that fetches the official registry entry,
  Glama page, and directory listings and diffs description/version against the repo's
  `server.json` + latest tag (field reports: listings drift silently for months).
- [ ] **T4.4** Skills catalog on GitHub Pages (`index.json` + SKILL.md served raw) — the
  one-URL install for harnesses that take catalog endpoints.
- [ ] **T4.5** Windows decision (open, parked): the cgo cross-build recipe now proven on
  macOS-x86_64 *might* port to `GOOS=windows` (tree-sitter C compiles with zig/mingw). No
  promises, no "coming soon" in README until an asset exists in a release. CBM has Windows;
  that's an honest scoreboard line, not a TODO we pretend-close.
- [ ] **T4.6** Post-launch: harvest Show HN/Reddit objections into FAQ entries; each real
  issue reported from a launch link gets a FIX_PLAN-tier fix + a "you said, we shipped"
  follow-up post.

---

## 7. Sequencing (owners: agent = doable now; user = account/hands)

| # | Item | Owner | Depends on |
|---|---|---|---|
| 1 | P0.1–P0.3 README trust/uninstall/repo polish | agent | — |
| 2 | P0.8 webflux `api` fix | agent | — |
| 3 | T1.1–T1.3 Claude plugin + own marketplace | agent | P0.1 |
| 4 | T1.5–T1.7 skills.sh layout + index request | agent + user | T1.2 |
| 5 | P0.4 measurement + P0.5 GIF | agent + user | P0.8 |
| 6 | P0.6 npm publish, P0.7 hygiene | user | — |
| 7 | T2.1–T2.2 OpenCode plugin | agent + user (npm) | P0.6 |
| 8 | T1.4 Anthropic directory submission | user | T1.1–T1.3 |
| 9 | T1.8–T1.10 hub claims + awesome-list PRs | agent + user | P0.1 |
| 10 | L.1–L.5 coordinated launch week | both | 1–9 |
| 11 | T3.1–T3.4 MCP shim v0.5.0 + registry publish | agent + user (claims) | launch done |
| 12 | T4.1 tap automation, T4.3 drift cron | agent | — |

Rule from FIX_PLAN applies unchanged: **each phase lands as one green commit** (suite +
perf gate + the phase's own acceptance check), tag per release, README claims re-verified
live before every commit that touches them. A marketplace listing is a claim; claims get
verified the same way flag names do.
