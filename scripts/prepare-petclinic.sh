#!/usr/bin/env bash
set -euo pipefail
mkdir -p third_party
if [[ ! -d third_party/spring-petclinic/.git ]]; then
  git clone https://github.com/spring-projects/spring-petclinic.git third_party/spring-petclinic
fi
cd third_party/spring-petclinic
commit="$(git rev-parse HEAD)"
printf 'PETCLINIC_ROOT=%s\nPETCLINIC_COMMIT=%s\n' "$(pwd)" "$commit"
echo 'Record the commit in STATE.md; never silently advance it during benchmark runs.'
