#!/usr/bin/env bash
# Build cxj. Needle 3 is optional and is copied only if already present.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

os="$(uname -s)"
arch="$(uname -m)"
case "$os-$arch" in
  Darwin-arm64) platform="macos-arm64"; lib="libneedle.dylib" ;;
  Darwin-x86_64) platform="macos-x86_64"; lib="libneedle.dylib" ;;
  Linux-x86_64) platform="linux-x86_64"; lib="libneedle.so" ;;
  Linux-aarch64|Linux-arm64) platform="linux-arm64"; lib="libneedle.so" ;;
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
  ldflags="$ldflags -X example.com/cx/internal/version.Version=$ver"
fi
echo "building cxj -> $out/cxj ($ver)"
go build -trimpath -ldflags="$ldflags" -o "$out/cxj" ./cmd/cxj
chmod +x "$out/cxj"

weights="needle3.cact"
if [[ -f "$root/needle/$lib" ]]; then
  cp "$root/needle/$lib" "$out/$lib"
fi
if [[ -f "$root/needle/$weights" ]]; then
  cp "$root/needle/$weights" "$out/$weights"
fi

"$out/cxj" install -g || true

cat > "$out/README.txt" <<EOF
cxj (j = Java)

Usage:
  ./cxj install       # project: .agents/skills + .claude/skills symlink
  ./cxj install -g    # global: ~/.agents/skills + symlink; /usr/local/bin/cxj
  ./cxj brief --root /path/to/workspace
  ./cxj ask 'who calls OwnerController' --root /path/to/workspace

ask is optional. If Needle 3 files are missing, cxj ask prompts to download them.
EOF

echo "Wrote $out"
ls -lh "$out"
