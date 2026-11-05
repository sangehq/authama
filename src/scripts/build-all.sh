#!/usr/bin/env bash
#
# Hand-rolled cross-compile build (GoReleaser is the canonical release path).
# Regenerates build/targets.txt from an embedded matrix and produces the
# standard layout: one folder per OS under build/, and nothing else.
#
#   build/{os}/authama_{os}_{arch}[.exe]   versionless, ready-to-run binaries
#   build/macos/authama_macos_universal    Intel + Apple Silicon fat binary (on macOS)
#   build/checksums.txt                    SHA-256 over every binary, as {os}/{file},
#                                          so `cd build && shasum -a 256 -c checksums.txt`
#                                          verifies as-is
#   build/archives/authama_{os}_{arch}.{tar.gz,zip}  the binary + LICENSE/README/CHANGELOG;
#                                          the Windows zips add "Start authama.cmd"
#   build/archives/authama_macos_app.zip   Authama.app (see macos-app.sh)
#   build/archives/checksums.txt           SHA-256 over the archives (bare names)
#
# Naming: darwin -> macos, arm -> armvN, .exe on windows. Static
# (CGO_ENABLED=0), reproducible (-trimpath), stripped (-s -w), with version
# metadata embedded.
#
# Every OS folder, build/archives/ and build/checksums.txt are DELETED and
# REGENERATED on every run, so nothing stale from a removed or renamed target
# survives. The tracked inputs (targets.txt, nfpm.yaml) stay; targets.txt is
# still rewritten from the effective matrix below.
#
#   VERSION=v1.2.3 ./scripts/build-all.sh          # normal
#   ARCHIVES=0 ./scripts/build-all.sh              # binaries only, skip archives
#   TARGETS=path/to/list ./scripts/build-all.sh    # use an external target list
#
# Portable to bash 3.2 (the macOS system bash): no mapfile / wait -n / assoc arrays.
set -euo pipefail

# Run from the module root (src/), because `go build ./cmd/authama` resolves
# relative to it. The repo root is one level up, and that is where build/ and the
# LICENSE/README/CHANGELOG bundled into each archive live - so paths below are
# deliberately explicit about which of the two roots they mean.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$(dirname "$0")/.."
REPO_ROOT=".."

# Archives are named after authama (sshmgr_ before 0.1.2). BINARIES is every
# program an archive holds: authama alone since the sshmgr alias was removed.
BINARY="authama"
BINARIES="authama"
PKG="github.com/simtabi/authama/src/internal/version"
BUILD_DIR="${REPO_ROOT}/build"
OUT="${BUILD_DIR}"
ARCHIVE_DIR="${OUT}/archives"
# The OS folders this script owns, and wipes before it writes.
OS_DIRS="macos linux windows freebsd openbsd netbsd"
TARGETS_FILE="${BUILD_DIR}/targets.txt"
WITH_ARCHIVES="${ARCHIVES:-1}"

# The canonical target matrix. Regenerated into build/targets.txt each run; used
# as the fallback when no targets.txt (or --TARGETS override) is present.
DEFAULT_TARGETS='# Release targets for scripts/build-all.sh — "GOOS GOARCH [GOARM]".
# Regenerated on every build. GoReleaser (.goreleaser.yaml) is the canonical
# release path; this drives the hand-rolled fallback. Names use {os}_{arch} with
# darwin → macos, arm → armvN, and .exe on windows; a macos_universal (Intel +
# Apple Silicon) fat binary is produced on macOS.
linux amd64
linux 386
linux arm64
linux arm 6
linux arm 7
darwin amd64
darwin arm64
windows amd64
windows 386
windows arm64'

# --- preflight ---------------------------------------------------------------
command -v go >/dev/null 2>&1 || { echo "error: 'go' is not on PATH" >&2; exit 1; }

# Capture the effective target matrix BEFORE wiping build/, in priority order:
# an explicit TARGETS override, then the existing build/targets.txt, then the
# embedded default. $MATRIX is what the build loop iterates - not targets.txt,
# which is an output of this decision rather than an input to it. The loop used
# to read the file, so the two agreed only because the file was unconditionally
# rewritten first; once it is not, a TARGETS= override would be ignored.
OVERRIDDEN=0
if [ -n "${TARGETS:-}" ] && [ -f "$TARGETS" ]; then
  MATRIX="$(cat "$TARGETS")"
  OVERRIDDEN=1
