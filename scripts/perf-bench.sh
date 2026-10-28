#!/usr/bin/env bash
# Track P performance gate.
#
# Times the real binary end to end (process start + cold Open + one query), which
# is the unit an coding agent actually pays. Go benchmarks cover the in-process
# view; this covers the whole thing, including the part no in-process benchmark
# can see: process startup and full-pipeline allocations.
#
#   scripts/perf-bench.sh                 # fixture mode: measure + gate vs baseline
#   scripts/perf-bench.sh --runs 10       # more samples
#   scripts/perf-bench.sh --write-baseline# re-capture the baseline (after an intended win)
#   scripts/perf-bench.sh --repo /path/to/java/repo   # report-only on a real repo
#
# Modes
#   fixture (default)  deterministic synthetic workspace from internal/benchfixture.
#                      Comparable across machines, so this mode is GATED.
#   --repo             a real repository. Same machine-to-machine variance as your
#                      laptop, so this mode REPORTS and never gates.
#
# Gate: each metric must stay under baseline * tolerance. Tolerances are deliberately
# loose in CI (shared runners vary a lot) and tight locally:
#   CXJ_PERF_TOLERANCE   default 2.0 in CI (CI=true), 1.25 locally
# Baseline lives in scripts/perf-baseline.txt; commit it with the change that moves
# a number on purpose, so every perf change is a reviewed diff.
#
# What gets gated, and why wall is special:
#   cpu_ms    gated everywhere. It is total work summed over cores, so it barely moves
#             with core count: the same fixture at GOMAXPROCS=4 on the machine that owns
#             the baseline came in *under* a 1.25 tolerance on every case (less worker
#             sync overhead), which makes it the one metric a shared runner can judge.
#   wall_ms   gated locally, reported not gated in CI (CXJ_PERF_GATE_WALL=1 forces it).
#             Wall scales with core count and with whoever else is on the runner: the
#             same 4-core run was 1.4x on every wall number, which fails even x2.0 once
#             a runner adds its own contention and a cold page cache for a fresh binary.
#             Wall is what a user feels, so it stays gated where the baseline was
#             captured, i.e. on a developer machine.
#   rss       absolute ceiling, machine-independent: CXJ_PERF_RSS_CEILING_MB (default 250).
set -euo pipefail
export LC_ALL=C   # /usr/bin/time must print '.' decimals, not the locale's ','

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

repo=""
binary=""
runs=5
baseline="$root/scripts/perf-baseline.txt"
out_file="$root/dist/perf/results.txt"
write_baseline=0
explicit_gate=0
gate=1
gate_wall=""   # empty means "decide from the environment"
md_file=""

usage() {
  # Everything from line 2 down to the first non-comment line, so the header above is
  # the help text and cannot silently fall out of date with a hardcoded line range.
  awk 'NR>1 { if ($0 !~ /^#/) exit; sub(/^# ?/, ""); print }' "$0"
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) repo="${2:?--repo needs a path}"; shift 2 ;;
    --binary) binary="${2:?--binary needs a path}"; shift 2 ;;
    --runs) runs="${2:?--runs needs a number}"; shift 2 ;;
    --baseline) baseline="${2:?--baseline needs a path}"; shift 2 ;;
    --out) out_file="${2:?--out needs a path}"; shift 2 ;;
    --markdown) md_file="${2:?--markdown needs a path}"; shift 2 ;;
    --write-baseline) write_baseline=1; shift ;;
    --no-gate) gate=0; shift ;;
    --gate) gate=1; explicit_gate=1; shift ;;
    --gate-wall) gate_wall=1; shift ;;
    --no-gate-wall) gate_wall=0; shift ;;
    -h|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
done

# Real repos are report-only unless the caller insists on a gate.
if [[ -n "$repo" && $explicit_gate -eq 0 ]]; then
  gate=0
fi

if [[ -z "${CXJ_PERF_TOLERANCE:-}" ]]; then
  if [[ "${CI:-false}" == "true" ]]; then
    CXJ_PERF_TOLERANCE=2.0
  else
    CXJ_PERF_TOLERANCE=1.25
  fi
fi

# Wall is only comparable to a baseline captured on comparable hardware, so it is gated
# where it was captured (a developer machine) and merely reported on shared runners.
# See the header for the measurement behind that rule.
in_ci=0
if [[ "${CI:-false}" == "true" ]]; then
  in_ci=1
fi
if [[ -z "$gate_wall" ]]; then
  if [[ -n "${CXJ_PERF_GATE_WALL:-}" ]]; then
    gate_wall="$CXJ_PERF_GATE_WALL"
  elif [[ $in_ci -eq 1 ]]; then
    gate_wall=0
  else
    gate_wall=1
  fi
fi

