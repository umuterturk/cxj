---
name: cx-cli
description: Query a local Java/Spring workspace with the cx CLI (find, references, endpoints, beans, config, context). One-shot index of --root (default cwd); no daemon, no MCP, no embeddings.
---
# cx CLI (agent code intelligence)

Use the `cx` binary against a workspace path. Each invocation indexes `--root` in RAM and exits. Prefer `--json` for machine parsing.

## Setup

```sh
# From the workspace, or pass --root
cx status --json
# status=ready means the lexical index completed for this invocation
```

Set `CX_BIN` if the binary is not on `PATH` (acceptance uses `./bin/cx`).

`--root` defaults to the current working directory. Nonexistent roots fail with nonzero exit.

## Commands

| Need | Command |
|------|---------|
| Symbol / text search | `cx find 'QUERY' [--root PATH] [--limit 10] --json` |
| References | `cx references 'Type.method' [--root PATH] --json` |
| HTTP routes | `cx endpoints [--root PATH] [--method GET] [--path /owners] --json` |
| Beans / injection | `cx beans --type SomeType [--root PATH] --json` |
| Config key | `cx config --key a.b.c [--profile prod] [--root PATH] --json` |
| Bounded context | `cx context --symbol Qualified.name [--budget 500] [--root PATH]` |
| Index counters | `cx status [--root PATH] --json` |

Repeatable `--exclude PATTERN` applies to every command.

## Search

Lexical only: exact/prefix/fuzzy identifiers and plain text lines. Exact identifier hits always rank first. Do not invent synonyms or translations.

## Certainty rules (do not overclaim)

- `exact` only when the index is sure (unique qualifier, unambiguous bean, etc.).
- Interface dispatch / multi-bean injection may be `candidate` or `ambiguous` — report that; do not treat as proven runtime wiring.
- `@Transactional` in context is **declared** only, not proven effective (self-invocation may bypass proxies).
- Config `resolution=external` means placeholders like `${ENV}` — do not invent runtime values.
- Secret-like keys appear as `[REDACTED]`.

## Workflow

1. `cx status --json` — confirm `status=ready` and file counts.
2. Known symbol → `cx find 'OwnerController' --json`.
3. Spring HTTP → `cx endpoints --method GET --path /owners --json`.
4. DI → `cx beans --type PaymentService --json`.
5. Properties → `cx config --key server.port --profile prod --json`.
6. Deepen → `cx context --symbol OwnerController --budget 500`.

Do not write on-disk indexes. Re-run after edits; there is no live watcher.
