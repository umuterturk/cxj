# Frozen MVP external contract (do not casually change)

**CURRENT STATE: specification, NOT IMPLEMENTED.** The CLI response contract is binding for all Go black-box acceptance tests. If there is a genuine technical impossibility, stop and document it; do not silently rewrite tests.

## CLI

Each command one-shot indexes `--root` (default: current working directory) in RAM, answers, and exits. No daemon, no HTTP server, no embeddings, no on-disk index. File edits are visible on the next invocation.

```sh
cx status [--root PATH] [--exclude 'generated/**']... [--json]
cx find 'Payment.process' [--root PATH] [--limit 10] [--exclude PATTERN]... [--json]
cx references 'OwnerController' [--root PATH] [--exclude PATTERN]... [--json]
cx endpoints [--method GET] [--path /owners] [--root PATH] [--exclude PATTERN]... [--json]
cx beans --type SomeType [--root PATH] [--exclude PATTERN]... [--json]
cx config --key a.b.c [--profile prod] [--root PATH] [--exclude PATTERN]... [--json]
cx context --symbol Qualified.name [--budget 500] [--root PATH] [--exclude PATTERN]...
```

`--root` must exist and be a directory; nonexistent roots fail with nonzero exit. Tests exec the real binary; they do not start a server.

## JSON (UTF-8)
- `cx status --json`: `{ "status":"ready", "indexed_files": N, "indexed_java_files": N, "generation": N, "go_heap_alloc_bytes": N }`.
- `cx find QUERY --json`: `{ "results": [{"path":"relative/forward/slashes","line":1,"symbol":"qualified or empty","kind":"class|method|field|text|config", "match":"exact|prefix|fuzzy|text", "score":0.5, "snippet":"bounded"}] }`. `line` is 1-based. `limit` defaults 10, maximum 50. Exact symbol hit outranks fuzzy; deterministic ties by `path,line,symbol`.
- `cx references SYMBOL --json`: `{ "results":[{"path":"...","line":1,"target":"...","certainty":"exact|candidate|unresolved"}] }`. Do not invent exact references.
- `cx endpoints --json`: `{ "results":[{"path":"...","line":1,"symbol":"...","http_method":"GET","route":"/owners", "certainty":"exact|candidate"}] }`.
- `cx beans --type SomeType --json`: `{ "results":[{"bean":"...","symbol":"...","injected_into":["..."],"certainty":"exact|candidate|ambiguous"}] }`.
- `cx config --key payment.timeout [--profile prod] --json`: `{ "definitions":[{"path":"...","line":1,"value":"...","profile":"..."}], "bindings":["..."], "repository_value":"...", "resolution":"known|external|ambiguous|unknown" }`. `repository_value` omitted/empty when not determinable. Redact secrets (value `"[REDACTED]"`) in all APIs and logs.
- `cx context --symbol Qualified.symbol [--budget 500]`: `{ "symbol":"...", "locations":[...], "relationships":[...], "certainty":"..." }`; must respect bounded text budget.

## Files and exclusions
- `--exclude` is repeatable. `/`-normalized patterns: `generated` excludes that directory recursively; `generated/**` also does; `**/generated/**` excludes any nested generated directory. No symlink following; exclude before parse and text index.
- `.cxignore` in root: one path/glob per line, `#` comments. Merge with flags; default exclude `.git`, `node_modules`, `.idea`, `target`, `build`, `out`, `.gradle`; additional user exclusions always respected. Hidden files not otherwise globally excluded.
- Detect UTF-8 text (not binary); max file 1 MiB by default, skip oversized and invalid UTF-8 with explicit counters. Java files get AST + lexical indexing; other text files get lexical search, with YAML/properties also parsed for Spring config. Files other than Java are **not** given invented Java symbol definitions.

## Acceptance design
`./scripts/acceptance.sh N` builds `bin/cx` and runs `go test -count=1 -timeout=20m -v ./acceptance -run '^TestP[0-N]'` (N substituted). Each test execs the real compiled binary against a temporary `--root` and checks stdout JSON. No skips or mocks. Phase 7 uses `PETCLINIC_ROOT` pinned and cloned beforehand. See `acceptance/`.
