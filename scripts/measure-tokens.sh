#!/bin/sh
# Replay the cxj half of the token measurement in docs/MEASURING.md.
#
# This is the deterministic part: it runs the calls that answer the twelve
# questions and prints the size of each answer (lines, bytes). It does not
# need a model, a key, or network. The token numbers in docs/MEASURING.md
# come from the harness's per-session accounting; this script shows the
# output surface those tokens were spent on.
#
#   ./scripts/measure-tokens.sh [--repo /path/to/java/repo]
#
# The experiment's cxj arm passed --json on several calls, so the footprint
# it actually paid was larger than the compact forms shown here.
set -eu

repo="."
case "${1:-}" in
  --repo)
    [ $# -ge 2 ] || { echo "usage: $0 [--repo PATH]" >&2; exit 2; }
    repo="$2"
    ;;
  "")
    ;;
  *)
    echo "usage: $0 [--repo PATH]" >&2
    exit 2
    ;;
esac

cxj="${CJX:-cxj}"
command -v "$cxj" >/dev/null 2>&1 || {
  echo "cxj not found on PATH (set CJX=/path/to/cxj)" >&2
  exit 1
}
[ -d "$repo" ] || { echo "no such directory: $repo" >&2; exit 1; }
repo="$(cd "$repo" && pwd)"

# Symbols are overridable so the script can run on another repo.
ctrl="${CTRL:-com.trendyol.paymentapi.infrastructure.adapter.inbound.rest.PaymentController}"
consumer="${CONSUMER:-TransactionReconciledConsumer}"
checker="${CHECKER:-PaymentIdempotencyChecker}"

tmp="$(mktemp "${TMPDIR:-/tmp}/cxj-measure.XXXXXX")"
trap 'rm -f "$tmp"' EXIT INT TERM

total_bytes=0
total_lines=0
n=0

# run LABEL COMMAND... — execute, record size, never abort on a cxj error.
run() {
  label="$1"
  shift
  n=$((n + 1))
  if ( cd "$repo" && "$cxj" "$@" ) >"$tmp" 2>&1; then
    status="ok"
  else
    status="empty/error"
  fi
  bytes=$(wc -c <"$tmp" | tr -d ' ')
  lines=$(wc -l <"$tmp" | tr -d ' ')
  total_bytes=$((total_bytes + bytes))
  total_lines=$((total_lines + lines))
  printf '%2d. %-46s %6s lines %8s bytes  (%s)\n' "$n" "$label" "$lines" "$bytes" "$status"
}

echo "repo: $repo"
echo "cxj:  $("$cxj" version 2>/dev/null || echo unknown)"
echo

# Round 1 — structural questions.
run "brief (endpoints/out_http/out_db/profiles)" brief
run "endpoints (every route)" endpoints
run "deps spring-kafka (resolved version + source)" deps spring-kafka
run "show PaymentController (injected fields)" show "$ctrl"

# Round 2 — one-line lookups.
run "find Job (scheduled jobs)" find Job
run "find Consumer (Kafka consumers)" find Consumer
run "find PaymentIdempotencyChecker (implementation)" find "$checker"
run "endpoints --method GET (route lookup)" endpoints --method GET
run "find Bus (type provenance)" find Bus
run "status (file/type counts)" status
run "trail TransactionReconciledConsumer" trail "$consumer"
run "keys (config names)" keys

echo
echo "total: $total_lines lines, $total_bytes bytes across $n calls"
echo "Token accounting and the whole experiment: docs/MEASURING.md"
