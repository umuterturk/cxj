# Benchmark notes

- Generated: 2026-10-25T01:36:02+03:00
- Hardware: darwin/arm64 gogo1.24.0
- PetClinic commit: 500158f732419217507c7656904b8e6aa1bcc0d6
- Model: qwen3-embedding:0.6b via http://127.0.0.1:11434
- Go index heap (PetClinic, semantic off): 5281752 bytes
- Warm lexical OwnerController: p50=3.00ms p95=3.00ms (n=30)
- Semantic recall@10 on held-out probes: 1.00 (5/5)
- Embedding index wall time (small corpus): 0.72s
- Numbers are measured from a live local binary + Ollama; not fabricated.
