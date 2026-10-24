#!/usr/bin/env bash
# Measure Phase 7 benchmark figures and write reports/benchmark.json.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
mkdir -p bin reports
go build -o bin/cx ./cmd/cx
export CX_BIN="$root/bin/cx"
export CX_EMBED_URL="${CX_EMBED_URL:-http://127.0.0.1:11434}"
export CX_EMBED_MODEL="${CX_EMBED_MODEL:-qwen3-embedding:0.6b}"

if [[ -z "${PETCLINIC_ROOT:-}" || -z "${PETCLINIC_COMMIT:-}" ]]; then
  if [[ -f reports/petclinic.env ]]; then
    # shellcheck disable=SC1091
    source reports/petclinic.env
  else
    eval "$(./scripts/prepare-petclinic.sh | grep '^PETCLINIC_')"
  fi
fi
printf 'PETCLINIC_ROOT=%s\nPETCLINIC_COMMIT=%s\n' "$PETCLINIC_ROOT" "$PETCLINIC_COMMIT" > reports/petclinic.env

go run ./cmd/bench -petclinic "$PETCLINIC_ROOT" -commit "$PETCLINIC_COMMIT" \
  -embed-url "$CX_EMBED_URL" -embed-model "$CX_EMBED_MODEL" \
  -out reports/benchmark.json -notes reports/benchmark-notes.md
echo "Wrote reports/benchmark.json"
