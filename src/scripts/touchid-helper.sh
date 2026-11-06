#!/usr/bin/env bash
#
# Builds authama-touchid, the helper that asks macOS for Touch ID on authama's
# behalf (authama is built without cgo and cannot call LocalAuthentication):
#
#   scripts/touchid-helper.sh <out>              this Mac's architecture
#   scripts/touchid-helper.sh <out> universal    arm64 + x86_64, joined by lipo
#
# It is ad-hoc signed (codesign -s -), which Apple silicon requires of any
# binary it runs; notarizing it needs a Developer ID, which a release supplies.
# Off macOS, or without Xcode's command line tools, it says so and exits 0:
# authama works without it, and `owner fingerprint` then says the helper is
# missing. REQUIRE_TOUCHID=1 makes that an error instead.
set -euo pipefail

out="${1:?usage: touchid-helper.sh <out> [universal]}"
mode="${2:-host}"
here="$(cd "$(dirname "$0")" && pwd)"
src="${here}/../helpers/touchid/main.swift"

skip() {
  echo "  - authama-touchid not built: $1" >&2
  [ "${REQUIRE_TOUCHID:-0}" = "1" ] && exit 1
  exit 0
}
[ "$(uname -s)" = "Darwin" ] || skip "not on macOS"
command -v swiftc >/dev/null 2>&1 || skip "swiftc not found (install Xcode's command line tools: xcode-select --install)"

mkdir -p "$(dirname "$out")"
work="$(mktemp -d "${TMPDIR:-/tmp}/authama-touchid.XXXXXX")"
trap 'rm -rf -- "${work:?}"' EXIT
build() { swiftc -O -target "$1-apple-macos12" "$src" -o "$2"; }

case "$mode" in
  universal)
    build arm64 "${work}/arm64"
    build x86_64 "${work}/x86_64"
    lipo -create "${work}/arm64" "${work}/x86_64" -output "${work}/authama-touchid"
    ;;
  host)
    arch="$(uname -m)"
    [ "$arch" = "aarch64" ] && arch=arm64
    build "$arch" "${work}/authama-touchid"
    ;;
  *) echo "usage: touchid-helper.sh <out> [universal]" >&2; exit 64 ;;
esac
codesign -f -s - "${work}/authama-touchid" >/dev/null 2>&1 || true
chmod 0755 "${work}/authama-touchid"
mv -f "${work}/authama-touchid" "$out"
echo "built ${out}"
