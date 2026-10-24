# Benchmark notes

- Generated: 2026-10-25T02:02:43+03:00
- Hardware: darwin/arm64 gogo1.24.0
- PetClinic commit: 500158f732419217507c7656904b8e6aa1bcc0d6
- Go index heap (one-shot status): 4269616 bytes
- One-shot lexical OwnerController: p50=128.00ms p95=140.00ms (n=10)
- Each sample is a full index + find (no daemon).
- Numbers are measured from a live local binary; not fabricated.
