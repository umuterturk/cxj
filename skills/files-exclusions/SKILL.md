---
name: files-exclusions
description: Index ordinary UTF-8 text as searchable lines, apply folder exclusions before every indexing path, and protect against binary files and symlink traversal.
---
# Plain text and exclusions

Use one canonical path matcher shared by file walking, Java AST, text search, and config index. Recognize `.cxignore`, `--exclude` and built-in directories. Ordinary text should be lexical only with source line and bounded snippet; don't create fake Java symbols for docs. Skip binary, invalid UTF-8 and oversized files with visible counters. Exclusions apply on every CLI invocation (one-shot scan).
