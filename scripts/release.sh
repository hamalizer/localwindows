#!/usr/bin/env bash
set -euo pipefail

# Build installable binaries and create a GitHub release.
# Requires: go, gh (GitHub CLI), and cross-compilation toolchains for
# non-native platforms.

APP_NAME="localwindows"
VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
BUILD_DIR="build"
LDFLAGS="-s -w -X main.version=${VERSION}"
BUILD_FLAGS="-trimpath -ldflags \"${LDFLAGS}\""

echo "=== Building ${APP_NAME} ${VERSION} ==="

rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# --- Build matrix ---
# Each entry: GOOS GOARCH CC suffix
TARGETS=(
  "linux   amd64  gcc                       -linux-amd64"
  "linux   arm64  aarch64-linux-gnu-gcc     -linux-arm64"
  "darwin  amd64  o64-clang                 -darwin-amd64"
  "darwin  arm64  oa64-clang                -darwin-arm64"
  "windows amd64  x86_64-w64-mingw32-gcc   -windows-amd64.exe"
)

built=()
for target in "${TARGETS[@]}"; do
  read -r os arch cc suffix <<< "$target"
  out="${BUILD_DIR}/${APP_NAME}${suffix}"
  echo "  Building ${os}/${arch}..."

  # Try the build; skip if cross-compiler is missing.
  if CGO_ENABLED=1 GOOS="$os" GOARCH="$arch" CC="$cc" \
     go build -trimpath -ldflags "${LDFLAGS}" -o "$out" . 2>/dev/null; then
    echo "    -> ${out} ($(du -h "$out" | cut -f1))"
    built+=("$out")
  else
    echo "    -> SKIPPED (cross-compiler '${cc}' not available)"
  fi
done

if [ ${#built[@]} -eq 0 ]; then
  echo "ERROR: No binaries were built."
  exit 1
fi

# --- Checksums ---
echo ""
echo "=== Generating checksums ==="
checksum_file="${BUILD_DIR}/checksums-sha256.txt"
(cd "${BUILD_DIR}" && sha256sum * > checksums-sha256.txt)
cat "${checksum_file}"

# --- Release ---
echo ""
echo "=== Creating GitHub release ${VERSION} ==="

if ! command -v gh &>/dev/null; then
  echo "WARNING: gh CLI not found. Skipping GitHub release creation."
  echo "To create the release manually, run:"
  echo "  gh release create ${VERSION} --prerelease --title '${VERSION} — Canary Dev Release' ${built[*]} ${checksum_file}"
  exit 0
fi

RELEASE_NOTES="## ${APP_NAME} ${VERSION} — Canary Dev Release

Lightweight LAN remote desktop application.

### Included Binaries
$(for b in "${built[@]}"; do echo "- \`$(basename "$b")\`"; done)

### Checksums (SHA-256)
\`\`\`
$(cat "${checksum_file}")
\`\`\`

### Features
- Screen sharing with tile-based delta compression (JPEG)
- Full mouse + keyboard remote input
- Password (SHA-256 challenge-response) or one-time PIN authentication
- Automatic LAN host discovery via UDP broadcast
- File transfer (viewer to host)
- Bidirectional clipboard sync
- Special key combos (Ctrl+Alt+Del, Alt+Tab, Win+D, etc.)
- TLS encrypted connections
"

gh release create "${VERSION}" \
  --target "$(git rev-parse HEAD)" \
  --prerelease \
  --title "${VERSION} — Canary Dev Release" \
  --notes "${RELEASE_NOTES}" \
  "${built[@]}" "${checksum_file}"

echo ""
echo "=== Done! Release ${VERSION} published. ==="
