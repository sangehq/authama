#!/usr/bin/env bash
#
# Wraps an authama macOS binary in Authama.app and zips it:
#
#   scripts/macos-app.sh <authama binary> <out.zip>
#
# Double-clicking the app opens Terminal running `authama serve`, which prints
# the sign-in address and opens the web UI in the browser. The server runs in
# that window and stops when it is closed (or with Ctrl-C), so nothing is left
# running out of sight, and the app needs no native GUI code to be built or
# signed. The binary inside is the same one the command line uses:
#   Authama.app/Contents/MacOS/authama
#
# The app is not signed or notarized: macOS asks once before opening it (open
# it with right-click > Open, or allow it in System Settings > Privacy &
# Security). Signing needs a Developer ID, which a release supplies.
set -euo pipefail

bin="${1:?usage: macos-app.sh <authama binary> <out.zip>}"
out="${2:?usage: macos-app.sh <authama binary> <out.zip>}"
version="${VERSION:-dev}"
plain="${version#v}"
# CFBundleShortVersionString must be numeric dotted; a git-describe suffix goes.
short="$(printf '%s' "$plain" | sed -E 's/^([0-9]+(\.[0-9]+){0,2}).*/\1/')"
case "$short" in [0-9]*) ;; *) short="0.0.0" ;; esac

stage="$(mktemp -d "${TMPDIR:-/tmp}/authama.XXXXXX")"
trap 'rm -rf "$stage"' EXIT
app="${stage}/Authama.app"
mkdir -p "${app}/Contents/MacOS" "${app}/Contents/Resources"
cp "$bin" "${app}/Contents/MacOS/authama"
chmod 0755 "${app}/Contents/MacOS/authama"

# The launcher: Terminal runs a .command file, so the server is visible.
cat > "${app}/Contents/Resources/authama-serve.command" <<'CMD'
#!/bin/sh
here="$(cd "$(dirname "$0")/../MacOS" && pwd)"
printf 'authama web UI - close this window, or press Ctrl-C, to stop it.\n\n'
exec "$here/authama" serve
CMD
chmod 0755 "${app}/Contents/Resources/authama-serve.command"

# The bundle's executable. It is authama-app, not Authama: APFS and HFS+ are
# case-insensitive by default, so "Authama" would be the same file as the
# "authama" binary beside it and overwrite it.
cat > "${app}/Contents/MacOS/authama-app" <<'LAUNCH'
#!/bin/sh
res="$(cd "$(dirname "$0")/../Resources" && pwd)"
exec /usr/bin/open -a Terminal "$res/authama-serve.command"
LAUNCH
chmod 0755 "${app}/Contents/MacOS/authama-app"

cat > "${app}/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>Authama</string>
  <key>CFBundleDisplayName</key><string>Authama</string>
  <key>CFBundleIdentifier</key><string>com.simtabi.authama</string>
  <key>CFBundleExecutable</key><string>authama-app</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${short}</string>
  <key>CFBundleVersion</key><string>${plain}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHumanReadableCopyright</key><string>Copyright © Simtabi LLC. MIT licence.</string>
</dict>
</plist>
PLIST

mkdir -p "$(dirname "$out")"
rm -f "$out"
# ditto keeps the bundle's modes and structure the way Finder expects.
# --norsrc (which implies --noextattr and --noacl) and --noqtn leave out the
# resource forks, extended attributes and quarantine flag of this machine, so
# the zip holds no ._ AppleDouble files. zip is the fallback off macOS, with
# COPYFILE_DISABLE for the same reason.
abs_out="$(cd "$(dirname "$out")" && pwd)/$(basename "$out")"
if command -v ditto >/dev/null 2>&1; then
  ditto -c -k --norsrc --noqtn --keepParent "$app" "$abs_out"
else
  ( cd "$stage" && COPYFILE_DISABLE=1 zip -qrX "$abs_out" Authama.app )
fi
# Check what was written rather than trusting the exit codes: the binary must
# be the real one (a name clash once replaced it with the 120-byte launcher),
# and no AppleDouble file may ride along.
listing="$(unzip -l "$abs_out")"
case "$listing" in *"/._"*) echo "error: ${out} holds AppleDouble (._) files" >&2; exit 1 ;; esac
size="$(unzip -l "$abs_out" 'Authama.app/Contents/MacOS/authama' | awk 'NR==4 {print $1}')"
want="$(wc -c < "$bin" | tr -d ' ')"
if [ "$size" != "$want" ]; then
  echo "error: Authama.app/Contents/MacOS/authama is ${size:-missing} bytes, the binary is ${want}" >&2
  exit 1
fi
echo "wrote $out"
