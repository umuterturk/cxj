---
name: acceptance-gates
description: Build and enforce understandable black-box Go acceptance tests for phase gates without mocks, skips, hardcoded vocabulary shortcuts, or weakened correctness requirements.
---
# Acceptance gates

Read `../../CONTRACT.md` and `../../acceptance/`. Tests run the real compiled binary, create temporary Java/config/text files and talk to the public HTTP/MCP API. Model tests call actual local Ollama, never fake vectors. `go test -count=1` must run every test from Phase 0 through current; no `t.Skip`. When model or fixture dependency is absent: fail with setup error, not silently skip.

Guard against vacuous tests: verify that results point to correct file and line; verify exclusions in all search modes; verify ambiguity instead of fabricated exact references; test multiple unseen semantic queries. Avoid brittle ranking assertion from a single one-off phrase; use a documented recall threshold across a fixed diverse dataset. Keep Go tests simple (one scenario = one Test function), standard library where possible.
