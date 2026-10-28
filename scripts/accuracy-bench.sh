#!/usr/bin/env bash
# Track A accuracy benchmark — the §4.4 exit criteria, run against a real repo.
#
# REPORT-ONLY, like perf-bench.sh's --repo mode: real repositories drift, so this
# is a regression tripwire for humans, not a CI gate. The law itself lives in the
# deterministic collision-shaped tests (trail_golden, typedcall, exclude_parity,
# match_parity), which run in `go test ./...` everywhere.
#
#   scripts/accuracy-bench.sh --repo /path/to/java/repo \
#       [--sym  com.acme.Foo.bar]   # method nobody calls from code (e.g. an HTTP endpoint)
#       [--sym2 com.acme.Foo.baz]   # method to check callees on (default: --sym)
#       [--term Controller]         # capitalized name-suffix every repo has
#
# Exit code: number of failed checks (0 = everything the tool claims is true).
set -uo pipefail

repo="" sym="" sym2="" term="Controller"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) repo="$2"; shift 2 ;;
    --sym) sym="$2"; shift 2 ;;
    --sym2) sym2="$2"; shift 2 ;;
    --term) term="$2"; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$repo" ]] || { echo "usage: accuracy-bench.sh --repo PATH [options]" >&2; exit 2; }
cxj="${CXJ_BIN:-./bin/cxj}"; [[ -x "$cxj" ]] || cxj="$(dirname "$0")/../bin/cxj"
[[ -x "$cxj" ]] || { echo "cxj binary not found (build it or set CXJ_BIN)" >&2; exit 2; }
cd "$repo" || exit 2

fails=0
check() { # name expected actual
  if [[ "$2" == "$3" ]]; then printf '  ok   %-46s %s\n' "$1" "$3"
  else printf '  FAIL %-46s %s (expected %s)\n' "$1" "$3" "$2"; fails=$((fails+1)); fi
}
check_report() { # name already-verdict (the verdict carries its own measurement)
  case "$2" in yes*) printf '  ok   %-46s %s\n' "$1" "$2" ;;
                    *) printf '  FAIL %-46s %s\n' "$1" "$2"; fails=$((fails+1)) ;;
  esac
}

# --sym defaults to the first HTTP endpoint: a method real code never calls.
if [[ -z "$sym" ]]; then
  sym=$("$cxj" endpoints --json 2>/dev/null | python3 -c '
import json,sys
try: eps=json.load(sys.stdin).get("results") or []
except Exception: eps=[]
print(eps[0]["symbol"] if eps else "")' 2>/dev/null)
  [[ -n "$sym" ]] || { echo "no endpoints found; pass --sym TYPE.MEMBER" >&2; exit 2; }
  echo "using endpoint symbol: $sym"
fi

# 1. find <TERM>: first page is all classes, all main sources (A6/A7/A4 rows).
read -r classes main_first <<<"$("$cxj" find "$term" --limit 5 --json 2>/dev/null | python3 -c '
import json,sys
try: r=json.load(sys.stdin)["results"]
except Exception: r=[]
cls=sum(1 for x in r if x["kind"]=="class")
main=1 if r and all(not x["path"].startswith("src/test") for x in r) else 0
print(cls, main)')"
check "find $term: classes on page 1" "5" "$classes"
check "find $term: page 1 has no test rows" "1" "$main_first"

# 2. trail <SYM>: every http row belongs to the queried symbol's file (A1/A2
# row: routes used to match the package name, so a trail showed other classes'
# routes). Checking file identity, not owner prefix: nested types live in the
# same file and legitimately share it.
own_file=$("$cxj" show --json "$sym" 2>/dev/null | python3 -c '
import json,sys
try: print(json.load(sys.stdin).get("path",""))
except Exception: print("")' 2>/dev/null)
wrong_http=$("$cxj" trail --json "$sym" 2>/dev/null | python3 -c "
import json,sys
own='''$own_file'''
try: h=json.load(sys.stdin).get('http') or []
except Exception: h=[]
print(sum(1 for x in h if own and x['path']!=own))" 2>/dev/null)
check "trail $sym: http rows from other files" "0" "$wrong_http"

# 3. callers <SYM>: a symbol nobody calls from code says nearly so (A3 row).
# Name-only candidates with untypeable receivers legitimately remain; a flood
# of them means the receiver typing regressed. Ceiling is generous on purpose.
cands=$("$cxj" callers --scope all --json "$sym" 2>/dev/null | python3 -c '
import json,sys
try: print(len(json.load(sys.stdin).get("results") or []))
except Exception: print(-1)')
if [[ "$cands" -ge 0 && "$cands" -le 20 ]]; then got="yes ($cands rows)"; else got="NO ($cands rows)"; fails=$((fails+1)); fi
check_report "callers $sym: candidate ceiling (<=20)" "$got"

# 4. callees <SYM2>: results stay inside one file (A2/A7 row: `handle` matched
# repo-wide produced 292 rows across 23 files).
sym2="${sym2:-$sym}"
nfiles=$("$cxj" callees --json "$sym2" 2>/dev/null | python3 -c '
import json,sys
try: r=json.load(sys.stdin).get("results") or []
except Exception: r=[]
print(len({x["path"] for x in r}))')
check "callees $sym2: results in one file" "1" "$nfiles"

echo
if [[ "$fails" -eq 0 ]]; then echo "accuracy: all checks passed"; else echo "accuracy: $fails check(s) failed"; fi
exit "$fails"
