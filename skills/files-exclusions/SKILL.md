---
name: files-exclusions
description: Index ordinary UTF-8 text as searchable lines, apply folder exclusions before every indexing path, and protect against binary files and symlink traversal.
---
# Plain text and exclusions

Use one canonical path matcher shared by initial file walking, incremental file watching, Java AST, text search, config index and embedding queue. Recognize `.cxignore`, `--exclude` and built-in directories. Ordinary text should be lexical only with source line and bounded snippet; don't create fake Java symbols or semantic vectors for docs. Skip binary, invalid UTF-8 and oversized files with visible counters. Delete excluded content from any existing indexes immediately when exclusion changes / restart (document live option support accurately).