elif [ -f "$TARGETS_FILE" ]; then
  MATRIX="$(cat "$TARGETS_FILE")"
else
  MATRIX="$DEFAULT_TARGETS"
fi

# Portable SHA-256: Linux ships sha256sum, macOS/BSD ship shasum.
sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"
  elif command -v shasum   >/dev/null 2>&1; then shasum -a 256 "$@"
  else echo "error: no sha256sum or shasum found" >&2; return 1; fi
}

# --match 'v[0-9]*' so no non-release tag becomes the version, and so the src/
# module-alias tags (see the Makefile) are skipped: they are not semver and are
# read by nothing but the Go toolchain.
VERSION="${VERSION:-$(git describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X ${PKG}.Version=${VERSION} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.Date=${DATE}"

# Extra files bundled into each archive (whichever exist). They are repo-root
# files, not module files, so they are reached through REPO_ROOT; `cp` still
# lands them in the archive under their bare names.
EXTRAS=""
for f in LICENSE README.md CHANGELOG.md; do
  [ -f "${REPO_ROOT}/${f}" ] && EXTRAS="${EXTRAS} ${REPO_ROOT}/${f}"
done

# --- fresh output folder -----------------------------------------------------
echo "==> ${BINARY} ${VERSION} (${COMMIT}) — regenerating ${OUT}/{os}/"
case "$OUT" in ''|/|.|..) echo "error: unsafe OUT='$OUT'" >&2; exit 1 ;; esac
for d in $OS_DIRS archives; do rm -rf "${OUT:?}/${d}"; done
rm -f "${OUT}/checksums.txt"
mkdir -p "$OUT"
[ "$WITH_ARCHIVES" = "1" ] && mkdir -p "$ARCHIVE_DIR"
# Regenerate the tracked target list - but never from a TARGETS= override. That
# override exists so you can build one target while iterating; writing it back
# would silently shrink the release matrix to whatever you last debugged with,
# and the next release would ship one binary without anyone noticing.
if [ "$OVERRIDDEN" = "1" ]; then
  echo "==> TARGETS override in effect; leaving ${TARGETS_FILE} alone"
else
  printf '%s\n' "$MATRIX" > "$TARGETS_FILE"
  echo "==> regenerated ${TARGETS_FILE}"
fi

# make_archive <authama-bare-binary-filename> <goos>: the binaries, under their
# plain names, plus the docs.
make_archive() {
  [ "$WITH_ARCHIVES" = "1" ] || return 0
  bin="$1"; goos="$2"
  base="${bin%.exe}"                      # authama_macos_arm64
  ext="";  [ "$goos" = "windows" ] && ext=".exe"
  stage="$(mktemp -d "${TMPDIR:-/tmp}/authama.XXXXXX")"
  os_dir="$goos"; [ "$goos" = "darwin" ] && os_dir="macos"
  for b in $BINARIES; do
    cp "${OUT}/${os_dir}/${b}${bin#"${BINARY}"}" "${stage}/${b}${ext}"
  done
  # shellcheck disable=SC2086
  [ -n "$EXTRAS" ] && cp $EXTRAS "${stage}/"
  # Windows: a double-click starts the web UI in a console window; closing the
  # window stops the server.
  if [ "$goos" = "windows" ]; then
    printf '@echo off\r\ntitle authama\r\n"%%~dp0authama.exe" serve\r\npause\r\n' > "${stage}/Start authama.cmd"
  fi
  if [ "$goos" = "windows" ]; then
    if command -v zip >/dev/null 2>&1; then
      ( cd "$stage" && zip -qr "${base}.zip" . )
      mv "${stage}/${base}.zip" "${ARCHIVE_DIR}/"
    else
      echo "  ! 'zip' not found — skipping ${base}.zip" >&2
    fi
  else
    tar -czf "${ARCHIVE_DIR}/${base}.tar.gz" -C "$stage" .
  fi
  rm -rf "$stage"
}

# --- build every target ------------------------------------------------------
count=0
have_macos_amd64=0
have_macos_arm64=0

