---
name: cxj
description: Token-cheap Java/Spring workspace queries via the cxj CLI (j = Java). Start with brief, then trail.
---
# cxj CLI

Index `--root` (default cwd) in RAM and exit. Default text is compact. Do not pass `--verbose`. Never run `status`. Never run `ask` unless the question is vague. Do not read a file when `show`/`outline`/`trail` exists.

```
cxj brief                          # first call: kind, entry pkg+depth1 (main|test), ALL ingress, profiles
cxj trail SYMBOL                   # one hop: http, injects, callers, callees, tests
cxj find QUERY [--limit 5]
cxj show SYMBOL | cxj outline PATH|SYMBOL
cxj callers|callees SYMBOL
cxj keys                           # config names only; do not read application.yml
cxj endpoints [--method GET] [--path /owners]
cxj beans --type SomeType
cxj config --key a.b.c             # only when you need a value
```

Workflow: `brief` → `trail SYMBOL` → `show`/`callees`. Need a real argument? `cxj trail --example` (works on every command). `--json` is full; add `--compact` to keep it small. Re-run after edits. `ask` is optional; first use downloads Needle 3 if you confirm. `cxj install` writes this skill to `.agents/skills` and symlinks `.claude/skills`; `cxj install -g` does the same under `$HOME` and on macOS copies the binary to `~/bin/cxj`.