# Toolchain: PATH first (CI), then the vendored SDK this repo develops with.
GO="${GO:-go}"
if ! command -v "$GO" >/dev/null 2>&1; then
  if [[ -x "$root/.tools/go/bin/go" ]]; then
    GO="$root/.tools/go/bin/go"
  else
    echo "go toolchain not found; set GO=/path/to/go" >&2
    exit 2
  fi
fi

# ---------------------------------------------------------------- workspace
cleanup_repo=0
if [[ -z "$repo" ]]; then
  repo="$(mktemp -d "${TMPDIR:-/tmp}/cxj-perf-fixture-XXXXXX")"
  cleanup_repo=1
  echo "generating benchmark fixture in $repo"
  "$GO" run ./internal/benchfixture/gen -out "$repo" -quiet >/dev/null
  symbol="${CXJ_BENCH_SYMBOL:-com.example.bench.mod0.Widget0Controller.get}"
  mode="fixture"
else
  [[ -d "$repo" ]] || { echo "not a directory: $repo" >&2; exit 2; }
  repo="$(cd "$repo" && pwd)"
  symbol="${CXJ_BENCH_SYMBOL:-}"
  mode="repo:$repo"
  if [[ ! -f "$repo/pom.xml" && ! -f "$repo/build.gradle" && ! -f "$repo/build.gradle.kts" ]]; then
    echo "note: $repo has no pom.xml/build.gradle; measured as a plain directory" >&2
  fi
fi

# ---------------------------------------------------------------- binary
if [[ -z "$binary" ]]; then
  binary="$root/dist/perf/cxj"
  mkdir -p "$(dirname "$binary")"
  echo "building $binary"
  "$GO" build -trimpath -ldflags="-s -w" -o "$binary" ./cmd/cxj
fi
[[ -x "$binary" ]] || { echo "not executable: $binary" >&2; exit 2; }
version="$("$binary" version 2>/dev/null | head -1 || echo unknown)"

# ---------------------------------------------------------------- measuring
# median wall, median cpu (user+sys), max RSS per case. Median not mean: one
# scheduler hiccup on a laptop should not fail a build.
results="$(mktemp)"
gate_log="$(mktemp)"
trap 'rm -rf "${results:-}" "${gate_log:-}"; [[ $cleanup_repo -eq 1 ]] && rm -rf "$repo" || true' EXIT

# say prints a gate line and records it, so --markdown can reproduce the verdict table
# without a second parsing pass. A tee to a temp file rather than a pipe: a piped block
# would run the gate loop in a subshell and lose the fail flag set inside it.
say() {
  printf '%s\n' "$*" | tee -a "$gate_log"
}

median_ms() { # stdin: seconds, one per line -> stdout: median in ms
  awk '{v[NR]=$1}
    END{
      for(i=2;i<=NR;i++){k=v[i];j=i-1;while(j>=1&&v[j]>k){v[j+1]=v[j];j--}v[j+1]=k}
      m=(NR%2)?v[(NR+1)/2]:(v[NR/2]+v[NR/2+1])/2
      printf "%.0f", m*1000 }'
}

measure() {
  local key="$1"; shift
  local walls=() cpus=() rss=0 line

  for _ in $(seq "$runs"); do
    # Run under time and keep only time's stderr.
    line="$( { /usr/bin/time -lp "$binary" "$@" >/dev/null; } 2>&1 >/dev/null )"
    local real user sys rssv
    real="$(awk '/^ *real /{print $2}' <<<"$line")"
    user="$(awk '/^ *user /{print $2}' <<<"$line")"
    sys="$(awk '/^ *sys /{print $2}' <<<"$line")"
    rssv="$(awk '/maximum resident set size/{print $1}' <<<"$line")"
    [[ -n "$real" && -n "$user" && -n "$sys" ]] || { echo "measure failed for $key" >&2; exit 1; }
    walls+=("$real")
    cpus+=("$(awk -v u="$user" -v s="$sys" 'BEGIN{printf "%.4f", u+s}')")
    if [[ -n "$rssv" ]] && awk -v a="$rssv" -v b="$rss" 'BEGIN{exit !(a>b)}'; then
      rss="$rssv"
    fi
  done

  local wall_ms cpu_ms rss_mb
  wall_ms="$(printf '%s\n' "${walls[@]}" | median_ms)"
  cpu_ms="$(printf '%s\n' "${cpus[@]}" | median_ms)"
  rss_mb="$(awk -v r="$rss" 'BEGIN{printf "%.0f", r/1048576}')"

  printf '%s.wall_ms=%s\n%s.cpu_ms=%s\n%s.rss_mb=%s\n' "$key" "$wall_ms" "$key" "$cpu_ms" "$key" "$rss_mb" >> "$results"
  printf '%-22s %6s ms wall  %6s ms cpu  %5s MB rss\n' "$key" "$wall_ms" "$cpu_ms" "$rss_mb"
}

