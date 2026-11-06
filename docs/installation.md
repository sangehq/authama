# Installation

How to get `authama` onto a machine, and where it keeps its state once it is there.

`make build` produces `authama`. The `sshmgr` alias that earlier releases
installed beside it is gone; the [migration guide](migration.md#removing-the-sshmgr-alias)
says how to clear one out.

## Requirements

`authama` is a single static binary. There is no runtime to install and no
interpreter to keep current.

- OpenSSH: `ssh`, `ssh-keygen`, `ssh-add`, `ssh-keyscan` (hard dependencies).
  `ssh-copy-id` is used when present; Microsoft's OpenSSH port does not ship it,
  so deployment falls back to plain `ssh` there.
- Optional, each degrading to a manual path when absent: `age` and `age-keygen`
  (encrypted bundles), `sops`, `gitleaks`, `gh` and `glab` (VCS key deployment), `age-plugin-yubikey`,
  `docker` (only for `token apply` against a registry kept in a credential store).

The auth-token side needs nothing: it writes the tools' files itself.

macOS, Linux and Windows are all first-class, each validated on its own CI runner.

## Install

authama is built from source. Binary releases are on hold until the maintainer
decides otherwise, so this page offers no download, package-manager or
`go install` path. You need Go (the version `src/go.mod` names), `git` and `make`.

```sh
git clone https://github.com/simtabi/authama ~/src/authama
make -C ~/src/authama install    # builds build/<your-os>/authama, then copies it to ~/.local/bin
authama doctor          # verify deps, perms, agent, known_hosts, drift, token-file modes
```

`make install` builds first, so what lands on `PATH` is the checkout as it is now, and says
whether `~/.local/bin` is on your `PATH` and whether another `authama` shadows it. Any
directory works: `make install BINDIR=/usr/local/bin`. To check what is installed:

```sh
command -v authama      # which file runs: ~/.local/bin/authama after make install
authama version         # the commit it was built from
```

On a Mac with Xcode's command line tools (`xcode-select --install`), `make build` also builds
`build/macos/authama-touchid`, the helper that lets Touch ID stand in for the owner PIN, and
`make install` puts it beside `authama`. Without Swift it is skipped with a note and authama works
the same, asking for the PIN. See [Fingerprint unlock](tools/owner.md#fingerprint-unlock).

On Windows, `make install` from Git Bash puts it in `%LOCALAPPDATA%\Programs\authama`, where
`install.ps1` does. Or run the same `git clone` and `make build` from Git Bash (or `go -C src build -o
..\build\windows\authama.exe ./cmd/authama` from PowerShell), then put `build\windows\authama.exe`
in a directory on your user `PATH`.

### A double-click app

`make dist` also packages the web UI for people who would rather not use a terminal:

- **macOS:** `build/archives/authama_macos_app.zip` holds `Authama.app`. Opening it opens Terminal
  running `authama serve`, which opens the web UI in your browser; closing that window stops it.
  The app is not signed, so macOS asks once: right-click it and choose Open, or allow it in System
  Settings > Privacy & Security. `Authama.app/Contents/MacOS/authama` is the same command-line
  binary.
- **Windows:** each `build/archives/authama_windows_*.zip` holds `Start authama.cmd` beside
  `authama.exe`; double-clicking it starts the web UI in a console window, and closing the window
  stops it.

A version is a tag: check one out to build exactly that release.

```sh
git -C ~/src/authama fetch --tags
git -C ~/src/authama checkout v0.1.0
make -C ~/src/authama build
```

### Updating

```sh
git -C ~/src/authama pull --ff-only
make -C ~/src/authama install
```

`authama version` reports the commit it was built from, so a build from a branch
is still identifiable.

**Restart the web UI after updating.** A running `authama serve` keeps the code it
started with, so after `make install` the browser still shows the old UI. Each page then
says so in a yellow banner (*This page is from an older build*), and the header marks the
build *older*. Either:

- press **Restart now** in that banner: the server relaunches from the binary now installed,
  on the same address with the same sign-in, and the page reloads itself; a green banner
  then says it runs the current build, and the header marks it *current*; or
- from a terminal, restart every authama web UI on this machine at once:

```sh
authama serve --restart     # stop every authama web UI found, then start the new build
authama serve --stop-all    # only stop them
```

`--stop-all` asks each of the usual ports (7822 to 7841, plus `web.port` and the recorded
server's) whether authama answers there, and stops each that does, whichever home or build
it serves. A server from before this release does not say which process it is; it is named
with the command that stops it:

```sh
lsof -ti tcp:7911 -sTCP:LISTEN | xargs kill -INT
```

On Windows, close the console window `authama serve` runs in.

### Uninstalling

```sh
make -C ~/src/authama uninstall                       # or with the BINDIR you installed to
make -C ~/src/authama uninstall BINDIR=/usr/local/bin
```

It removes `authama` from `BINDIR` (default `~/.local/bin`), after checking the file answers
`version` as authama, and the Touch ID helper `authama-touchid` beside it. It prints where
your config and state homes are and leaves them as they are, with `~/.ssh` and every token
file: uninstalling never removes a key or a credential. To remove those too, use the paths
it prints (`authama settings path` shows them while authama is still installed); do that only
when you mean to, since the config home holds the manifest your keys are made from. Stop any
running `authama serve` first (see above); the checkout in `~/src/authama` is yours to keep
or delete.

### Earlier binary releases

Before 2026-11-06, versions 0.1.0 to 0.1.3 were published with binaries, archives,
Linux packages, a Homebrew cask and a Scoop manifest. The history was then folded
into one commit and the version restarted at v0.1.0, and those releases were
withdrawn: an installed binary keeps working, but the cask and the Scoop manifest
no longer download, and nothing is published while releases are on hold. Build
from source. Go's module proxy still serves the old `src/v0.1.0` to `src/v0.1.3`
it cached, which is one more reason this page offers no `go install` path. [Migration](migration.md) covers
moving an `sshmgr` install to `authama`.

### Docker

No runtime image is published: authama manages files in your home directory, so
it belongs on the host. The repository's [dev container](#dev-container) is for
working on authama itself, with a scoped home that never touches your real files.

### Shell completion

Generate completions from the binary you built:

```sh
authama completion bash > ~/.local/share/bash-completion/completions/authama
authama completion zsh  > "${fpath[1]}/_authama"
authama completion fish > ~/.config/fish/completions/authama.fish
```

On Windows, add `authama completion powershell | Out-String | Invoke-Expression`
to your PowerShell profile.

`make` is the development front door: `make check` runs the same gate as CI
(gofmt, build, vet, tests) plus lint. There is no bootstrap step.

## <a name="dev-container"></a>Dev container

For working on authama without a local Go toolchain, the repository root has a
`Dockerfile` and `compose.yaml`:

```sh
make docker-shell                       # a shell with Go, git, make and OpenSSH; authama built
make docker-run ARGS="token list"       # one command
make docker-down
```

The container's home is `./.authama-home` (gitignored; override with
`AUTHAMA_DEV_HOME`), **not** your real home, so a token written inside never
reaches your `~/.npmrc` or `~/.docker`. Point `AUTHAMA_DEV_HOME` at `$HOME` only
when that is what you mean. The container runs as an unprivileged `dev` user
with no capabilities.

Directories, not single files, are mounted: authama writes by atomic rename,
which fails with `EBUSY` over a single-file bind mount. If you run into trouble:

- **Files in the mounted home owned by the wrong user (Linux):** set
  `AUTHAMA_UID` / `AUTHAMA_GID` to `id -u` / `id -g`, then rebuild.
- **`token copy` fails inside the container:** there is no host clipboard. Use
  `token show --reveal`, or run `copy` on the host.
- **`docker compose` not found:** the Make targets use Compose v2;
  `make COMPOSE=docker-compose docker-shell` uses the legacy binary.

The `.env` and the rest of the per-user home are created by `authama init`, which
seeds `.env` from the shipped `.env-example` at mode `0600`.

## The per-user homes: config and state

authama keeps your **settings and secrets** in one config home and its
**runtime state** in a separate state home. The config home is the same
`~/.config/authama` on every OS, Windows included
(`%USERPROFILE%\.config\authama`), so a dotfiles repo or a backup script has one
path to know.

| What | Where (first match wins) |
|------|--------------------------|
| Config home | `$AUTHAMA_CONFIG_DIR` → `$XDG_CONFIG_HOME/authama` (absolute values only) → `~/.config/authama` |
| State home | `$AUTHAMA_STATE_DIR` → the config home itself, when `$AUTHAMA_CONFIG_DIR` was set → `$XDG_STATE_HOME/authama` → `~/.local/state/authama` |

```
~/.config/authama/                  # config home: what you edit or back up
├── .env                            # secrets and AUTHAMA_* settings (0600), loaded at startup
├── authama.json                    # OPTIONAL - non-secret settings, validated against a schema
├── manifest.json  inventory.json   # manifest is the source of truth
├── providers.json                  # OPTIONAL - else the shipped default catalog is used
├── age-identity.txt                # OPTIONAL - a place for the bundle decryption key (0600); point AUTHAMA_AGE_IDENTITY_FILE or --identity at it
└── tokens.json                     # OPTIONAL - the token config (env var names, no secrets)

~/.local/state/authama/             # state home: written by authama, safe to lose
├── log/audit.log  log/history.db   # accountability log and command history
├── snapshots/                      # reversible ~/.ssh backups
├── dist/                           # exported encrypted bundles (authama-<stamp>.age)
└── .state/                         # transient: .lock, expiry/notify caches, init backups
```

Setting `$AUTHAMA_CONFIG_DIR` keeps everything in that one directory (handy for
tests and for running several configurations side by side) unless
`$AUTHAMA_STATE_DIR` is set too. `authama settings path` and `authama doctor`
print both resolved homes. `~/.ssh` itself is unchanged - it's the generated
output.

**Upgrading from ssh-manager or sshmgr.** On the first run without an explicit
`$AUTHAMA_CONFIG_DIR`, when `~/.config/authama` does not exist yet, authama copies
the old home in - `~/.config/ssh-manager` (on Windows also
`%APPDATA%\ssh-manager`), or an older `~/.config/sshmgr` or `~/.sshmgr` - moves
logs, history, snapshots and bundles into the state home, writes
`MIGRATED-TO-AUTHAMA.txt` into the old directory and leaves it in place. It says
so once on stderr. The old `SSH_MANAGER_*` / `SSHMGR_*` variable names keep
working, with a deprecation notice. See [Migration](migration.md).

### Configuration precedence (user first, then shipped default)

The **manifest** is always the source of truth and is read from your home
(`init` seeds an empty one). For the **provider catalog**, your
`<home>/providers.json` is used **if present**; otherwise the full default catalog
**shipped with the package** (kept byte-identical to the repo's `src/config/providers.json`)
is used - so providers work out of the box, and you only create your own file to
customize them.

## First run

```sh
authama init                      # create/converge the home + starter manifest/inventory/.env
# edit <home>/manifest.json for your profiles/hosts (or: authama ssh import / profile add / host add)
authama ssh reconcile --dry-run   # preview what would change
authama ssh reconcile             # build ~/.ssh from the manifest (--passphrase to protect new keys)
authama ssh config check          # confirm it's in sync (exit 0)
authama ssh knownhosts init --all # pin every reachable host into the one ~/.ssh/known_hosts
authama token init                # optional: a starter token config, then `authama token apply`
```

`authama init` is safe to re-run: every run (re)creates the directory structure and
re-asserts perms, and seeds any missing files **without** touching your existing
manifest/`.env`. To reset the seed files to defaults, use `authama init --force` (it overwrites
them in place). Add `--backup` to first copy the old ones into the state home's `.state/init-backup-<ts>/`.

Passphrases are **off by default** but a conscious choice: pass `--passphrase` to
`reconcile`, `keygen`, `key add` or `rotate` to protect the keys they mint. Without it nothing prompts.

## Permissions / groups

Perms are load-bearing (SSH refuses loose key/config modes) and secrets must
never be group/world readable. The tooling fixes this automatically:

```sh
authama doctor --fix         # re-assert perms on managed ~/.ssh paths, secrets and token files
make doctor FIX=1            # same, via the front door, against the binary you just built
```

`reconcile` sets the correct modes as it writes, and `doctor` reports and repairs
them afterwards - the two walk the same enumeration, so they cannot disagree
about what is managed.

We never `chgrp`/`chown` (that needs root); denying group and other access is the
safe, correct hardening for a single-user tool - run it as the file owner. On
Windows the equivalent is an owner-only ACL applied with `icacls`, since mode
bits do not apply there.

---

[← Docs index](../README.md#documentation)
