#!/bin/sh
# The demo take: petclinic, three commands, paced for screen recording.
#
# It prints each command the way a person would type it, then runs it, pausing
# long enough to read on video and clearing between beats so every answer is
# seen whole. Nothing is staged: the repo is a real clone, the output is cxj's.
#
#   ./scripts/demo.sh [--repo PATH] [--fast]
#
# --repo   an existing spring-petclinic checkout (default: ./spring-petclinic)
# --fast   shorter pauses, for a dry run
set -eu

repo="./spring-petclinic"
pause_cmd=0.7
pause_read=1.4

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)
      [ $# -ge 2 ] || { echo "usage: $0 [--repo PATH] [--fast]" >&2; exit 2; }
      repo="$2"; shift 2 ;;
    --fast)
      pause_cmd=0.1; pause_read=0.15; shift ;;
    *)
      echo "usage: $0 [--repo PATH] [--fast]" >&2; exit 2 ;;
  esac
done

cxj="${CJX:-cxj}"
command -v "$cxj" >/dev/null 2>&1 || { echo "cxj not found on PATH (set CJX=...)" >&2; exit 1; }
[ -d "$repo/src/main/java" ] || {
  echo "not a Spring checkout: $repo" >&2
  echo "clone it first:" >&2
  echo "  git clone --depth 1 https://github.com/spring-projects/spring-petclinic" >&2
  exit 1
}
repo="$(cd "$repo" && pwd)"

clear_screen() { printf '\033[2J\033[H'; }

# A bare prompt on a cleared screen is what the recording should start from.
clear_screen
printf '$ cd %s\n' "$repo"
cd "$repo"
sleep "$pause_cmd"

run() {
  clear_screen
  printf '$ %s\n' "$*"
  sleep "$pause_cmd"
  "$@"
  sleep "$pause_read"
}

# 1. The orientation: kind, versions, size, profiles, every route, DB entities.
run "$cxj" brief

# 2. One hop from an HTTP handler: the route, what it calls, the test that covers it.
#    --scope all includes test sources, which is where the tests row comes from.
run "$cxj" trail org.springframework.samples.petclinic.owner.OwnerController.processCreationForm --scope all

# 3. Declared-type resolution: the repository method and every use, tests included.
run "$cxj" callers org.springframework.samples.petclinic.owner.OwnerRepository.findByLastName --scope all
