cxj (j = Java) — macos-arm64

Usage:
  ./cxj install       # project: .agents/skills + .claude/skills symlink
  ./cxj install -g    # global: ~/.agents/skills + symlink; /usr/local/bin/cxj
  ./cxj brief --root /path/to/workspace
  ./cxj ask 'who calls OwnerController' --root /path/to/workspace

ask is optional. If Needle 3 files are missing, cxj ask prompts to download them.
