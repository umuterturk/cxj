#!/usr/bin/env bash
# Build cxj.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

os="$(uname -s)"
arch="$(uname -m)"
case "$os-$arch" in
  Darwin-arm64) platform="macos-arm64" ;;
  Darwin-x86_64) platform="macos-x86_64" ;;
  Linux-x86_64) platform="linux-x86_64" ;;
  Linux-aarch64|Linux-arm64) platform="linux-arm64" ;;
  *)
    echo "unsupported platform: $os $arch" >&2
    exit 2
    ;;
esac

out="${1:-$root/dist/$platform}"
mkdir -p "$out"

ver="$(git describe --tags --always --dirty 2>/dev/null || true)"
ldflags="-s -w"
if [[ -n "$ver" ]]; then
  ldflags="$ldflags -X github.com/umuterturk/cxj/internal/version.Version=$ver"
fi
echo "building cxj -> $out/cxj ($ver)"
go build -trimpath -ldflags="$ldflags" -o "$out/cxj" ./cmd/cxj
chmod +x "$out/cxj"

"$out/cxj" install -g || true

cat > "$out/README.txt" <<EOF
cxj (j = Java)

Usage:
  ./cxj install       # project: .agents/skills + .claude/skills symlink
  ./cxj install -g    # global: ~/.agents/skills + symlink; /usr/local/bin/cxj
  ./cxj brief --root /path/to/workspace

EOF

echo "Wrote $out"
ls -lh "$out"
