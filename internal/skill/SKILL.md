---
name: cxj
description: Token-cheap Java/Spring workspace queries via the cxj CLI (j = Java). Start with brief, then trail.
---
# cxj CLI

Index `--root` (default cwd) in RAM and exit. Default text is compact. Do not pass `--verbose`. Never run `status`. Never run `ask` unless the question is vague. Do not read a file when `show`/`outline`/`trail` exists.

```
cxj brief                          # first call: kind+versions, main size, profiles, app pkg+depth1 (subtree counts), http, ingress other, out_http peers + out_db
cxj trail SYMBOL                   # one hop: http, injects, callers, callees, supertypes, subtypes, tests; "(+K more)" means the section was capped, the count is true
cxj find QUERY [--limit 5]         # mid-name and humps work: Controller, PPC; main sources rank first
cxj show SYMBOL | cxj outline PATH|SYMBOL
cxj callers|callees SYMBOL         # callers matches the receiver's declared type: callers C.m lists calls on C/subtypes plus untyped-receiver candidates; qualify symbols (bare names are ambiguous)
cxj references SYMBOL              # defs, overrides, and uses of a symbol
cxj keys                           # config names only; do not read application.yml
cxj endpoints [--method GET] [--path /owners]
cxj beans --type SomeType
cxj config --key a.b.c             # only when you need a value
cxj deps [FILTER]                  # dependency versions established from your build files + local caches; tiers: declared/resolved/managed/cached/unknown
cxj api DEP [Type|member]          # a dependency's real API at YOUR resolved version (from local jars, no network) + your repo's usage of it; --sources-only skips workspace scan
```

Many questions at once? `printf '{"cmd":"find","arg":"Order"}\n{"cmd":"callers","arg":"com.acme.Foo.bar"}\n' | cxj batch` — one Open answers all of them (JSONL out, order kept); `open_ms` in the summary shows the amortization.

Workflow: `brief` → `trail SYMBOL` → `show`/`callees`. Need a real argument? `cxj trail --example` (works on every command). `--json` is full; add `--compact` to keep it small. If a line starts `ambiguous N:`, the results describe one of N same-named symbols — re-run with the qualified name it lists. Navigational commands index main sources only (fast); on a miss for a test symbol add `--scope all`. `callers` on an HTTP endpoint is legitimately empty or candidate-only — controllers are called by HTTP, not by code; its route is in its `http` trail row. Re-run after edits. `ask` is optional; first use downloads Needle 3 if you confirm. `cxj install` writes this skill to `.agents/skills` and symlinks `.claude/skills`; `cxj install -g` does the same under `$HOME` and copies the binary to `/usr/local/bin/cxj` (overwrites if present). `cxj version` prints the CLI version; `cxj update` replaces the binary from the latest GitHub release.
