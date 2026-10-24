**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 5 — Spring Boot configuration intelligence

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..4 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 5 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Index YAML, properties, profile-specific files and connect config keys to Java access points. Continue ordinary text search for all other file types.

**Implement:** YAML documents + Java `.properties` parser with exact locations and repeated definitions. Parse `application.yml`, `application.yaml`, `application-{profile}.*`, nested keys; support controlled local imports. Track origin and precedence for supported repository sources, but never claim to know external environment variables / command line secrets. Link `@Value`, `@ConfigurationProperties`, `@ConditionalOnProperty` and `@Profile` to config graph; show unresolved/ambiguous when appropriate. Redact secret-like keys and values in all exposed results, logs, embeddings and snippets. Validate relaxed binding carefully; don't globally collapse arbitrary keys.

**Gate:** `TestP5*`: nested YAML/property values, profile overrides, placeholders marked external, Java binding links, secrets not leaked, other text still searchable, excluded config absent. `./scripts/acceptance.sh 5`.

**Reference:** Spring externalized configuration order is complex. Explicitly support tested local cases and report unknown in unsupported cases; no general claim of runtime equivalence.