java_count="$(find "$repo" -name '*.java' 2>/dev/null | wc -l | tr -d ' ')"
if [[ "${java_count:-0}" -lt 1 ]]; then
  echo "measured workspace has no .java files: $repo" >&2
  exit 1
fi
if [[ "$mode" == "fixture" && ( "${java_count:-0}" -lt 800 || "${java_count:-0}" -gt 900 ) ]]; then
  echo "fixture workspace looks wrong: ${java_count} java files (expected ~850)." >&2
  echo "Either the fixture changed size (re-capture the baseline) or the wrong" >&2
  echo "directory is being measured." >&2
  exit 1
fi

echo
echo "cxj perf bench  ($mode)  runs=$runs  tolerance=$CXJ_PERF_TOLERANCE"
echo "binary:    $binary ($version)"
echo "workspace: ${java_count} java files"
cpu_brand="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || grep -m1 'model name' /proc/cpuinfo 2>/dev/null | sed 's/.*: //' || echo unknown)"
echo "cores:     GOMAXPROCS=${GOMAXPROCS:-default}  nproc=$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo '?')  cpu=$cpu_brand"
echo

# Every case must point at the measured workspace: without --root the CLI scans
# the caller's cwd, which silently benchmarks the wrong repository.
r=(--root "$repo")

# startup is the no-op floor; anything else that looks like startup cost shows up here.
measure startup version
measure brief brief "${r[@]}"
measure find_main find Widget "${r[@]}" --scope main --limit 5
measure find_all find Widget "${r[@]}" --scope all --limit 5
measure endpoints endpoints "${r[@]}"
measure keys keys "${r[@]}"

if [[ -n "$symbol" ]]; then
  measure trail_main trail "$symbol" "${r[@]}"
  measure callees_main callees "$symbol" "${r[@]}"
  measure trail_all trail "$symbol" "${r[@]}" --scope all
  # show/outline are the sharpest sub-index trim: definitions and edges only, so they
  # pay for no Spring parse, no binding parse and no text lines. They need a symbol, so
  # they live in this block rather than with the unconditional cases.
  measure show_main show "$symbol" "${r[@]}"
  measure outline_main outline "$symbol" "${r[@]}"
else
  echo
  echo "note: no symbol cases measured. Set CXJ_BENCH_SYMBOL=com.acme.Thing.method" >&2
fi

mkdir -p "$(dirname "$out_file")"
{
  echo "# cxj perf results. mode=$mode version=$version runs=$runs"
  echo "# generated by scripts/perf-bench.sh"
  sort "$results"
} > "$out_file"
echo
echo "wrote $out_file"

# ---------------------------------------------------------------- baseline
if [[ $write_baseline -eq 1 ]]; then
  {
    echo "# cxj performance baseline (Track P)."
    echo "# Captured $(date -u +%Y-%m-%dT%H:%MZ) on ${cpu_brand}, $(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo '?') cores, GOMAXPROCS=${GOMAXPROCS:-default}."
    echo "# wall_ms numbers in this file belong to that machine: they are gated where they"
    echo "# were captured and only reported on shared CI runners (see scripts/perf-bench.sh)."
    echo "# Mode: deterministic fixture (${java_count} java files). Report-only repo runs are not baselined."
    echo "# Gate: measured <= baseline * tolerance, per metric. Tolerance: 1.25 locally, 2.0 in CI."
    echo "# cpu_ms and the RSS ceiling are gated everywhere; wall_ms is gated only where it was"
    echo "# captured (this machine) and reported-only on shared CI runners, which have different"
    echo "# core counts and contention. --gate-wall forces it. Re-capture with"
    echo "#   scripts/perf-bench.sh --write-baseline"
    echo "# and review it as part of the perf change that moved it."
    sort "$results"
  } > "$baseline"
  echo "wrote baseline $baseline"
  exit 0
fi

if [[ $gate -eq 0 ]]; then
  echo "gate: off (report-only mode)"
  exit 0
fi

if [[ ! -f "$baseline" ]]; then
  echo "gate: no baseline at $baseline; run scripts/perf-bench.sh --write-baseline" >&2
  exit 1
fi

echo
echo "gate: comparing against $baseline (x$CXJ_PERF_TOLERANCE)"
if [[ "$gate_wall" == "1" ]]; then
  say "gate: wall_ms GATED, cpu_ms GATED, rss ceiling GATED"
else
  say "gate: cpu_ms GATED, rss ceiling GATED; wall_ms REPORTED ONLY"
  say "      (wall depends on core count and runner contention; it is gated locally, on"
  say "       the machine that owns the baseline. --gate-wall forces it here.)"
