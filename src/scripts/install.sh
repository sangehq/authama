#!/usr/bin/env bash
#
# Authentication Access Manager installer for macOS and Linux.
#
# Installs authama. The sshmgr alias that older releases installed beside it is
# no longer installed; one already there is left alone, and named, so it can be
# removed by hand.
#
#   curl -fsSL https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.sh | bash -s v0.1.0
#
# Windows users: use src/scripts/install.ps1 (irm ... | iex) or download the bare
# authama_windows_<arch>.exe from the releases page.
#
set -euo pipefail

OWNER="simtabi"
REPO="authama"
BINARY="authama"
# The name releases before 0.1.0 shipped as; only ever reported, never installed.
LEGACY="sshmgr"

# AUTHAMA_INSTALL_DIR, else the deprecated SSHMGR_INSTALL_DIR, else /usr/local/bin.
INSTALL_DIR="${AUTHAMA_INSTALL_DIR:-${SSHMGR_INSTALL_DIR:-/usr/local/bin}}"

# AUTHAMA_RELEASE_BASE is where the binaries and checksums.txt are read from
# instead of the GitHub release: a URL, or file:///path to a directory. CI
# points it at a binary it just built, since binary releases are on hold and
# the release has none to download.
RELEASE_BASE="${AUTHAMA_RELEASE_BASE:-}"

err()  { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*" >&2; }

[ "$(id -u)" = "0" ] && err "do not run this installer as root; it requests sudo only when needed."

# --- detect os (artifact token: darwin → macos) -----------------------------
case "$(uname -s)" in
  Darwin) os="macos" ;;
  Linux)  os="linux" ;;
  *) err "unsupported OS $(uname -s). On Windows, use the PowerShell installer or the .zip from the releases page." ;;
esac

# --- detect arch (Go-style GOARCH, matching the release artifact names) ------
case "$(uname -m)" in
  x86_64|amd64)   arch="amd64" ;;
  arm64|aarch64)  arch="arm64" ;;
  armv7l)         arch="armv7" ;;
  armv6l)         arch="armv6" ;;
  i386|i686)      arch="386" ;;
  *) err "unsupported architecture $(uname -m)." ;;
esac

if [ "$os" = "macos" ] && [ "$arch" != "amd64" ] && [ "$arch" != "arm64" ]; then
  err "unsupported macOS architecture $(uname -m)."
fi

# --- version resolution -----------------------------------------------------
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  api="https://api.github.com/repos/${OWNER}/${REPO}/releases/latest"
  auth=()
  [ -n "${GITHUB_TOKEN:-}" ] && auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
  # ${auth[@]+"${auth[@]}"}, not "${auth[@]}": under `set -u`, bash 3.2 treats an
  # EMPTY array expansion as an unbound variable and exits. macOS still ships
  # 3.2.57 as /bin/bash, which is what `#!/usr/bin/env bash` finds there unless
  # the user has installed a newer one - so the documented `curl ... | bash`
  # failed on a stock Mac with "auth[@]: unbound variable", before downloading
  # anything, for everyone who did not happen to have GITHUB_TOKEN set (which
  # made the array non-empty and hid it). Fixed in bash 4.4; this idiom works in
  # both.
  # Fetch first, parse second, rather than piping curl into `grep -m1`. grep -m1
  # closes the pipe as soon as it matches, curl gets EPIPE and exits 56
  # ("Failure writing output to destination"), and `set -o pipefail` turns that
  # into a failed install - intermittently, depending on whether curl had
  # finished writing. awk reads a here-string, so there is no pipeline left for
  # an early exit to break.
  body="$(curl -fsSL ${auth[@]+"${auth[@]}"} -H 'Accept: application/vnd.github+json' "$api" || true)"
  VERSION="$(awk -F'"' '/"tag_name"/{print $4; exit}' <<<"$body")"
  # The API allows 60 unauthenticated requests an hour per IP, and anyone behind
  # a shared NAT - or on a CI runner - can find that spent and get a 403. The
  # releases/latest web redirect is not counted against it and names the same
  # tag, so fall back to it rather than failing the install.
  if [ -z "$VERSION" ]; then
    latest="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${OWNER}/${REPO}/releases/latest" || true)"
    case "$latest" in */tag/*) VERSION="${latest##*/tag/}" ;; esac
  fi
fi
[ -n "$VERSION" ] || err "could not determine the version to install."
case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac

# Bare, ready-to-run binaries (the release tag carries the version).
base="${RELEASE_BASE:-https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}}"

sha_check() { # <checksums file> <asset>
  if command -v sha256sum >/dev/null 2>&1; then
    grep " ${2}\$" "$1" | sha256sum -c -
  else
    grep " ${2}\$" "$1" | shasum -a 256 -c -
  fi
}

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

have_sums=0
if curl -fsSL "${base}/checksums.txt" -o "${workdir}/checksums.txt" 2>/dev/null; then
  have_sums=1
else
  info "warning: checksums.txt not found; skipping verification."
fi

# fetch <binary>: download and verify ${binary}_${os}_${arch}; non-zero if the
# release does not carry it.
fetch() {
  asset="${1}_${os}_${arch}"
  info "Downloading ${asset} (${VERSION}) ..."
  curl -fsSL "${base}/${asset}" -o "${workdir}/${asset}" 2>/dev/null || return 1
  if [ "$have_sums" = 1 ]; then
    ( cd "$workdir" && sha_check checksums.txt "$asset" ) || err "checksum verification failed for ${asset}"
  fi
}

fetch "$BINARY" || err "${VERSION} has no ${BINARY}_${os}_${arch} (releases before 0.1.0 shipped as ${LEGACY})."
installed="$BINARY"

sudo=""
if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR" 2>/dev/null || { sudo="sudo"; $sudo mkdir -p "$INSTALL_DIR"; }
fi
if [ -z "$sudo" ] && [ ! -w "$INSTALL_DIR" ]; then
  sudo="sudo"
  info "Requesting sudo to write to ${INSTALL_DIR} ..."
fi
for b in $installed; do
  $sudo install -m 0755 "${workdir}/${b}_${os}_${arch}" "${INSTALL_DIR}/${b}"
done

# Shell completions (best effort), one set per installed name.
for b in $installed; do
  if command -v brew >/dev/null 2>&1; then
    prefix="$(brew --prefix)"
    "${INSTALL_DIR}/${b}" completion bash > "${prefix}/etc/bash_completion.d/${b}" 2>/dev/null || true
    "${INSTALL_DIR}/${b}" completion zsh  > "${prefix}/share/zsh/site-functions/_${b}" 2>/dev/null || true
  elif [ -d /etc/bash_completion.d ] && [ -w /etc/bash_completion.d ]; then
    "${INSTALL_DIR}/${b}" completion bash > "/etc/bash_completion.d/${b}" 2>/dev/null || true
  fi
done

first="${installed%% *}"
info ""
"${INSTALL_DIR}/${first}" version || true
info ""
for b in $installed; do
  info "Installed ${b} to ${INSTALL_DIR}/${b}"
done
if [ -e "${INSTALL_DIR}/${LEGACY}" ]; then
  info "An older ${LEGACY} is still at ${INSTALL_DIR}/${LEGACY}; ${BINARY} replaces it."
  info "Remove it with: ${sudo:+sudo }rm ${INSTALL_DIR}/${LEGACY}  (then: ${BINARY} ssh config render)"
fi
info ""
info "Settings live in ${XDG_CONFIG_HOME:-$HOME/.config}/authama (.env and authama.json, loaded at startup)."
info "A 0.1.0 home in ~/.config/ssh-manager is copied there on the first run and kept."
