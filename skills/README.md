# Builder skills

Each folder contains a portable `SKILL.md` with YAML frontmatter following the Agent Skills specification.

- `orchestrator`: phase controller
- `acceptance-gates`: enforce Go black-box gates
- `java-index`: Java grammar / IR / index
- `semantic-search`: local multilingual embedding retrieval
- `spring-config`: Spring bean/config graph
- `files-exclusions`: other text and path filters
- `cx-cli`: agent-facing CLI usage (replaces MCP)
- `handoff`: compact context and restart cleanly

An agent can explicitly read these paths from `AGENTS.md`. If its client supports automatic skill discovery, copy the skill directories into that client's expected skills folder (for example `.agents/skills/` or `.claude/skills/`, if supported). The folders in this kit are kept separate to avoid assuming one particular agent product.
