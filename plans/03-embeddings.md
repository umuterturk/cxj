**CURRENT STATE: HISTORICAL — embeddings removed from the product.** See `STATE.md` for actual progress.

# Phase 3 — Embeddings (removed)

This phase originally required local Ollama embeddings and hybrid search. The product is now a one-shot lexical CLI with no model dependency. `./scripts/acceptance.sh 3` still runs gates 0..2 (`TestP[0-3]` matches P0–P2 only). Do not reintroduce embeddings.
