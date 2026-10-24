---
name: java-index
description: Extract Java symbols and candidate relations using Tree-sitter into parser-neutral IR and build fast RAM-only symbol and reference indexes.
---
# Java and IR

Use Tree-sitter Java grammar behind a `JavaExtractor`, output neutral `FileFacts`. Define stable IDs, symbol kind, owner/signature/location and certainty. Store a compact in-RAM lookup; don't retain parse trees if not needed. Split Java-specific extraction from search. No compiler claims without compiler evidence. Exact symbol/ref searches must be correct before optimizing. No Spring annotations in the generic index unless emitted as generic metadata and enriched separately.
