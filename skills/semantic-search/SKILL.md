---
name: semantic-search
description: Implement real local multilingual code embeddings in RAM and hybrid retrieval for arbitrary unseen concepts; never use manual synonym lists.
---
# Semantic retrieval

Use the configured local Ollama embedding endpoint, default `qwen3-embedding:0.6b`. Index Java class/method chunks assembled from actual source, not fixture phrases. Keep all generated vectors in RAM. Normalize vectors and use cosine/dot with simple brute force before complex ANN. Hybrid merges with deterministic lexical rank and exact-hit precedence. No on-disk cache, cloud model, LLM summaries or fixed-language replacement table. Unavailable model is explicit 503. Test multilingual + semantic queries over held-out examples, with aggregate thresholds.
