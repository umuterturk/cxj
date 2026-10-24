---
name: spring-config
description: Enrich Java IR with Spring routes, beans, dependency injection, transactions, repository and application configuration relationships while retaining uncertainty.
---
# Spring and configuration

Read `../../plans/04-spring.md` and `../../plans/05-config.md` only when those phases become active. Detect annotations, source-defined route composition and bean candidates; do not assert dynamic runtime behavior. Parse Spring YAML/properties with source locations, profile precedence for explicitly supported local sources, placeholders and `@Value`/`@ConfigurationProperties` bindings. Redact secrets on every API/embedding/snippet. Never start a Spring app to make analysis look correct. Unsupported config precedence must surface `unknown` or `ambiguous`.
