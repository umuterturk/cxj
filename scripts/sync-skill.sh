#!/bin/sh
# Sync the canonical agent skill to its distribution mirrors.
#
# internal/skill/SKILL.md is canonical — it is what `cxj install` writes and
# what the binary embeds. Two mirrors exist purely for distribution formats:
#
#   plugins/cxj/skills/cxj/SKILL.md   the Claude Code plugin (marketplace)
#   skills/cxj/SKILL.md               the skills.sh layout (`npx skills add umuterturk/cxj`)
#
# `--check` verifies every mirror is byte-identical and the plugin manifest's
# version matches the newest CHANGELOG release, without writing anything
# (CI mode). Without it, mirrors are overwritten from the canonical copy.
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

canonical="internal/skill/SKILL.md"
mirrors="plugins/cxj/skills/cxj/SKILL.md skills/cxj/SKILL.md"
check=0
if [ "${1:-}" = "--check" ]; then
  check=1
elif [ $# -gt 0 ]; then
  echo "usage: $0 [--check]" >&2
  exit 2
fi

if [ ! -f "$canonical" ]; then
  echo "canonical skill missing: $canonical" >&2
  exit 1
fi

status=0
for m in $mirrors; do
  if [ "$check" -eq 1 ]; then
    if ! cmp -s "$canonical" "$m" 2>/dev/null; then
      echo "drift: $m differs from $canonical (run scripts/sync-skill.sh)" >&2
      status=1
    fi
  else
    mkdir -p "$(dirname "$m")"
    cp "$canonical" "$m"
    echo "synced $m"
  fi
done

# The plugin manifest carries a pinned version (users stay on it until it
# changes), so it must track the newest released CHANGELOG entry — a plugin
# announcing an old version is an old plugin in every client's cache.
released="$(sed -n 's/^## \[\([0-9][^]]*\)\].*/\1/p' CHANGELOG.md | head -1)"
manifest_version="$(sed -n 's/.*"version": "\([^"]*\)".*/\1/p' plugins/cxj/.claude-plugin/plugin.json | head -1)"
marketplace_version="$(sed -n 's/.*"version": "\([^"]*\)".*/\1/p' .claude-plugin/marketplace.json | head -1)"
if [ -z "$released" ]; then
  echo "cannot read newest release from CHANGELOG.md" >&2
  exit 1
fi
for pair in "plugins/cxj/.claude-plugin/plugin.json=$manifest_version" ".claude-plugin/marketplace.json=$marketplace_version"; do
  if [ "$check" -eq 1 ] && [ "${pair#*=}" != "$released" ]; then
    echo "drift: ${pair%%=*} version ${pair#*=} != newest release $released" >&2
    status=1
  fi
done

if [ "$check" -eq 1 ]; then
  # JSON shape is checked cheaply here; the authoritative `claude plugin
  # validate` is a release-day step (it needs the Claude Code CLI).
  python3 -c 'import json,sys; [json.load(open(p)) for p in sys.argv[1:]]' \
    .claude-plugin/marketplace.json plugins/cxj/.claude-plugin/plugin.json
fi
exit "$status"
