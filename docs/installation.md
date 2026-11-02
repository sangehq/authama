# Installation

How to get `authama` onto a machine, and where it keeps its state once it is there.

Every install path puts two binaries on `PATH`: `authama`, and `sshmgr`, the
original name, kept for existing scripts and `ProxyCommand` lines. They are the same
program; use either.

## Requirements

`authama` is a single static binary. There is no runtime to install and no
interpreter to keep current.

- OpenSSH: `ssh`, `ssh-keygen`, `ssh-add`, `ssh-keyscan` (hard dependencies).
  `ssh-copy-id` is used when present; Microsoft's OpenSSH port does not ship it,
  so deployment falls back to plain `ssh` there.
- Optional, each degrading to a manual path when absent: `age` and `age-keygen`
  (encrypted bundles), `gh` and `glab` (VCS key deployment), `age-plugin-yubikey`,
  `docker` (only for `token apply` against a registry kept in a credential store).

The auth-token side needs nothing: it writes the tools' files itself.

macOS, Linux and Windows are all first-class, each validated on its own CI runner.

## Install

From a release binary:

```sh
curl -fsSL https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.sh | bash
authama doctor          # verify deps, perms, agent, known_hosts, drift, token-file modes
```

The script installs `authama` and `sshmgr` into `/usr/local/bin` (override with
`AUTHAMA_INSTALL_DIR`; the old `SSHMGR_INSTALL_DIR` still works), verifies each
against the release checksums, and installs shell completions for both. Against a
release older than authama it installs `sshmgr` alone.

On Windows:

```powershell
irm https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.ps1 | iex
```

With the Go toolchain:

```sh
go install github.com/simtabi/authama/src/cmd/authama@latest
go install github.com/simtabi/authama/src/cmd/sshmgr@latest   # the compat name, if you want it
```

The `src` segment is load-bearing: it is the subdirectory the Go module lives
in. There is no major-version suffix, because versions restarted at v0.1.0.
`github.com/simtabi/authama/src/v3/...` is the path of the releases before the
restart (v3.6.1 and earlier); the Go proxy still serves them, but they are no
longer maintained.

To build the current `main` instead of the last release — useful for a fix that
has landed but not shipped — name the branch:

```sh
go install github.com/simtabi/authama/src/cmd/authama@main
```

Go turns that into a pseudo-version (`v0.1.1-0.20261102073200-60ce5f9c4ee6`) and
the binary reports it, so a build from a branch is still identifiable. It needs
no tag and no release, which also makes it the way in when a release is pending.

Or download the binary for your platform from
[Releases](https://github.com/simtabi/authama/releases) and put it on `PATH`.
Every artifact is checksummed and carries a build-provenance attestation
(`gh attestation verify <file> --repo simtabi/authama`).

## From a clone

```sh
git clone https://github.com/simtabi/authama && cd ssh-manager
make build                # -> build/authama and build/sshmgr
./build/authama doctor
```

`make` is the development front door: `make check` runs the same gate as CI
(gofmt, build, vet, tests) plus lint. There is no bootstrap step.

## <a name="dev-container"></a>Dev container

For working on authama without a local Go toolchain, the repository root has a
`Dockerfile` and `compose.yaml`:

```sh
make docker-shell                       # a shell with Go, git, make and OpenSSH; authama and sshmgr built
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

## The per-user home (OS-standard config dir)

All of a user's ssh-manager state lives in **one** per-user home - a single `ssh-manager`
folder in the OS-standard config location:

| OS | Default home |
|----|--------------|
| Linux / macOS | `$XDG_CONFIG_HOME/ssh-manager` if set, else `~/.config/ssh-manager` |
| Windows | `%APPDATA%\ssh-manager` |

```
<home>/                             # e.g. ~/.config/ssh-manager
├── manifest.json  inventory.json   # manifest is the source of truth
├── providers.json                  # OPTIONAL - else the shipped default catalog is used
├── .env  age-identity.txt          # secrets (0600)
├── tokens.json                     # OPTIONAL - the token config (env var names, no secrets)
├── log/audit.log                   # accountability log
├── snapshots/                      # reversible ~/.ssh backups
├── dist/                           # exported encrypted bundles (ssh-manager-<stamp>.age)
└── .state/                         # transient: .lock, expiry/notify caches
```

`ssh-manager` resolves the home in this order:

1. `$SSH_MANAGER_HOME` (alias `$SSH_MANAGER_CONFIG_DIR`) if set - explicit override (tests / multiple configs)
2. otherwise the OS-standard dir above

A legacy `~/.sshmgr` home (from older versions) is **auto-migrated** to the
standard location on first run. `~/.ssh` itself is unchanged - it's the generated
output. `authama doctor` prints the resolved home so it's always clear where state
lives.

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
them in place). Add `--backup` to first copy the old ones into `<home>/.state/`.

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
