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
| Repository and Go module | `github.com/simtabi/ssh-manager`, module `github.com/simtabi/ssh-manager/src/v3` | `github.com/simtabi/authama`, module `github.com/simtabi/authama/src/v3`. GitHub redirects the old repository URLs, so old clones, release downloads and the old `install.sh`/`install.ps1` URLs keep working; `go install` needs the new path for releases after v3.6.0 |
| Install | `install.sh`, `install.ps1`, release archives | the same installers and archives now carry both binaries (so do the Homebrew, Scoop, winget and deb/rpm/apk packages, once [release](release.md) publishes them) |

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
authama token import            # copies it to ~/.config/ssh-manager/tokens.json
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
| `--ui` | `authama tui` → **Auth tokens**; the **Tokens** page of `authama serve` | `tui_tokens_test.go`, `TestEveryWebChangeMatchesItsCommand` |
| `--ui` add: provider → cloud or self-hosted → URL → secret | **Add or replace a token**, same questions | `TestTheTUIAsksForAHostWhenTheProviderHasNoCloud`, `TestTheTUIRemindsAboutSelfHostedComposerDomains` |
| `--ui` Bitbucket key + secret, custom headers | the same prompts | `TestTheTUIStoresComposerPairsAndHeaders` |
| `--ui` edit | **Replace** an installed entry (Enter keeps the user name); `token set` on the CLI | `TestTheTUIReplacesAnInstalledToken` |
| `--ui` self-hosted `*-domains` reminder | printed after a self-hosted save | `TestTheTUIRemindsAboutSelfHostedComposerDomains` |
| `--ui` list "reveal?" question | the menu list stays masked; `token show --reveal` / `token list --reveal` ask instead | `TestTheTUIStoresATokenInTheSandbox` |
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
| `AUTH_TOKEN_MANAGER_CATALOG` (a replacement `catalog.json`) | `SSH_MANAGER_TOKENS_CATALOG`; the old name is still read, with a deprecation notice | `TestTokenCatalogOverride` |
| `ATM_DEBUG` (show the Python traceback) | dropped | Go errors are already one line; there is no traceback to show |
| 0600 atomic writes | the same (owner-only ACL on Windows) | `TestTheTUIStoresATokenInTheSandbox`, E |

#### Packaging, tests and docs

| auth-token-manager | authama | Proof |
|---|---|---|
| pip / pipx / `.pyz` install | a static binary: `install.sh`, `install.ps1`, `go install`, release archives, Homebrew, Scoop, winget, deb/rpm/apk | [installation](installation.md) |
| Docker dev container | `Dockerfile` + `compose.yaml`, `make docker-shell`; its home defaults to `./.authama-home`, not your `$HOME` | [installation](installation.md#dev-container) |
| Python library API (`render`, `remove_installed`, …) | dropped | authama is a command, not a library; scripts call the CLI with `--json` |
| `tests/test_core.py`, `test_merge.py`, `test_writers.py`, `test_inventory.py` | `core/tokens/port_test.go`, `golden_test.go` (golden files generated from the Python code) | 34 tests |
| `tests/test_cli_actions.py` | `cli/token_test.go` | 36 tests |
| `tests/test_catalog.py`, `test_templates.py` | `port_test.go` catalog and template tests | |
| `tests/test_windows.py` | `port_test.go` Windows tests, `util/perms/windows_acl_test.go`, `tokensvc_test.go`; `install.ps1` is parsed and checked by PSScriptAnalyzer in CI | |
| `tests/test_aliases.py` | `TestAliasBlockTransforms`, `TestTokenShellAliases*` | |
| `tests/test_tui.py` | `cli/tui_tokens_test.go` | 11 tests |
| `tests/test_no_install.py` | not applicable | it checked the Python package ran without `rich`/`questionary`; a static binary has no optional runtime dependencies |
| `.atm.bak` | `.authama.bak` | `TestTokenShellAliasesInstallRemoveStatus` |
| `docs/usage.md` (workflow, self-hosted, token sources, CI, troubleshooting) | [`token`](tools/tokens.md) | |
| `docs/configuration.md`, `docs/templates.md` | [`token` › The config / Templates](tools/tokens.md#the-config), [configuration](configuration.md#auth-tokens) | |
| `docs/tools/{composer,npm,docker,git,netrc}.md` | [tools/tokens/](tools/tokens/composer.md) | |
| `docs/tools/ui.md` | [`tui` › Auth tokens](tools/tui.md#auth-tokens), [`serve`](tools/serve.md) | |
| Make targets `run`, `ui`, `services`, `aliases-*`, `docker-build`/`up`/`check`/`clean`, `venv`, `pyz`, `binary` | dropped | each is one `authama` command (`token apply`, `tui`, `token providers`, `token shell-aliases`); the container is `make docker-shell`/`docker-run`/`docker-down`; there is no venv or zipapp to build |
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
| `$AUTH_TOKEN_MANAGER_CATALOG` (use a replacement catalog) | lost in the Go port, which embedded the catalog | `$SSH_MANAGER_TOKENS_CATALOG`, and the old name with a notice (`TestTokenCatalogOverride`) |

**Confirmed present:** every other command, alias and flag of every Go tag (as
`authama ssh …` or the hidden top-level spelling), every v1 Python command and
flag, every `SSH_MANAGER_*`/`SSHMGR_*` variable, every config key, all 25
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
