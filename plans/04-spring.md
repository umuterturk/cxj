**CURRENT STATE: PLAN NOT EXECUTED.** This is an entry specification, not evidence of a passing phase. See `STATE.md` for actual progress.

# Phase 4 — Spring-aware static relationships

> **CURRENT STATE / ENTRY CONDITION:** This phase is NOT COMPLETE in this distributed kit. Begin only after all gates 0..3 have actually passed and `STATE.md` confirms that. It is NOT a claim that earlier phases have run.
> **NEXT STATE IF PASS:** Mark phase 4 PASSED in `STATE.md`, record command/result, compact context, then read only the next phase.

**Goal:** Source-only insight into a Spring Boot Java application, without running the application.

**Implement:** Recognize controller/class/method request mappings including path composition and HTTP methods, stereotype/component beans, constructor injection, `@Bean`, `@Qualifier`/`@Primary` when unambiguous, `@Transactional` annotations (not effective runtime behavior), and Spring Data repository interfaces. Add Spring nodes/edges as generic IR relationships + provenance. Limit confidence claims: multiple bean candidates => `ambiguous` or `candidate`, NOT exact. Reuse symbols from Phase 1 and semantic chunks from Phase 3.

**Gate:** `TestP4*`: correct GET/POST paths, inherited mapping limitations disclosed, unique vs ambiguous injection, transaction declared vs effective, repository discovery. `./scripts/acceptance.sh 4`.

**Non-goals:** Full Spring context boot, proxies/AOP runtime fidelity, dynamic bean selection, classpath evaluation, compiled bytecode scanning.