fi
fail=0
while IFS='=' read -r key want; do
  case "$key" in ''|'#'*) continue ;; esac
  [[ "$key" == *.rss_mb ]] && continue   # RSS has its own absolute ceiling below
  have="$(awk -F= -v k="$key" '$1==k{print $2; found=1} END{exit !found}' "$results")" || {
    say "  MISSING $key (new case needs a baseline line)"; fail=1; continue; }
  verdict="$(awk -v h="$have" -v w="$want" -v t="$CXJ_PERF_TOLERANCE" 'BEGIN{
    if (w+0 <= 0) print "skip"
    else print (h+0 <= w*t+0.5) ? "ok" : "FAIL" }')"
  if [[ "$key" == *.wall_ms && "$gate_wall" != "1" && "$verdict" == "FAIL" ]]; then
    # Reported, not swallowed: the number still deserves to be seen on CI.
    say "  wall $(printf '%-22s' "$key") ${have}ms (baseline ${want}ms," \
      "x$(awk -v h="$have" -v w="$want" 'BEGIN{printf "%.1f", (w>0)?h/w:0}')) not gated"
    continue
  fi
  if [[ "$verdict" == "skip" ]]; then
    say "  skip $(printf '%-22s' "$key") baseline is 0"
  elif [[ "$verdict" == "FAIL" ]]; then
    say "  FAIL $key: ${have}ms > baseline ${want}ms x $CXJ_PERF_TOLERANCE"
    fail=1
  else
    say "  ok   $(printf '%-22s' "$key") ${have}ms (baseline ${want}ms)"
  fi
done < <(grep -vE '^[[:space:]]*(#|$)' "$baseline")

# Absolute RSS ceiling from the plan's budget, independent of machine speed.
rss_max="$(awk -F= '/\.rss_mb=/{if($2+0>m) m=$2} END{printf "%.0f", m+0}' "$results")"
rss_ceiling="${CXJ_PERF_RSS_CEILING_MB:-250}"
if [[ "${rss_max:-0}" -gt "$rss_ceiling" ]]; then
  say "  FAIL peak rss: ${rss_max}MB > ${rss_ceiling}MB ceiling"
  fail=1
else
  say "  ok   $(printf '%-22s' peak_rss) ${rss_max}MB (ceiling ${rss_ceiling}MB)"
fi

# ------------------------------------------------------------------ markdown
# CI publishes this as a commit comment. Job logs and artifacts both need a token to
# read, which means the numbers from a failing gate are invisible to anyone who has not
# been granted the repository; a comment on the commit is readable by the public API
# and makes "the perf job is red" an actionable statement.
if [[ -n "$md_file" ]]; then
  {
    if [[ $fail -eq 0 ]]; then
      echo "### cxj perf gate: green"
    else
      echo "### cxj perf gate: FAILED"
    fi
    echo
    echo "- mode \`$mode\` (fixture · ${java_count} java files) · runs=$runs · tolerance x$CXJ_PERF_TOLERANCE"
    echo "- cpu \`$cpu_brand\` · nproc $(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo '?') · GOMAXPROCS=${GOMAXPROCS:-default}"
    if [[ "$gate_wall" == "1" ]]; then
      echo "- \`wall_ms\` **gated**"
    else
      echo "- \`wall_ms\` reported only (gated on developer machines, where the baseline lives)"
    fi
    echo
    echo "| case | wall ms | cpu ms | rss MB |"
    echo "|---|---:|---:|---:|"
    awk -F= '
      /^#/ || NF != 2 { next }
      { dot = index($1, "."); c = substr($1, 1, dot - 1)
        cases[c SUBSEP substr($1, dot + 1)] = $2
        if (!(c in seen)) { seen[c] = 1; names[n++] = c } }
      END {
        for (i = 0; i < n; i++) for (j = i + 1; j < n; j++)
          if (names[j] < names[i]) { t = names[i]; names[i] = names[j]; names[j] = t }
        for (i = 0; i < n; i++) {
          c = names[i]
          printf "| `%s` | %s | %s | %s |\n", c,
            ((c SUBSEP "wall_ms") in cases) ? cases[c SUBSEP "wall_ms"] : "-",
            ((c SUBSEP "cpu_ms") in cases) ? cases[c SUBSEP "cpu_ms"] : "-",
            ((c SUBSEP "rss_mb") in cases) ? cases[c SUBSEP "rss_mb"] : "-"
        }
      }' "$results"
    echo
    echo '```'
    cat "$gate_log"
    echo '```'
  } > "$md_file"
  echo "wrote $md_file"
fi

if [[ $fail -ne 0 ]]; then
  echo
  echo "PERF REGRESSION (see FAIL lines above). If the change is intended," >&2
  echo "re-capture: scripts/perf-bench.sh --write-baseline" >&2
  exit 1
fi
echo
echo "perf gate green"
