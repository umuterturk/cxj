---
name: cx-cli
description: Query a local Java/Spring workspace with the cx CLI (find, references, endpoints, beans, config, context). Use instead of MCP when exploring code, Spring routes/DI, or configuration.
---
# cx CLI (agent code intelligence)

Use the `cx` binary against a running local daemon. Do **not** use MCP. Prefer `--json` for machine parsing.

## Setup

```sh
# Terminal A — keep running (loopback only)
cx serve --root /ABS/WORKSPACE --http 127.0.0.1:8765 \
  [--exclude 'generated/**'] \
  [--embed-url http://127.0.0.1:11434 --embed-model qwen3-embedding:0.6b]

# Wait until ready
cx status --server http://127.0.0.1:8765 --json
# status=ready means lexical index is up; semantic_state may still be building
```

Set `CX_BIN` if the binary is not on `PATH` (acceptance uses `./bin/cx`).

## Commands

All query commands take `--server http://127.0.0.1:8765`.

| Need | Command |
|------|---------|
| Symbol / text / semantic search | `cx find 'QUERY' --server URL --mode lexical\|semantic\|hybrid [--limit 10] --json` |
| References | `cx references 'Type.method' --server URL --json` |
| HTTP routes | `cx endpoints --server URL [--method GET] [--path /owners] --json` |
| Beans / injection | `cx beans --type SomeType --server URL --json` |
| Config key | `cx config --key a.b.c [--profile prod] --server URL --json` |
| Bounded context | `cx context --symbol Qualified.name [--budget 500] --server URL` |
| Daemon health | `cx status --server URL --json` |

## How to choose mode

- **lexical**: exact/prefix/fuzzy identifiers and plain text lines. Default first try for known names.
- **semantic**: natural language / multilingual intent. Requires embed flags and `semantic_state=ready`. HTTP 503 means not ready — wait or fall back to lexical; never invent synonyms.
- **hybrid**: merges both; exact identifier hits still win. Also 503 if semantic unavailable when hybrid was requested.

## Certainty rules (do not overclaim)

- `exact` only when the index is sure (unique qualifier, unambiguous bean, etc.).
- Interface dispatch / multi-bean injection may be `candidate` or `ambiguous` — report that; do not treat as proven runtime wiring.
- `@Transactional` in context is **declared** only, not proven effective (self-invocation may bypass proxies).
- Config `resolution=external` means placeholders like `${ENV}` — do not invent runtime values.
- Secret-like keys appear as `[REDACTED]`.

## Workflow

1. `cx status --json` — confirm `status=ready`.
2. Known symbol → `cx find 'OwnerController' --mode lexical --json`.
3. Concept / other language → wait for `semantic_state=ready`, then `cx find '...' --mode semantic --json`.
4. Spring HTTP → `cx endpoints --method GET --path /owners --json`.
5. DI → `cx beans --type PaymentService --json`.
6. Properties → `cx config --key server.port --profile prod --json`.
7. Deepen → `cx context --symbol OwnerController --budget 500`.

Stay on loopback. Do not start public binds. Do not write on-disk indexes.
