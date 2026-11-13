# Migration guide

authama (Authentication Access Manager) is ssh-manager v3 with
auth-token-manager merged in. It has two features, under two groups:

- `authama ssh …` - SSH keys, profiles, hosts, config and trust (all of v3's `sshmgr`).
- `authama token …` - package-manager and git auth tokens (all of auth-token-manager).

`doctor`, `init`, `migrate`, `settings`, `history`, `serve` and
`version` stay at the top level, because they cover both.

> The versions 0.1.1 to 0.1.4 named below are builds released before 2026-11-06, when the
> history was folded into one commit and the version restarted at v0.1.0. Their releases are
> withdrawn, but installs of them exist, so the steps for upgrading from them stay.

## Removing the sshmgr alias

The `sshmgr` binary that shipped beside `authama` as a deprecated alias is no longer built, released
or installed: not by `make build`, `install.sh`, `install.ps1`, the archives, the Linux packages,
the Homebrew cask or Scoop. The `simtabi/sshmgr` Scoop manifest stops updating. Nothing about your
config, keys or `~/.ssh` changes, and everything older releases wrote under the old name (managed
blocks, `known_hosts` tags, homes, variables) is still recognized.

What could still call `sshmgr`, and what to do once:

```sh
authama ssh config render        # rewrites a VM ProxyCommand that still names sshmgr
authama ssh notify install       # only if you use expiry reminders: points the job at authama
authama doctor                   # reports any scheduled job whose binary has gone
command -v sshmgr && rm "$(command -v sshmgr)"   # an old copy left on PATH, if you want it gone
```

Then replace `sshmgr` with `authama` in scripts and aliases. The installers leave an `sshmgr` they
find in place and print the command that removes it.

## To 0.1.2: authama everywhere, sshmgr deprecated

0.1.2 moves the last places that still said `sshmgr` - release assets, packages, the Homebrew cask
and the Scoop manifest - to `authama`. Your config, keys and `~/.ssh` need nothing.

| What | 0.1.1 and earlier | 0.1.2 |
|---|---|---|
| `sshmgr` binary | the same program | still installed, still runs every command, but prints a one-line deprecation notice on stderr (not for `vm proxy` or completion); since [removed](#removing-the-sshmgr-alias) |
| Release assets | `sshmgr_<os>_<arch>.tar.gz`/`.zip`, `sshmgr_linux_<arch>.{deb,rpm,apk}` | `authama_*` (the bare `sshmgr_<os>_<arch>` binaries are still attached) |
| Homebrew | `brew install --cask simtabi/tap/sshmgr` | `brew install --cask simtabi/tap/authama`; the tap renamed `sshmgr` to `authama` (both since withdrawn, see [installation](installation.md)) |
| Scoop | `simtabi/sshmgr` | `simtabi/authama`; `simtabi/sshmgr` was published, deprecated (both since withdrawn) |
| deb / rpm / apk | package `sshmgr` | package `authama`, which replaces `sshmgr` |
| `known_hosts` tag | `sshmgr` | `authama`; old `sshmgr` tags are still recognized |
| Expiry reminder job | `ssh_manager.expiry` | `authama.expiry`; `notify install` removes the old job |
| Remote lock and temp files on servers | `.ssh-manager.lock`, `authorized_keys.ssh-manager.*` | `.authama.lock`, `authorized_keys.authama.*` |
| VM `ProxyCommand` | the binary that rendered it | always `authama` |

To move over:

```sh
# Homebrew: the rename is picked up on update; if it is not, swap the casks
brew update && brew upgrade
brew list --cask sshmgr >/dev/null 2>&1 && brew uninstall --cask sshmgr && brew install --cask simtabi/tap/authama

# Scoop
scoop uninstall sshmgr; scoop install simtabi/authama

# deb / rpm / apk: install the authama package; it replaces sshmgr
sudo apt install ./authama_linux_amd64.deb

# install.sh / install.ps1 / go install: nothing changes, they install authama

# then, once:
authama ssh config render        # rewrites VM ProxyCommand lines to authama
authama ssh notify install       # only if you use expiry reminders: moves the job to its new name
authama doctor
```

Then replace `sshmgr` with `authama` in scripts and aliases, and put `ssh` before an SSH verb
(`authama ssh <verb>`): the top-level spellings were removed on 2026-11-07. Deploy from 0.1.2 on every machine that
writes to the same server, since the old and new remote lock names do not exclude each other.

## To 0.1.1: the authama config home and AUTHAMA_* names

0.1.1 finishes the rename. What changes, and what happens on its own:

| What | 0.1.0 and earlier | 0.1.1 |
|---|---|---|
| Config home | `~/.config/ssh-manager` (`%APPDATA%\ssh-manager` on Windows) | `~/.config/authama` on every OS (`%USERPROFILE%\.config\authama` on Windows); `$AUTHAMA_CONFIG_DIR`, else `$XDG_CONFIG_HOME/authama`, overrides it |
| State | inside the config home | `~/.local/state/authama` (`$AUTHAMA_STATE_DIR`, else `$XDG_STATE_HOME/authama`): `log/`, `snapshots/`, `dist/` |
| `.env` | read only if your shell exported it (`set -a; . .env; set +a`) | loaded at startup; the environment still wins; `--no-dotenv` / `AUTHAMA_NO_DOTENV=1` skips it |
| Settings file | none | optional `authama.json`, schema-validated (see [Configuration](configuration.md#settings-env-and-authamajson)) |
| Variables | `SSH_MANAGER_*`, `SSHMGR_DEV_ROOT`, `SSHMGR_GIT_MODE` | `AUTHAMA_*`. The old names were read with a deprecation notice until 2026-11-07, and are ignored now |
| Managed block in `~/.ssh/config` | `# Managed by ssh-manager …` | `# Managed by authama …`; the old markers are recognised and rewritten on the next render |
| Bundles | `ssh-manager-<stamp>.age` | `authama-<stamp>.age`; `ssh restore` still reads the old names |
| Provider key titles | `ssh-manager <file>` | `authama <file>`; a re-deploy renames the old title |
| Windows install folder | `%LOCALAPPDATA%\Programs\ssh-manager` | `%LOCALAPPDATA%\Programs\authama`; `install.ps1` takes the old folder off `PATH` and leaves its files |

**The first run moves you over.** When no config dir is set explicitly and
`~/.config/authama` does not exist, the first command (anything but `help`,
`version` or `completion`) copies the old home - `~/.config/ssh-manager`, on
Windows also `%APPDATA%\ssh-manager`, or an older `~/.config/sshmgr` or
`~/.sshmgr` - into `~/.config/authama`, moves `log/`, `snapshots/` and `dist/`
into the state home, writes `MIGRATED-TO-AUTHAMA.txt` into the old directory and
says so once on stderr. The old directory is kept, so 0.1.0 still runs against
it if you go back. A directory carrying the marker is never migrated again.

If you set `SSH_MANAGER_HOME` yourself, rename it to `AUTHAMA_CONFIG_DIR` first:
authama no longer reads the old name, and would otherwise use (and fill) the
default home instead. `authama migrate` does the copy on demand, and
`authama migrate --force` replaces a home that already exists (it is backed up
aside first).

After upgrading:

```sh
authama settings path              # where everything is now
authama settings show --effective  # every setting and where it came from
authama doctor
```

Then rename `SSH_MANAGER_*` / `SSHMGR_*` in your shell profile and `.env`
(`SSH_MANAGER_HOME` → `AUTHAMA_CONFIG_DIR`, `SSHMGR_DEV_ROOT` →
`AUTHAMA_DEV_ROOT`, `SSH_MANAGER_<NAME>` → `AUTHAMA_<NAME>`): the old names are
ignored, so a value left under one has no effect. Drop any
`set -a; . ~/.config/ssh-manager/.env; set +a` line - the file is loaded for you.

## From sshmgr v3

Nothing you have stops working.

| What | Before | Now |
|---|---|---|
| Binary | `sshmgr` | `authama`; `sshmgr` shipped beside it as a deprecated alias from [0.1.2](#to-012-authama-everywhere-sshmgr-deprecated) until it was [removed](#removing-the-sshmgr-alias) |
| SSH verbs | at the top level: `reconcile`, `host add …` | `authama ssh reconcile`, `authama ssh host add …`. The top-level spellings were removed on 2026-11-07, except the hidden `vm` and `audit`, which older `~/.ssh/config` files and scheduled jobs run |
| Shared verbs | `sshmgr doctor`, `sshmgr serve` | `authama doctor`, `authama serve` |
| `ui` | alias of `serve` | removed on 2026-11-07: use `serve`, or a bare `authama` |
| Config home | `~/.config/ssh-manager` | `~/.config/authama` from 0.1.1, copied in on the first run ([above](#to-011-the-authama-config-home-and-authama_-names)) |
| Variables | `SSH_MANAGER_*`, `SSHMGR_DEV_ROOT` | `AUTHAMA_*` from 0.1.1; the old names are ignored since 2026-11-07 |
| `~/.ssh` | managed-block markers, `known_hosts` tags | the managed block's markers are rewritten under the authama name on the next render, with no other diff; new `known_hosts` pins are tagged `authama` from 0.1.2, and `sshmgr`-tagged ones are still recognized |
| `vm` ProxyCommand | `sshmgr vm proxy …` | `authama ssh config render` points it at `authama`; run it before removing an old `sshmgr` |
| Repository and Go module | `github.com/simtabi/ssh-manager`, module `github.com/simtabi/ssh-manager/src/v3` | `github.com/simtabi/authama`, module `github.com/simtabi/authama/src` (v3.6.1 alone was published as `.../authama/src/v3`). GitHub redirects the old repository URLs, so old clones and the old `install.sh`/`install.ps1` URLs keep working |
| Version | 3.6.1 | **0.1.0**: authama restarted its version numbers. The 3.x tags and GitHub releases were removed; their history is in the [CHANGELOG](../CHANGELOG.md#pre-010-history-formerly-ssh-manager-1x361-and-auth-token-manager-0x). A package manager may treat 0.1.0 as older than 3.6.x; see below |
| Install | `install.sh`, `install.ps1`, release archives | binaries ship on the GitHub release only, from v0.1.6 on, or authama is built from source ([installation](installation.md)). The installers, archives and packages of the withdrawn 0.1.x builds carried both binaries until the `sshmgr` alias was [removed](#removing-the-sshmgr-alias); those channels are frozen ([earlier binary releases](installation.md#earlier-binary-releases)) |

Because 0.1.0 sorts below 3.6.x, a package manager that only moves forward
may leave you on 3.6.x. Remove that install, then build authama from source
([installation](installation.md)):

```sh
brew uninstall --cask authama sshmgr 2>/dev/null   # Homebrew
scoop uninstall authama sshmgr                     # Scoop
authama version                                     # after `make install`: the build you made
```

v0.1.6, the first release since the history restarted, is on the GitHub release only. The Homebrew cask
and the Scoop manifest were withdrawn with the 0.1.x assets they named, and stay frozen.

Behaviour changes to know about:

- **`doctor` also checks token files.** It lists the Composer, npm, Docker, git
  and netrc credential files and their modes, and `doctor --fix` makes them
  owner-only. A token file with a loose mode is a warning, so `doctor` still
  exits 0 where it did on v3; `doctor --strict` (the CI mode) fails on it.
- **`--json` output is unchanged.** `doctor --json` keeps every v3 key in the
  same order and adds one, `tokens`; `history --json` records commands as v3
  did. A test pins both.
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
authama token import --dry-run  # parses it and says where it would go
authama token import            # copies it to ~/.config/authama/tokens.json
```

Until you do, the old locations (`$AUTH_TOKEN_MANAGER_CONFIG`,
`$COMPOSER_AUTH_CONFIG`, `./auth-token-manager.json`,
`~/.config/auth-token-manager.json`) are still read, with a deprecation notice.
The new locations win: `--config`, `$AUTHAMA_TOKENS_CONFIG`,
`./authama-tokens.json`, then `tokens.json` in the config home. The file format
is unchanged.

### Move your aliases

```sh
authama token shell-aliases --install    # rewrites the atm block to call authama
```

`atm`, `atm-ui`, `atm-doctor` and `atm-init` keep their names.

### Feature parity

Every auth-token-manager flag, behaviour, file and test, with where it lives now
and what proves it. Tests are Go test names (`go test ./...` from `src/`); `E`
marks the end-to-end run (`make e2e`). The two tools were also run side by side
in throwaway homes against the all-targets template: the five files `token
apply` writes are byte-identical to the Python tool's, on the first run, on a
re-run, after an add and after a remove; so are `--project` writes, `--stdout`
JSON, `--env` exports, `--init` (apart from the `_help` text naming the new
commands) and `--list --json` (with and without secrets).

#### Commands and flags

| auth-token-manager | authama | Proof |
|---|---|---|
| `auth-token-manager` (no flags: write every target) | `token apply` | `TestRendersMatchThePythonTool`, `TestTokenApplyWritesProjectTargets`, E |
| `--target T` (repeatable) | `--target T` on `apply`, `list`, `audit`, `examples`, `init` | `TestTokenApplyPreviewHonoursTarget`, `TestTokenListEmptyAndFiltered` |
| `--config FILE` | `--config FILE` on `apply`, `audit`, `status` | `TestConfigResolutionMatchesThePythonTool` |
| `--project DIR` | `--project DIR` on `apply`, `set`, `list`, `show`, `status`, `copy`, `delete`, `audit`, `examples` | `TestTokenApplyWritesProjectTargets`, `TestTokenExamplesProjectWritesOnlyProjectTargets` |
| `--source FILE` | `--source FILE` | `TestTokenApplySourceLoadsTheEnvFile` |
| `--env` | `token apply --exports` | `TestTokenApplyExportIsOneUnwrappedLine`, `TestEnvExportShellVariants` |
| `--stdout` | `token apply --dry-run` | `TestRedactedJSONHidesSecretsAndFiltersTargets` |
| `--no-env-hint` | `--no-env-hint` | `TestTokenApplyWritesProjectTargets` |
| `--template` | `token examples` (`--json` for the masked JSON) | `TestTokenExamplesPrintsAndWritesNothing`, `TestTokenExamplesPreviewIsRedacted` |
| `--init [PATH]` | `token init [FILE]` | `TestTokenInitNeverOverwrites`, `TestAStarterConfigHasHelpAndTheChosenTargets` |
| `--services` | `token providers` (adds the npm, Docker and git hosts) | `TestTokenProviders`, `TestTokenProvidersListsServicesAndHosts` |
| `--doctor` | `token audit`; also part of `authama doctor` | `TestTokenAudit`, `TestDoctorChecksAndFixesTokenFilePerms` |
| `--list [--json]` | `token list [--json]` | `TestScansMatchThePythonTool`, `TestTokenListJSON`, E |
| `--list --show-secrets [--yes]` | `token list --reveal [-y]` | `TestTokenListRevealNeedsYesWithoutATerminal`, `TestTokenListRevealAsksAtATerminal` |
| `--copy T H` | `token copy T H` | `TestTokenCopyCopiesWithoutPrinting` |
| `--add --host H …` | `token set --server H …` (`--username`, `--service`, `--scope`, `--target` unchanged) | `TestTokenSetGitFromStdin`, `TestTokenSetComposerViaService`, `TestTokenSetNPMWithScope`, `TestTokenSetRefusesIncompleteRequests` |
| `--remove T H` | `token delete T H` (asks first at a terminal; `-y`) | `TestPurgesMatchThePythonTool`, `TestTokenDeleteRefusals`, `TestTokenDeleteAsksAtATerminal`, E |
| `--ui` | the **Auth tokens** page of `authama serve` (the terminal menu that also covered it was [removed](tools/tui.md)) | `TestEveryWebChangeMatchesItsCommand` |
| `--ui` add: provider → cloud or self-hosted → URL → secret | `token set` on the CLI; the Auth tokens page in the web UI | none since the menu was [removed](tools/tui.md) |
| `--ui` Bitbucket key + secret, custom headers | `token set` | none since the menu was [removed](tools/tui.md) |
| `--ui` edit | `token set` replaces the entry for that host | none since the menu was [removed](tools/tui.md) |
| `--ui` self-hosted `*-domains` reminder | was printed by the removed terminal menu | none since the menu was [removed](tools/tui.md) |
| `--ui` list "reveal?" question | lists stay masked; `token show --reveal` / `token list --reveal` ask instead | none since the menu was [removed](tools/tui.md) |
| `--version` | `authama version` | `TestVersionCommand` |
| `auth-token-manager-aliases [status]` | `token shell-aliases` | `TestTokenShellAliasesInstallRemoveStatus` |
| `… install` / `update` | `token shell-aliases --install` | same |
| `… remove` | `token shell-aliases --uninstall` | same |
| `… print` | `token shell-aliases --print` | `TestTokenShellAliasesPrint` |
| aliases `--file`, `--login`, `--no-backup`, `--quiet` | the same flags | `TestTheDefaultProfileIsTheRcFile`, `TestAliasBlockTransforms` |
| aliases `--command CMD` | dropped | the aliases always call `authama` |

#### Data, files and environment

| auth-token-manager | authama | Proof |
|---|---|---|
| `config/catalog.json` (targets, menu, services, enterprise hosts, providers, fields) | embedded unchanged as `internal/core/tokens/catalog.json` | `TestTheCatalogIsConsistent` |
| `config/templates/*.example.json` | embedded under `internal/core/tokens/templates/` | `TestTheAllTargetsTemplateIsTheUnionOfThePerToolOnes`, `TestTheComposerTemplateCoversEveryService` |
| Composer mechanisms (github-oauth, gitlab-token, gitlab-oauth, bitbucket-oauth, http-basic, bearer, forgejo-token, custom-headers) | the same | `TestComposerShapes` |
| `.npmrc` scope and `always_auth` | the same | `TestNPMRCWithAScope` |
| Docker `auths` base64 | the same; a registry in a credential store is logged in or refused | `TestDockerAuthIsBase64`, `TestTokenSetDockerRespectsACredentialStore` |
| git credentials (URL-encoded, scheme kept, XDG fallback) | the same | `TestGitCredentialsEncodeSecretsAndKeepTheScheme`, `TestGitFallsBackToTheXDGFile` |
| `.netrc` / `_netrc` | the same | `TestNetrcLine`, `TestWindowsPaths` |
| Merge into existing files, last write wins | the same | `TestTheLastWriteWins`, `TestPurgesMatchThePythonTool` |
| Whitespace trimmed; empty secrets skipped; `${VAR}` set-but-empty | the same | `TestEmptySecretsAreSkippedAndWhitespaceTrimmed`, `TestExpandEnvPresentButEmpty` |
| Bare Composer config, `credentials` wrapper, `_`-keys ignored | the same | `TestABareConfigIsComposer` |
| Unknown service warns | the same | `TestAnUnknownServiceWarns` |
| `COMPOSER_HOME`, `APPDATA`, `NPM_CONFIG_USERCONFIG`, `DOCKER_CONFIG`, `NETRC`, `XDG_CONFIG_HOME` | honoured the same way | `TestDefaultPathsAndOverrides`, `TestWindowsPaths` |
| `AUTH_TOKEN_MANAGER_CONFIG`, `COMPOSER_AUTH_CONFIG`, `./auth-token-manager.json`, `~/.config/auth-token-manager.json` | still read, with a deprecation notice, after the new locations; `token import` moves the file | `TestTokenApplyReadsALegacyConfigWithANotice`, `TestTokenImportFromALegacyLocationOrAFile` |
| `SHELL` (alias rc file, export syntax) | the same, plus PowerShell and cmd | `TestEnvExportShellVariants`, `TestDetectShellPowerShell` |
| Windows owner-only ACL (`icacls`, `USERNAME`) | the same | `util/perms/windows_acl_test.go`, `TestOwnerOnlyAndPermsOnWindows` |
| `AUTH_TOKEN_MANAGER_CATALOG` (a replacement `catalog.json`) | `AUTHAMA_TOKENS_CATALOG`; the old name is still read, with a deprecation notice | `TestTokenCatalogOverride` |
| `ATM_DEBUG` (show the Python traceback) | dropped | Go errors are already one line; there is no traceback to show |
| 0600 atomic writes | the same (owner-only ACL on Windows) | `TestOwnerOnlyAndPermsOnWindows`, E |

#### Packaging, tests and docs

| auth-token-manager | authama | Proof |
|---|---|---|
| pip / pipx / `.pyz` install | a static binary, downloaded from the GitHub release or built from source with `make build`; the install scripts, archives and Homebrew, Scoop and deb/rpm/apk packages of the withdrawn builds are frozen | [installation](installation.md) |
| Docker dev container | `Dockerfile` + `compose.yaml`, `make docker-shell`; its home defaults to `./.authama-home`, not your `$HOME` | [installation](installation.md#dev-container) |
| Python library API (`render`, `remove_installed`, …) | dropped | authama is a command, not a library; scripts call the CLI with `--json` |
| `tests/test_core.py`, `test_merge.py`, `test_writers.py`, `test_inventory.py` | `core/tokens/port_test.go`, `golden_test.go` (golden files generated from the Python code) | 34 tests |
| `tests/test_cli_actions.py` | `cli/token_test.go` | 36 tests |
| `tests/test_catalog.py`, `test_templates.py` | `port_test.go` catalog and template tests | |
| `tests/test_windows.py` | `port_test.go` Windows tests, `util/perms/windows_acl_test.go`, `tokensvc_test.go`; `install.ps1` is parsed and checked by PSScriptAnalyzer in CI | |
| `tests/test_aliases.py` | `TestAliasBlockTransforms`, `TestTokenShellAliases*` | |
| `tests/test_tui.py` | none: the terminal menu was [removed](tools/tui.md) | |
| `tests/test_no_install.py` | not applicable | it checked the Python package ran without `rich`/`questionary`; a static binary has no optional runtime dependencies |
| `.atm.bak` | `.authama.bak` | `TestTokenShellAliasesInstallRemoveStatus` |
| `docs/usage.md` (workflow, self-hosted, token sources, CI, troubleshooting) | [`token`](tools/tokens.md) | |
| `docs/configuration.md`, `docs/templates.md` | [`token` › The config / Templates](tools/tokens.md#the-config), [configuration](configuration.md#auth-tokens) | |
| `docs/tools/{composer,npm,docker,git,netrc}.md` | [tools/tokens/](tools/tokens/composer.md) | |
| `docs/tools/ui.md` | [`serve`](tools/serve.md) (the terminal menu that first replaced it was [removed](tools/tui.md)) | |
| Make targets `run`, `ui`, `services`, `aliases-*`, `docker-build`/`up`/`check`/`clean`, `venv`, `pyz`, `binary` | dropped | each is one `authama` command (`token apply`, `serve`, `token providers`, `token shell-aliases`); the container is `make docker-shell`/`docker-run`/`docker-down`; there is no venv or zipapp to build |
| `.env.example` `ATM_*` container variables | `AUTHAMA_*` in `compose.yaml` (`ATM_AUTH_HOME` is `AUTHAMA_DEV_HOME`, `ATM_UBUNTU_VERSION` is `AUTHAMA_GO_VERSION`) | [installation](installation.md#dev-container) |
| `docs/aliases.md` | [`token` › Shell aliases](tools/tokens.md#shell-aliases); the Make shortcuts are the repo's `make help` | |
| `docs/container.md` | [installation › Dev container](installation.md#dev-container) | |
| `docs/installation.md`, `release.md`, `shipping-checklist.md`, `architecture.md` | [installation](installation.md), [release](release.md), [architecture](architecture.md) | |
| every flag's `--help` | [command reference](reference/cli.md), generated from the binary | `TestCLIReferenceIsCurrent`, `TestEveryCommandHasExamplesThatParse` |

#### Added beyond auth-token-manager

| Command | Why |
|---|---|
| `token show TARGET HOST [--reveal] [--json]` | Inspect one entry without scanning the whole list. |
| `token status [--json]` | Answers "why did apply skip that?": the config in use and why, each file and its mode, and the variables that are unset. Never fails, unlike `audit`. |
| `token import [FILE] [--dry-run]` | Moves the old config (or any file) into `tokens.json` after parsing it. |
| `token delete` asks at a terminal | Every other delete in authama asks; `-y` skips. |
| `token providers` lists hosts | The npm, Docker and git shortcuts the menu offers were not visible from the CLI. |
| `--dev-root` sandbox | Every token command can run against a throwaway home. |
| History | Every token write and removal is in `authama history`, without the secret. |

Not added, on purpose: online token verification (it would send each secret to
its host from a tool that otherwise never touches the network for tokens); a
bulk secret export (`apply --exports` covers CI, and `copy` / `show --reveal`
cover one secret); `edit` and `rotate` verbs (`token set` replaces an entry, and
the menu's **Replace** keeps its user name).

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
- **Usage errors exit 1**, not argparse's 2, and errors start `authama:`.
- **`token audit` checks all five tools** by default; `--doctor` checked only
  the ones in the config (Composer, with no config). `--target` or `--config`
  narrows it.
- **A sandbox.** `--dev-root` runs every token command against a throwaway
  home, with the tools' own path variables hidden.
- **History.** Every token write and removal is recorded in `authama history`
  (without the secret).

## History audit

Before v3.6.1 was declared complete, every release of both tools was checked
against it, not just the last one, so nothing added and later lost along the
way went unnoticed. (November 2026, on `main` after v3.6.1.)

**What was walked**

- sshmgr / ssh-manager: all 445 commits and every tag (`v0.1.0`, `v1-final`,
  `v2.0.0-rc.1`, `v2.0.0`, `v3.0.0` to `v3.6.1`), their release notes and
  `CHANGELOG.md`. The Python CLI (v0.1.0 to v1-final) was read from source; for
  every Go tag the full command tree (commands, aliases, flags, hidden or not)
  was dumped from the binary and diffed against `main`. Environment variables,
  JSON and YAML config keys, the provider catalog and both installers were
  compared the same way.
- auth-token-manager: all 43 commits on every branch and the v0.3.0 release
  notes. Every `argparse` flag of the CLI and the aliases tool, every
  environment variable it read, every Composer service and provider shortcut
  in every revision of its catalog, every template, the container variables
  and the Make targets were listed and matched to `authama token`.
- The token commands were then run in a throwaway `HOME` (`token init`,
  `apply`, `apply --dry-run`/`--exports`, `set` from stdin, `list [--json]`,
  `delete -y`, `audit`, `status`, `providers`, `shell-aliases --print`): files
  were written owner-only and nothing reached the real home.

**Gaps found and fixed**

| Missing | Since | Now |
|---|---|---|
| `--no-passphrase` on `keygen`, `reconcile`, `rotate` | dropped silently in 2.0.0 (the notes claimed the same command surface) | accepted again, hidden, as the spelling of the default; exclusive with `--passphrase` (`TestNoPassphraseIsAcceptedAndExclusive`, and `v1Surface` in `surface_test.go`) |
| `$AUTH_TOKEN_MANAGER_CATALOG` (use a replacement catalog) | lost in the Go port, which embedded the catalog | `$AUTHAMA_TOKENS_CATALOG`, and the old name with a notice (`TestTokenCatalogOverride`) |

**Confirmed present:** every other command, alias and flag of every Go tag (as
`authama ssh …`), every v1 Python command and flag, every config key, all 25
providers, the installers' old `SSHMGR_*` variables, and every
auth-token-manager flag, alias, service, provider and template in the tables
above.

**Dropped on purpose** (each recorded where it was decided): `knownhosts init
--user` (one trust store, CHANGELOG 3.0.0, `v1Dropped`); the Python
implementation and pip/pipx/zipapp packaging; rich tables (plain text now);
pre-commit hooks and the `.build` harnesses; rendered config inside bundles
(3.4.0); `ui` as a separate command (an alias of `serve`); cosign (see
[release](release.md)); and from auth-token-manager, `ATM_DEBUG`, the Python
library API, the aliases tool's `--command`, and the Make targets listed above.

## Removing auth-token-manager

Once `authama token list` shows what the old tool listed (compare it with
`auth-token-manager --list` while that is still installed) and `token import`
has moved the config. The two list the same JSON, so this prints nothing:

```sh
diff <(auth-token-manager --list --json | jq -S .) <(authama token list --json | jq -S .)
```

Then:

```sh
pipx uninstall auth-token-manager      # or pip uninstall, or delete the .pyz
authama token shell-aliases --install  # if you used the aliases
```

The credential files themselves need no change: authama reads and merges the
same files.

<!-- nav-footer -->

---

[← Docs index](../README.md#documentation) · [`token`](tools/tokens.md) · [Feature catalog](features.md)
