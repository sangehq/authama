# Installation

How to get `authama` onto a machine, and where it keeps its state once it is there.

Every install path puts `authama` on `PATH`, and beside it `sshmgr`, the former
name, kept as a deprecated alias for existing scripts and `ProxyCommand` lines. It
accepts the same commands but prints a one-line deprecation notice on stderr
(not for `vm proxy` or shell completion, where stderr reaches ssh or the shell),
and it will be removed in a future release: switch scripts to `authama`.

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

From a release binary:

```sh
curl -fsSL https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.sh | bash
authama doctor          # verify deps, perms, agent, known_hosts, drift, token-file modes
```

The script installs `authama` and the `sshmgr` alias into `/usr/local/bin` (override with
`AUTHAMA_INSTALL_DIR`; the old `SSHMGR_INSTALL_DIR` still works), verifies each
against the release checksums, and installs shell completions for both. It asks
for `sudo` only when the directory is not writable, so a per-user directory needs
none. A version argument pins the release; without one it takes the latest:

```sh
curl -fsSL https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.sh \
  | AUTHAMA_INSTALL_DIR="$HOME/.local/bin" bash -s v0.1.3
```

On Windows:

```powershell
irm https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.ps1 | iex
# pin a version or a directory (the old SSHMGR_* names still work)
$env:AUTHAMA_VERSION = 'v0.1.3'; irm https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.ps1 | iex
```

It installs into `%LOCALAPPDATA%\Programs\authama` and adds it to the user
`PATH`. Upgrading from 0.1.0, it also takes the old
`%LOCALAPPDATA%\Programs\ssh-manager` folder off the user `PATH`; the files in it
are left alone, so delete that folder when you like.

### Package managers

The packages are named `authama` from 0.1.2 on (before that they were published
as `sshmgr`) and install both binaries. Upgrading from the `sshmgr` packages is
covered in [migration.md](migration.md#to-012-authama-everywhere-sshmgr-deprecated).

```sh
# Homebrew (macOS and Linux)
brew install --cask simtabi/tap/authama
```

```powershell
# Scoop (Windows)
scoop bucket add simtabi https://github.com/simtabi/scoop-bucket
scoop install simtabi/authama
```

Linux packages (`.deb`, `.rpm`, `.apk`) for `amd64`, `arm64`, `386` and `armv6`/`armv7`
are attached to every release and install `/usr/bin/authama` and `/usr/bin/sshmgr`:

```sh
curl -fsSLO https://github.com/simtabi/authama/releases/latest/download/authama_linux_amd64.deb
sudo apt install ./authama_linux_amd64.deb      # Debian, Ubuntu
sudo dnf install ./authama_linux_amd64.rpm      # Fedora, RHEL (download the .rpm instead)
sudo apk add --allow-untrusted ./authama_linux_amd64.apk   # Alpine (the .apk)
```

### Docker

No runtime image is published: authama manages files in your home directory, so
it belongs on the host. The repository's [dev container](#dev-container) is for
working on authama itself, with a scoped home that never touches your real files.

With the Go toolchain:

```sh
go install github.com/simtabi/authama/src/cmd/authama@latest
go install github.com/simtabi/authama/src/cmd/sshmgr@latest   # the deprecated alias, only if a script still needs it
```

The `src` segment is load-bearing: it is the subdirectory the Go module lives
in. There is no major-version suffix, because versions restarted at v0.1.0.
The releases before the restart were published as
`github.com/simtabi/ssh-manager/src/v3` (v3.6.1 alone as
`github.com/simtabi/authama/src/v3`); they are no longer maintained.

To build the current `main` instead of the last release — useful for a fix that
has landed but not shipped — name the branch:

```sh
go install github.com/simtabi/authama/src/cmd/authama@main
```

Go turns that into a pseudo-version (`v0.1.3-0.20261102100209-4d6289dd4059`) and
the binary reports it, so a build from a branch is still identifiable. It needs
no tag and no release, which also makes it the way in when a release is pending.

Or download the binary for your platform from
[Releases](https://github.com/simtabi/authama/releases) and put it on `PATH`.
Every artifact is checksummed and carries a build-provenance attestation
(`gh attestation verify <file> --repo simtabi/authama`).

### Shell completion

`install.sh` installs bash and zsh completions when it can write to a completion
directory. After any other install (Go, a downloaded binary), generate them
yourself:

```sh
authama completion bash > ~/.local/share/bash-completion/completions/authama
authama completion zsh  > "${fpath[1]}/_authama"
authama completion fish > ~/.config/fish/completions/authama.fish
```

On Windows, add `authama completion powershell | Out-String | Invoke-Expression`
to your PowerShell profile.

## From a clone

```sh
git clone https://github.com/simtabi/authama && cd authama
make build                # -> build/authama (and build/sshmgr, the deprecated alias)
./build/authama doctor
```

`make` is the development front door: `make check` runs the same gate as CI
(gofmt, build, vet, tests) plus lint. There is no bootstrap step.

## <a name="dev-container"></a>Dev container

For working on authama without a local Go toolchain, the repository root has a
`Dockerfile` and `compose.yaml`:

```sh
make docker-shell                       # a shell with Go, git, make and OpenSSH; authama (and the sshmgr alias) built
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

**Upgrading from 0.1.0 or earlier.** On the first run without an explicit
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
