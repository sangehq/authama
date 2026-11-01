# Migration guide

authama (Authentication Access Manager) is ssh-manager v3 with
auth-token-manager merged in. It has two features, under two groups:

- `authama ssh …` - SSH keys, profiles, hosts, config and trust (all of v3's `sshmgr`).
- `authama token …` - package-manager and git auth tokens (all of auth-token-manager).

`doctor`, `init`, `migrate`, `history`, `serve`, `tui` and `version` stay at
the top level, because they cover both.

## From sshmgr v3

Nothing you have stops working.

| What | Before | Now |
|---|---|---|
| Binary | `sshmgr` | `authama`; `sshmgr` still ships beside it and is the same program |
| SSH verbs | `sshmgr reconcile`, `sshmgr host add …` | `authama ssh reconcile`, `authama ssh host add …`. The top-level spellings (`authama reconcile`, `sshmgr reconcile`) still work; they are hidden from help |
| Shared verbs | `sshmgr doctor`, `sshmgr serve` | `authama doctor`, `authama serve` |
| `ui` | alias of `serve` | still works, deprecated: use `serve` (web) or `tui` (terminal) |
| Config home | `~/.config/ssh-manager` | unchanged |
| Variables | `SSH_MANAGER_*`, `SSHMGR_DEV_ROOT` | unchanged |
| `~/.ssh` | managed-block markers, `known_hosts` tags | unchanged, so a v3 tree reconciles with no diff |
| `vm` ProxyCommand | `sshmgr vm proxy …` | unchanged; `sshmgr` stays installed for it |
| Go module | `github.com/simtabi/ssh-manager/src/v3` | unchanged |
| Install | `install.sh`, Homebrew, Scoop, winget, deb/rpm/apk | the same packages now install both binaries |

Behaviour changes to know about:

- **`doctor` also checks token files.** It lists the Composer, npm, Docker, git
  and netrc credential files and their modes, `doctor --fix` makes them
  owner-only, and a token file with a loose mode makes `doctor` exit non-zero.
  A CI job that runs `doctor` on a machine with a group-readable `~/.npmrc`
  will now fail where it passed.
- **Messages teach the new spelling** (`authama ssh deploy …`, not
  `sshmgr deploy …`). Scripts that parse message text may need updating.
- **No prefix matching.** Every command name and alias is distinct and must be
  typed in full; a test keeps it that way.

## From auth-token-manager

auth-token-manager was a Python tool driven by flags. Its behaviour is now the
`token` group of a single static binary, ported from
simtabi/auth-token-manager@20dda1e2c56ee23a87c0768edb6fadbd0c298227 with the
same rendering (checked against golden files generated from the Python code)
and all 158 of its pytest cases ported to Go.

### Move your config

```sh
authama token adopt-legacy     # copies it to ~/.config/ssh-manager/tokens.json
```

Until you do, the old locations (`$AUTH_TOKEN_MANAGER_CONFIG`,
`$COMPOSER_AUTH_CONFIG`, `./auth-token-manager.json`,
`~/.config/auth-token-manager.json`) are still read, with a deprecation notice.
The new locations win: `--config`, `$SSH_MANAGER_TOKENS_CONFIG`,
`./authama-tokens.json`, then `tokens.json` in the config home. The file format
is unchanged.

### Move your aliases

```sh
authama token shell-aliases --install    # rewrites the atm block to call authama
```

`atm`, `atm-ui`, `atm-doctor` and `atm-init` keep their names.

### Feature parity

| auth-token-manager | authama | Notes |
|---|---|---|
| `auth-token-manager` (no flags: write every target) | `authama token sync` | |
| `--target T` (repeatable) | `--target T` | on `sync`, `inventory`, `skeleton`, `checkup`, `scaffold` |
| `--config FILE` | `--config FILE` | |
| `--project DIR` | `--project DIR` | |
| `--source FILE` | `--source FILE` | |
| `--env` | `token sync --exports` | |
| `--stdout` | `token sync --preview` | |
| `--no-env-hint` | `--no-env-hint` | |
| `--template` | `token skeleton` | |
| `--init [PATH]` | `token scaffold [FILE]` | default is `tokens.json` in the config home |
| `--services` | `token services` | |
| `--doctor` | `token checkup` | also part of `authama doctor` |
| `--list [--json]` | `token inventory [--json]` | |
| `--list --show-secrets [--yes]` | `token inventory --reveal [--yes]` | |
| `--copy T H` | `token clip T H` | |
| `--add --host H …` | `token store --server H …` | `--username`, `--service`, `--scope` unchanged |
| `--remove T H` | `token forget T H` | |
| `--ui` | `authama tui` → **Auth tokens** | inventory, checkup, clip, store, sync, forget. The **Tokens** page in `authama serve` adds forget and sync with a preview |
| `--ui` edit flow | `token store` for the same host | a store replaces the existing entry |
| `--version` | `authama version` | |
| `auth-token-manager-aliases [status]` | `token shell-aliases` | |
| `… install` / `update` | `token shell-aliases --install` | |
| `… remove` | `token shell-aliases --uninstall` | |
| `… print` | `token shell-aliases --print` | |
| `--file`, `--login`, `--no-backup`, `--quiet` | the same | |
| `--command CMD` | - | dropped: the aliases always call `authama` |
| `.atm.bak` | `.authama.bak` | |
| Docker dev container | `Dockerfile` + `compose.yaml` at the repo root, `make docker-shell` | its home defaults to `./.authama-home`, not your `$HOME` |
| pip / `.pyz` install | a static binary: `install.sh`, `install.ps1`, Homebrew, Scoop, winget, deb/rpm/apk | |
| `tests/test_no_install.py` | not applicable | it checked the Python package ran without `rich`/`questionary` installed; a static Go binary has no optional runtime dependencies to be missing |

### Deliberate differences

Each one fixes something auth-token-manager got wrong or left unsafe:

- **Docker credential stores.** When Docker keeps a registry in `credsStore` or
  `credHelpers`, an inline `auths` entry is never read. auth-token-manager wrote
  one anyway; authama runs `docker login --password-stdin` or refuses with a
  warning, chosen by `--creds-store-mode`, `_settings.docker_creds_store`, or a
  prompt. With no terminal and no setting it refuses. See [Docker](tools/tokens/docker.md).
- **Injection.** `.npmrc`, `.netrc` and the git credential store have no
  quoting, so a newline in a token could plant a second credential. Such values
  are refused before anything is written.
- **Malformed files are refused, not replaced.** A target file that is not
  valid JSON (or a config target that is not a JSON object) stops the run
  instead of being overwritten.
- **Secrets are not echoed.** A Composer write no longer prints `COMPOSER_AUTH`
  with the token in it.
- **Usage errors exit 1**, not argparse's 2.
- **A sandbox.** `--dev-root` runs every token command against a throwaway
  home, with the tools' own path variables hidden.
- **History.** Every token write and removal is recorded in `authama history`
  (without the secret).

## Removing auth-token-manager

Once `authama token inventory` lists what you expect and `token adopt-legacy`
has moved the config:

```sh
pipx uninstall auth-token-manager      # or pip uninstall, or delete the .pyz
authama token shell-aliases --install  # if you used the aliases
```

The credential files themselves need no change: authama reads and merges the
same files.

<!-- nav-footer -->

---

[← Docs index](../README.md#documentation) · [`token`](tools/tokens.md) · [Feature catalog](features.md)
