# Measuring cxj against file-reading

The launch question is "how many tokens does this actually save?" This page answers it
with a measurement, not an estimate — and it reports the case where cxj **lost**.

## What was measured

Twelve orientation questions about one production Spring service, answered twice:

- **Arm A (baseline)** — an agent limited to the tools agents use today: `rg`/`grep`,
  `find`, `sed`, `head`, `wc`, and file reads.
- **Arm B (cxj)** — an agent that loaded the bundled `cxj` skill and used only the `cxj`
  CLI (no grepping, no reading source files).

Both arms ran in **fresh sessions with the same model, the same agent configuration and
the same question text**; only the tooling rule differed. Answers were checked against
ground truth captured independently *before* the runs.

**Provenance (2026-11-03)**

| | |
| --- | --- |
| Repository | `payment-api` @ `5bcf691` — 561 Java files under `src/main`, Java 25, Spring Boot 4.1.0 |
| Tool | `cxj v0.4.0` (Homebrew build) |
| Harness | OpenCode, `explore` agent, model `deepseek-v4.1-flash` (high variant) |
| Accounting | the harness's per-session records (`tokens_input`, `tokens_cache_read`, `tokens_output`, `tokens_reasoning`, `cost`) — read from the session store, not estimated |
| Machine | Apple M3 Pro, macOS 14.4 |

`tokens_input` is full-price prompt input; `tokens_cache_read` is the cached share.
`cost` is the provider's own accounting and is the fairest single economic number.

## Round 1 — structural questions (where cxj is designed to win)

Six questions that each require sweeping the repo: list every HTTP endpoint; which
outbound HTTP clients the service calls; which DB entities it persists; the exact
resolved `spring-kafka` version and its declaring build line; which Spring profiles are
configured; what `PaymentController` injects.

| | Arm A (grep + read) | Arm B (cxj) | B vs A |
| --- | ---: | ---: | ---: |
| Full-price input tokens | 78,743 | 12,258 | **6.4× fewer** |
| Cached prompt tokens | 270,080 | 28,416 | 9.5× fewer |
| Output + reasoning tokens | 5,597 | 1,924 | 2.9× fewer |
| Cost (provider accounting) | $0.02403 | $0.00427 | **5.6× cheaper** |
| Wall clock | 41.8 s | 19.1 s | 2.2× faster |
| Assistant steps | 11 | 8 | — |
| Tool calls | 32 | 13 | — |

Both arms answered all six correctly. Arm A's shape is visible in the transcript: eight
controller files read in full, plus `build.gradle` and a dozen greps. Arm B ran
`cxj brief`, `endpoints`, `deps`, `show`/`outline`, and `status`.

## Round 2 — one-line annotation lookups (where grep wins)

Six questions answerable from a single annotation or filename: the two `@Scheduled`
jobs; the `@KafkaListener` consumer; which class implements `PaymentIdempotencyChecker`;
which method serves `GET /v1/payments/{paymentId}`; which class defines the injected
`Bus` type (and whether it is external); the count of Java files under `src/main`.

| | Arm A (grep + read) | Arm B (cxj) | B vs A |
| --- | ---: | ---: | ---: |
| Full-price input tokens | 13,505 | 32,377 | 2.4× **more** |
| Cached prompt tokens | 19,200 | 111,360 | 5.8× more |
| Output + reasoning tokens | 1,637 | 4,416 | 2.7× more |
| Cost (provider accounting) | $0.00412 | $0.01147 | 2.8× **more expensive** |
| Wall clock | 13.2 s | 40.0 s | 3.0× slower |
| Assistant steps | 6 | 13 | — |
| Tool calls | 14 | 26 | — |

Both arms answered all six correctly here too. This is the honest counterexample:
**for a question whose answer is one annotation in one file, `grep` is cheaper.** Reaching
for cxj there pays a fixed skill cost and a discovery loop for no sweep.

Two measured reasons the gap is this wide, both visible in the transcript:

- the cxj arm used `--json` on most calls (verbose payloads) instead of the compact
  default text;
- it spent several calls finding the right query (`find KafkaListener` before
  `find Consumer`), which is discovery cost a less exploratory agent would not pay.

Both are addressable; the raw numbers above are what the shipped v0.4.0 skill produced
in a single run.

**Action taken after this measurement:** the bundled skill now states plainly that
un-compacted `--json` is the expensive form — plain text, or `--json --compact` when the
agent needs to parse.

## What the two rounds mean

Use cxj where the answer **spans files or requires build truth**; use `grep`/read where
the answer is a single line you can locate mechanically.

| Question shape | Cheaper arm in this measurement |
| --- | --- |
| "List every endpoint / client / entity / job / listener" | cxj (swept in one call; no per-file reads) |
| "Which version is actually resolved, and where is it declared" | cxj (build files + BOM + caches, one answer) |
| "What does this class inject", "who calls this", "what does this call" | cxj (declared-type resolution, certainty labels) |
| "Which method maps `GET /x/{id}`" | either — single annotation |
| "Which classes extend/implements X", "count files", "read this method" | `grep`/read — one file, one pass |

Across all twelve questions the cxj arm used **2.1× fewer full-price input tokens**
(44,635 vs 92,248) and **1.8× less cost** — but that aggregate is dominated by Round 1,
so it should not be quoted as a per-question average.

## Caveats (read before quoting)

1. **One run per arm.** Model output is nondeterministic; treat ratios as directional.
   The method is what reproduces — repeat it on your repo and compare.
2. **One repository, one model, one harness, twelve questions.** Results on a 50-file
   service or a 100k-file monorepo will differ.
3. **Arm B's skill text is counted against it.** The ~2.4 KB `SKILL.md` is prompt cost;
   it is paid once per session and amortizes over every later question.
4. **Arm B used `--json` heavily.** Compact text output would shrink its footprint. The
   numbers are therefore an upper bound for the cxj arm, not a best case.
5. **Wall clock includes model latency**, so it measures the whole loop, not cxj's own
   milliseconds (those are in the [README](../README.md#performance)).
6. **Correctness was not the variable** — both arms were right in both rounds. This page
   measures what it cost to be right.

## Reproducing it

The prompts are the whole experiment. Give two fresh sessions this question list, with
one line changed:

- **Arm A:** "Use ONLY file exploration — rg/grep, find, ls, sed, head, wc, and reading
  files. You MUST NOT run `cxj`."
- **Arm B:** "First load the `cxj` skill, then use ONLY the `cxj` CLI. You MUST NOT grep,
  find, or read source files yourself."

Then read the harness's accounting. For OpenCode:

```sh
sqlite3 ~/.local/share/opencode/opencode.db \
  "select tokens_input, tokens_cache_read, tokens_output, tokens_reasoning, cost
     from session_v2 where id='<session-id>';"
```

The deterministic half — the cxj commands themselves and their output footprint — can be
replayed without a model:

```sh
./scripts/measure-tokens.sh --repo /path/to/java/repo
```

It runs the twelve cxj calls this experiment used and prints lines/bytes per call, so you
can see the surface area cxj replaces even if you do not re-run the agents.

_Measured 2026-11-03. Numbers belong to the machine, model and repo above; the method
does not._
