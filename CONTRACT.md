# Frozen MVP external contract (do not casually change)

**CURRENT STATE: specification, NOT IMPLEMENTED.** The CLI/HTTP response contract is binding for all Go black-box acceptance tests. If there is a genuine technical impossibility, stop and document it; do not silently rewrite tests.

## CLI and daemon

```sh
cx serve --root /abs/workspace --http 127.0.0.1:8765 [--exclude 'generated/**'] \
  [--embed-url http://127.0.0.1:11434 --embed-model qwen3-embedding:0.6b]
# omit embed flags => semantic is off for phases 0..2; with embed flags it is on
cx find 'Payment.process' --server http://127.0.0.1:8765 --mode hybrid --json
cx references 'OwnerController' --server http://127.0.0.1:8765 --json
cx endpoints --method GET --path /owners --server http://127.0.0.1:8765 --json
cx beans --type SomeType --server http://127.0.0.1:8765 --json
cx config --key a.b.c --profile prod --server http://127.0.0.1:8765 --json
cx context --symbol Qualified.name --budget 500 --server http://127.0.0.1:8765
cx status --server http://127.0.0.1:8765 --json
# Agent transport is CLI + skills/cx-cli/SKILL.md (no MCP).
```

HTTP only binds to loopback; never run a public server. `GET /v1/status` returns when lexical index is ready, with semantic status independently reported. `POST /v1/shutdown` is **not** required. Tests manage child processes.

## HTTP (UTF-8 JSON)
- `GET /v1/status`: `{ "status":"ready", "indexed_files": N, "indexed_java_files": N, "generation": N, "semantic_state":"off|building|ready|unavailable", "semantic_generation":N, "model":"...", "go_heap_alloc_bytes": N }`. `status=ready` means lexical-ready, not semantic-ready.
- `GET /v1/search?q=...&mode=lexical|semantic|hybrid&limit=10`: `{ "results": [{"path":"relative/forward/slashes","line":1,"symbol":"qualified or empty","kind":"class|method|field|text|config", "match":"exact|prefix|fuzzy|text|semantic|hybrid", "score":0.5, "snippet":"bounded"}], "semantic_state":"..." }`. `line` is 1-based. `limit` defaults 10, maximum 50. Exact symbol hit outranks fuzzy/semantic; deterministic ties by `path,line,symbol`.
- For `mode=semantic`, if embeddings aren't ready return **503** and `{ "error":"semantic_not_ready" }`; for `mode=hybrid` when unavailable return **503** rather than quietly passing lexical results as hybrid. No query-specific translations.
- `GET /v1/references?symbol=Qualified.symbol`: `{ "results":[{"path":"...","line":1,"target":"...","certainty":"exact|candidate|unresolved"}] }`. Do not invent exact references.
- `GET /v1/endpoints?method=GET&path=/owners`: `{ "results":[{"path":"...","line":1,"symbol":"...","http_method":"GET","route":"/owners", "certainty":"exact|candidate"}] }`.
- `GET /v1/beans?type=SomeType`: `{ "results":[{"bean":"...","symbol":"...","injected_into":["..."],"certainty":"exact|candidate|ambiguous"}] }`.
- `GET /v1/config?key=payment.timeout&profile=prod`: `{ "definitions":[{"path":"...","line":1,"value":"...","profile":"..."}], "bindings":["..."], "repository_value":"...", "resolution":"known|external|ambiguous|unknown" }`. `repository_value` omitted/empty when not determinable. Redact secrets (value `"[REDACTED]"`) in all APIs and logs.
- `GET /v1/context?symbol=Qualified.symbol&budget=500`: `{ "symbol":"...", "locations":[...], "relationships":[...], "certainty":"..." }`; must respect bounded text budget.

## Files and exclusions
- `--exclude` is repeatable. `/`-normalized patterns: `generated` excludes that directory recursively; `generated/**` also does; `**/generated/**` excludes any nested generated directory. No symlink following; exclude before parse, text index and embeddings.
- `.cxignore` in root: one path/glob per line, `#` comments. Merge with flags; default exclude `.git`, `node_modules`, `.idea`, `target`, `build`, `out`, `.gradle`; additional user exclusions always respected. Hidden files not otherwise globally excluded.
- Detect UTF-8 text (not binary); max file 1 MiB by default, skip oversized and invalid UTF-8 with explicit counters. Java files get AST + lexical indexing; other text files get lexical search, with YAML/properties also parsed for Spring config. Files other than Java are **not** given invented Java symbol definitions or semantic vectors by default.

## Embeddings
- Local HTTP `POST /api/embed` (Ollama). Default model `qwen3-embedding:0.6b`, overridable; fail explicitly if unavailable. No cloud fallback. Only Java class/method chunks + relevant metadata / comments get vectors. Text docs remain plain search.
- Code chunks from Tree-sitter symbol boundaries; truncate/limit bounded chunks with evidence rather than whole-file arbitrary slices; include identifiers, signature, path, nearby documentation, and Spring annotations. Query and index embeddings use same model and instruction strategy.
- Stable semantic IDs, per-file invalidation and regeneration. Bounded worker queues; semantic state `building` until indexing complete. Store vector matrix exclusively in RAM; float32 first, optimize only after benchmarks. Brute-force dot product first, no vector DB. Hybrid ranking via a documented simple scheme such as reciprocal rank fusion; do not claim score is probability.
- Model weights may reside on disk (Ollama manages them), but repository vectors and indexed metadata may not. Report model process memory separately from Go heap; 512 MiB target refers to Go indexing process, **not** Ollama model which itself may use >600 MB.

## Acceptance design
`./scripts/acceptance.sh N` builds `bin/cx` and runs `go test -count=1 -timeout=20m -v ./acceptance -run '^TestP[0-N]'` (N substituted). Each test launches a real daemon, makes HTTP calls, and checks externally observable behavior. For phases requiring embeddings, model connection must be real. No skips or mocks. Phase 7 uses `PETCLINIC_ROOT` pinned and cloned beforehand. See `acceptance/`.