# `|| [ -n "$goos" ]` processes a final line with no trailing newline; the \r in
# IFS tolerates CRLF line endings.
while IFS=$' \t\r' read -r goos goarch goarm || [ -n "${goos:-}" ]; do
  case "$goos" in ''|\#*) continue ;; esac   # skip blanks and comments

  os_token="$goos";   [ "$goos" = "darwin" ] && os_token="macos"
  arch_token="$goarch"; [ "$goarch" = "arm" ] && arch_token="armv${goarm}"
  ext="";             [ "$goos" = "windows" ] && ext=".exe"

  mkdir -p "${OUT}/${os_token}"
  for b in $BINARIES; do
    printf '  -> %-40s' "${os_token}/${b}_${os_token}_${arch_token}${ext}"
    GOOS="$goos" GOARCH="$goarch" GOARM="${goarm:-}" CGO_ENABLED=0 \
      go build -trimpath -ldflags "$LDFLAGS" -o "${OUT}/${os_token}/${b}_${os_token}_${arch_token}${ext}" "./cmd/${b}"
    echo "ok"
  done
  name="${BINARY}_${os_token}_${arch_token}${ext}"

  make_archive "$name" "$goos"
  count=$((count + 1))
  [ "$name" = "${BINARY}_macos_amd64" ] && have_macos_amd64=1
  [ "$name" = "${BINARY}_macos_arm64" ] && have_macos_arm64=1
done <<< "$MATRIX"

[ "$count" -gt 0 ] || { echo "error: no targets built (empty matrix?)" >&2; exit 1; }

# --- macOS universal (Intel + Apple Silicon in one binary) -------------------
if [ "$have_macos_amd64" = 1 ] && [ "$have_macos_arm64" = 1 ]; then
  if command -v lipo >/dev/null 2>&1; then
    for b in $BINARIES; do
      printf '  -> %-40s' "macos/${b}_macos_universal"
      lipo -create -output "${OUT}/macos/${b}_macos_universal" \
        "${OUT}/macos/${b}_macos_amd64" "${OUT}/macos/${b}_macos_arm64"
      echo "ok"
    done
    make_archive "${BINARY}_macos_universal" "darwin"
  else
    echo "  ! 'lipo' not available (non-macOS host) — skipping macos_universal" >&2
  fi
fi

# --- macOS app ----------------------------------------------------------------
# The universal binary when there is one, else whichever macOS build exists.
if [ "$WITH_ARCHIVES" = "1" ]; then
  app_bin=""
  for cand in universal arm64 amd64; do
    [ -f "${OUT}/macos/${BINARY}_macos_${cand}" ] && { app_bin="${OUT}/macos/${BINARY}_macos_${cand}"; break; }
  done
  if [ -n "$app_bin" ]; then
    # The Touch ID helper needs Swift, so only a Mac builds it; elsewhere the
    # app ships without it and the PIN is the unlock.
    helper="${OUT}/macos/authama-touchid_macos_universal"
    "${SCRIPT_DIR}/touchid-helper.sh" "$helper" universal >/dev/null || true
    [ -f "$helper" ] || helper=""
    printf '  -> %-40s' "archives/${BINARY}_macos_app.zip"
    VERSION="$VERSION" "${SCRIPT_DIR}/macos-app.sh" "$app_bin" "${ARCHIVE_DIR}/${BINARY}_macos_app.zip" ${helper:+"$helper"} >/dev/null
    echo "ok"
  fi
fi

# --- checksums: {os}/{file} paths, so the file verifies from build/ as-is -------
# Every OS folder's binaries; archives/ has its own checksums.txt.
( cd "$OUT" && for d in $OS_DIRS; do [ -d "$d" ] && sha256 "$d"/${BINARY}_*; done > checksums.txt ) || true
echo "==> wrote ${OUT}/checksums.txt"
if [ "$WITH_ARCHIVES" = "1" ] && ls "${ARCHIVE_DIR}/${BINARY}_"* >/dev/null 2>&1; then
  ( cd "$ARCHIVE_DIR" && sha256 ${BINARY}_* > checksums.txt )
  echo "==> wrote ${ARCHIVE_DIR}/checksums.txt"
fi

echo "==> done: ${count} targets (each: ${BINARIES}) in ${OUT}/{os}/$( [ "$WITH_ARCHIVES" = "1" ] && echo ", archives in ${ARCHIVE_DIR}" )"
