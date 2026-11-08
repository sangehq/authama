# authama

<!-- Three badges, not the usual four: authama ships as a static binary and has no
     package-registry release, so a registry-version badge would have no target. -->
[![Tests](https://img.shields.io/github/actions/workflow/status/simtabi/authama/ci.yml?branch=main&label=tests)](https://github.com/simtabi/authama/actions/workflows/ci.yml)
[![Static analysis](https://img.shields.io/github/actions/workflow/status/simtabi/authama/codeql.yml?branch=main&label=codeql)](https://github.com/simtabi/authama/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> Authentication Access Manager: SSH keys and config, and package-manager / git auth tokens, in one tool.

Two features in a single static binary with no runtime dependencies:

- **SSH management** (`authama ssh …`) - profile-based key and config lifecycle:
  one manifest is the source of truth, and `~/.ssh` is reproducible output.
- **Auth token management** (`authama token …`) - the credential files Composer,
  npm, Docker, git and curl read, built from a config that names environment
  variables, never secrets.

macOS, Linux and Windows are all first-class; OpenSSH is the only requirement
for the SSH side. `sshmgr`, the former name, is no longer built or installed.
Coming from sshmgr or auth-token-manager? See the [migration guide](docs/migration.md).

## Install

authama is built from source: binary releases are on hold, so there is no
download, package-manager or `go install` channel to use. You need Go (the
version `src/go.mod` names), `git` and `make`.

```sh
git clone https://github.com/simtabi/authama ~/src/authama
make -C ~/src/authama install    # builds build/<your-os>/authama, then copies it to ~/.local/bin
authama doctor
```

Updating, shell completion, where per-user state lives, and what became of the
binaries published before the history restarted are in [docs/installation.md](docs/installation.md). There is
no runtime Docker image; the [dev container](docs/installation.md#dev-container) is
for working on authama itself.

## Quick start guide and usage

### Getting started

`init` seeds an **empty** manifest; add a profile and a host before `reconcile`
mints anything:

```sh
authama init                      # create ~/.config/authama (dirs, perms, seed files)
authama ssh profile add work
authama ssh host add work gh -H github.com -u git --provider github   # or edit manifest.json
authama ssh host apply work acme-prd-web-00 203.0.113.10 deploy web   # add-or-update from a compact spec
authama ssh import ~/.ssh/config                       # or onboard an existing setup
```

### Usage

SSH - build `~/.ssh` from the manifest, and get a key onto its target:

```sh
authama ssh reconcile --dry-run   # preview: what would be minted, what would be written
authama ssh reconcile             # build ~/.ssh from the manifest
authama ssh config check          # confirm the file matches the manifest (exit != 0 on drift)
authama ssh deploy work_gh-ed25519    # ssh-copy-id, gh, glab, a REST API, or manual
authama ssh validate                  # every keypair parses and the halves match
```

Auth tokens:

```sh
authama token init                # a starter tokens.json (env var names, no secrets)
GH_TOKEN="$(op read op://Private/GitHub/token)" authama token apply   # write auth.json, .npmrc, …
authama token set --target npm --server registry.npmjs.org        # or one token, from a hidden prompt
authama token list                # what is installed, secrets masked
authama token show npm registry.npmjs.org   # one entry in full
authama token status              # which config, which files, which variables are missing
authama token audit               # owner-only modes, .gitignore coverage
```

Both:

```sh
authama doctor                    # SSH health and token-file modes; --fix repairs modes and syncs VM hosts
authama ssh vm sync --dry-run     # keep VM hosts matching their VMs (--install-schedule: daily)
authama ssh key reserve docker-tunnel --for devbox   # keep a loose key for later; doctor stops warning
authama                           # on a terminal: the web UI on this machine, as authama serve
authama serve                     # a web UI on this machine, with previewed changes
authama ssh help deploy           # help at any level: authama help, authama auth help set
authama history                   # every change made, SSH or token
authama events --since 24h        # every change to a tracked file, by anyone
authama man ~/.local/share/man/man1   # manual pages: man authama-ssh-deploy
```

The full walkthrough is in [Getting started](docs/getting-started.md); everything
else is in the [documentation index](#documentation).

## <a name="documentation"></a>Documentation

### Guides

- [Installation](docs/installation.md) — install paths, requirements, and where per-user state lives.
- [Getting started](docs/getting-started.md) — first run, from `init` to a deployed key.
- [Configuration](docs/configuration.md) — the manifest, profiles, hosts, `.env`, environment variables.
- [Architecture](docs/architecture.md) — packages, key flows, and why the layout is what it is.
- [Migration](docs/migration.md) — from sshmgr 3.x and from auth-token-manager, with a feature-parity table.
- [Release](docs/release.md) — the tag-driven GoReleaser flow.

### Reference

- [Command reference](docs/reference/cli.md) — every command and subcommand with its flags, aliases and examples, generated from the binary (a test keeps it current), plus a "which command do I want" table.
- [Feature catalog](docs/features.md) — every command, what it does, and how it is tested.
- [`token`](docs/tools/tokens.md) — auth tokens: apply, set, list, show, status, copy, delete, audit, import; per tool: [Composer](docs/tools/tokens/composer.md), [npm](docs/tools/tokens/npm.md), [Docker](docs/tools/tokens/docker.md), [git](docs/tools/tokens/git.md), [netrc](docs/tools/tokens/netrc.md), [PyPI](docs/tools/tokens/pypi.md), [Cargo](docs/tools/tokens/cargo.md), [RubyGems](docs/tools/tokens/gem.md), [Gradle](docs/tools/tokens/gradle.md), [Maven](docs/tools/tokens/maven.md), [Yarn Berry](docs/tools/tokens/yarn.md), [Helm](docs/tools/tokens/helm.md), [GitHub CLI](docs/tools/tokens/gh.md).
- [`doctor`](docs/tools/doctor.md) — what it checks (SSH and token files), `--fix`, `--json`, `--strict`.
- [`deploy`](docs/tools/deploy.md) — installing a public key on its target.
- [`checklist`](docs/tools/checklist.md) — where each key and token is used, and what is left to rotate or revoke it; choosing several keys to rotate, revoke or delete.
- [`providers`](docs/tools/providers.md) — the adapter catalog and how to extend it.
- [VPS keys](docs/tools/vps.md) — cloud account keys and server keys.
- [Key lifecycle](docs/tools/key-lifecycle.md) — a key's lifetime, rotation, regeneration and revocation, in one place.
- [Repair and factory reset](docs/tools/repair.md) — fix a broken home, `~/.ssh` modes and config, and token file modes; reset without deleting anything.
- [Owner binding](docs/tools/owner.md) — a PIN for secrets and destructive changes, and a seal that notices outside edits and a home copied elsewhere.
- [`rotate`](docs/tools/rotate.md) — zero-downtime staged rotation and rollback.
- [`expiry`](docs/tools/expiry.md) — rotation age and scheduled reminders.
- [`knownhosts`](docs/tools/knownhosts.md) — pinning host keys into the trust store.
- [`git`](docs/tools/git.md) — checkouts' GitHub remotes in profile SSH, plain SSH or HTTPS mode.
- [`vm`](docs/tools/vm.md) — local VMs as hosts, addressed through their hypervisor; configuring hypervisors, `vm sync`, `vm adopt-key`, keeping a key for later (`key reserve`).
- [`export`](docs/tools/export.md) — `authama export <list>`: any list (hosts, keys, profiles, tokens, history, events, snapshots, deployments), filtered, with the columns you choose, in nine formats; and a flat `ssh_config`, a Termius CSV, or your hosts for Windows Terminal, iTerm2, PuTTY, Remmina or any CSV importer.
- [`net`](docs/tools/network.md) — reachability and VPN-gated hosts.
- [`validate`](docs/tools/validate.md) — keypair integrity checks.
- [`bundle`](docs/tools/bundle.md) — encrypted backup and restore.
- [`history`](docs/tools/history.md) — every change authama has made, filtered by host, profile or key.
- [`events`](docs/tools/events.md) — every change seen to a file under `~/.ssh`, the config home or a token file, by anyone.
- [`man`](docs/tools/man.md) — a manual page for every command, and the layout every `--help` shares.
- [`serve`](docs/tools/serve.md) — the web UI: overview, hosts, keys, snapshots, auth tokens and history in a browser, with previewed changes.
- [`recover`](docs/tools/recover.md) — break-glass when you are locked out.
- [`tui`](docs/tools/tui.md) — the terminal menu, removed: where each of its entries went.

### Recipes

- [Add or update many hosts at once](docs/recipes/host-specs.md) — `host apply` with `<alias> <host> <user> [role]` lines, re-runnable.
- [Two GitHub accounts on one machine](docs/recipes/two-github-accounts.md) — one hostname, two identities, neither able to act as the other.
- [Keep keys in 1Password](docs/recipes/keep-keys-in-1password.md) — private keys generated and rotated inside a vault; only the `.pub` on disk.
- [Use a GitHub machine user](docs/recipes/github-machine-user.md) — one bot key with read access to every org repo, rotated in one command.
- [Group profiles into categories](docs/recipes/group-profiles-into-categories.md) — `internal`, `clients`, `others`: one directory and one `config` file each.
- [Keep your keys in a directory of your choosing](docs/recipes/choose-where-keys-live.md) — move `~/.ssh/profiles/` to `~/.ssh/keys/` or any name, keys and config together.
- [Switch checkouts to profile SSH](docs/recipes/switch-git-remote-modes.md) — each owner's own key, every URL proven before it is written.
- [Use a local VM as a devbox](docs/recipes/devbox-vm.md) — reach a VM by name whatever its IP, and point Docker at it.
- [Import your hosts into Termius](docs/recipes/import-hosts-into-termius.md) — every host with its user, then `termius keys` to paste each key into Termius's Keychain (the CSV carries no keys).
- [Reach a server through Cloudflare Access](docs/recipes/cloudflare-access.md) — `ssh <alias>` through `cloudflared`, with no SSH port open to the internet.
- [Onboard an existing `~/.ssh`](docs/recipes/onboard-an-existing-ssh-dir.md) — bring a hand-built setup under management without regenerating anything.
- [Rotate a key with no downtime](docs/recipes/rotate-a-key.md) — stage, verify, commit; and how to step back.
- [Rotate or repair many keys at once](docs/recipes/rotate-many-keys.md) — a category or everything in one batch, and rebuilding `.pub` files.
- [Manage SSH from the browser](docs/recipes/manage-ssh-from-the-browser.md) — preview a change before it is written, and watch a deploy or rotation run.
- [Work against a sandbox](docs/recipes/dev-mode.md) — exercise every verb without touching your real `~/.ssh`.
- [Back up, and restore onto a new machine](docs/recipes/back-up-and-restore.md) — the only path that recovers the same keys.
- [Recover from a locked-out server](docs/recipes/recover-from-lockout.md) — break-glass with no working SSH.
- [Run it in CI](docs/recipes/run-in-ci.md) — non-interactive, sandboxed, failing loudly.
- [Develop in a container](docs/installation.md#dev-container) — `make docker-shell`, with a scoped home so tokens never reach your real files.

## Community

[Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md) · [Code of conduct](CODE_OF_CONDUCT.md) · [Changelog](CHANGELOG.md)

## License

MIT © Simtabi LLC — see [LICENSE](LICENSE).
