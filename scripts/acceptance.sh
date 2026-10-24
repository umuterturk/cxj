#!/usr/bin/env bash
set -euo pipefail
phase="${1:?Usage: ./scripts/acceptance.sh <phase-number 0..7>}"
[[ "$phase" =~ ^[0-7]$ ]] || { echo "Phase must be 0..7" >&2; exit 2; }
mkdir -p bin
# Set CX_RACE=1 for an instrumented engine AND acceptance process.
export CX_BIN="$(pwd)/bin/cx"
if [[ -f reports/petclinic.env ]]; then
  # shellcheck disable=SC1091
  source reports/petclinic.env
  export PETCLINIC_ROOT PETCLINIC_COMMIT
fi
regex="^TestP[0-${phase}]"
if [[ "${CX_RACE:-0}" == "1" ]]; then
  go build -race -o bin/cx ./cmd/cx
  go test -race -count=1 -timeout=20m -v ./acceptance -run "$regex"
else
  go build -o bin/cx ./cmd/cx
  go test -count=1 -timeout=20m -v ./acceptance -run "$regex"
fi
