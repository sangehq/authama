# `token` - package-manager and git auth tokens

`authama token` (also `authama auth`, the same group) writes the credential files Composer, npm, Docker, git and curl
read, lists what is installed, and copies or removes one entry. It is the
auth-token-manager tool, ported into authama (see the
[migration guide](../migration.md)).

The config names **environment variables, never secrets**, so it is safe to
commit. Every write merges into the existing file, atomically, owner-only
(`0600`, or a user-only ACL on Windows).

Every command has a full entry, with all its flags and examples, in the
[command reference](../reference/cli.md#authama-token).

| Command | What it does |
|---|---|
| [`token apply`](../reference/cli.md#authama-token-apply) | Resolve the config from the environment and write each tool's file. The everyday command, and the one CI runs. `--dry-run` shows the result masked and writes nothing; `--exports` prints `COMPOSER_AUTH` for CI. |
| [`token set`](../reference/cli.md#authama-token-set) | Add one token, or replace the one saved for that host: this is also how you rotate or edit a token. The secret comes from stdin or a hidden prompt, never an argument. |
| [`token list`](../reference/cli.md#authama-token-list) | List installed tokens, secrets masked. `--reveal` shows them after asking (`-y`/`--yes` without a terminal); `--json` for scripts. |
| [`token show TARGET HOST`](../reference/cli.md#authama-token-show) | Everything about one entry: tool, host, kind, user, file. Masked unless `--reveal`. |
| [`token status`](../reference/cli.md#authama-token-status) | Which config is in use and why, each tool's file and mode, and which variables `apply` would find unset. Never fails. |
| [`token copy TARGET HOST`](../reference/cli.md#authama-token-copy) | Copy one token to the clipboard without printing it. |
| [`token delete TARGET HOST`](../reference/cli.md#authama-token-delete) | Delete one entry from its tool's file. Asks first at a terminal (`-y` skips). |
| [`token audit`](../reference/cli.md#authama-token-audit) | Check the files are owner-only and a project's `.gitignore` covers them. Writes nothing; exits non-zero when something needs fixing. |
| [`token examples`](../reference/cli.md#authama-token-examples) | Show what each tool's file looks like, with placeholder values (`--json` for the masked JSON). |
| [`token init [FILE]`](../reference/cli.md#authama-token-init) | Write a starter config from the templates, never over an existing file. |
| [`token providers`](../reference/cli.md#authama-token-providers) | List the Composer services a config can name, and the well-known npm, Docker and git hosts. |
| [`token import [FILE]`](../reference/cli.md#authama-token-import) | Copy an auth-token-manager (or any) token config to `tokens.json`, so the old location stops being read. `--dry-run` checks it. |
| [`token shell-aliases`](../reference/cli.md#authama-token-shell-aliases) | Show, install or remove the `atm` shell aliases. |

The targets are `composer`, `npm`, `docker`, `git` and `netrc`; each has a page:
[Composer](tokens/composer.md) · [npm](tokens/npm.md) · [Docker](tokens/docker.md) ·
[git](tokens/git.md) · [netrc](tokens/netrc.md).

## How it works

```
  config (variable names) + environment  →  token apply  →  each tool's own file
  or: token set / the menu  →  pick tool → provider → cloud or self-hosted → type the secret
```

authama writes the token into the file the tool itself reads (Composer
`auth.json`, npm `.npmrc`, Docker `config.json`, `git-credentials`, `.netrc`), so
you never edit those files by hand.

## Everyday use

```sh
authama token init                             # a starter tokens.json in the config home
$EDITOR ~/.config/authama/tokens.json      # keep the hosts you use
authama token status                           # which variables are still missing
GH_TOKEN="$(op read op://Private/GitHub/token)" authama token apply
authama token list
```

One token, typed once:

```sh
authama token set --service github --server github.com          # Composer
authama token set --target npm --server npm.pkg.github.com --scope @acme
authama token set --target git --server github.com --username octocat
printf '%s\n' "$TOKEN" | authama token set --target netrc --server api.heroku.com --username me@example.com
```

Rotate a token by setting it again: `token set` replaces the entry for that host
and keeps every other one. `token show git github.com` confirms what is there.

The menu (`authama tui`, **Auth tokens → Add or replace a token**) asks the same
questions the auth-token-manager menu did: the tool, the provider (from
`token providers`), cloud or self-hosted, then the secret. It also offers
**Replace** for each installed entry, keeping its user name when you press Enter.

## The config

Found in this order, first match wins:

1. `--config FILE`
2. `$AUTHAMA_TOKENS_CONFIG`
3. `./authama-tokens.json` in the current directory
4. `tokens.json` in the config home (`~/.config/authama/tokens.json`)
5. the auth-token-manager locations - `$AUTH_TOKEN_MANAGER_CONFIG`,
   `$COMPOSER_AUTH_CONFIG`, `./auth-token-manager.json`,
   `~/.config/auth-token-manager.json` - read with a deprecation notice
6. the built-in default: Composer for `github.com` from `$GH_TOKEN`

```json
{
  "composer": { "github.com":         { "service": "github", "env": "GH_TOKEN" } },
  "npm":      { "registry.npmjs.org": { "env": "NPM_TOKEN" } },
  "docker":   { "ghcr.io":            { "user_env": "GH_USER", "pass_env": "GH_TOKEN" } },
  "git":      { "github.com":         { "user_env": "GH_USER", "pass_env": "GH_TOKEN" } },
  "netrc":    { "api.heroku.com":     { "user_env": "HEROKU_USER", "pass_env": "HEROKU_API_KEY" } }
}
```

| Entry kind | Fields | Used by |
|---|---|---|
| token | `env` | Composer token mechanisms, npm |
| http-basic | `user_env`, `pass_env` | Composer http-basic, docker, git, netrc |
| bitbucket | `key_env`, `secret_env` | Composer bitbucket |
| custom-headers | `headers` (list, `${VAR}` allowed) | Composer custom-headers |

npm entries also take `scope` (`@your-org`) and `always_auth`. Keys that start
with `_` (such as `_help`) are ignored. An entry whose variable is unset is
skipped with a warning and the rest still render; a variable that is set but
empty substitutes as empty. A bare Composer-only map (`{"github.com": {...}}` or
`{"credentials": {...}}`) is read as the `composer` target. A target whose value
is not a JSON object is refused rather than ignored. A value that would break its
file's line format - a newline in any secret, whitespace in a netrc login or npm
token, `@` in a git host - is refused before anything is written.

`_settings.docker_creds_store` (`login` or `refuse`) sets what happens when
Docker keeps a registry in a credential store; see [Docker](tokens/docker.md).

## Templates

`token init` writes the combined starter config; `token init --target npm`
(repeatable) starts from one tool's template only. `token examples` shows the
files those templates render, with placeholders. Each template carries a `_help`
line describing its fields; delete it, and the hosts you do not use, once you
have edited the file. A test keeps the combined template equal to the union of
the per-tool ones.

### A custom catalog

The Composer services, provider shortcuts and enterprise hosts come from a
catalog built into the binary (`authama token providers` lists it). To add a
service without waiting for a release, copy
[`catalog.json`](../../src/internal/core/tokens/catalog.json), edit it, and
point `$AUTHAMA_TOKENS_CATALOG` at it. auth-token-manager's
`$AUTH_TOKEN_MANAGER_CATALOG` is still read, with a deprecation notice. A
variable naming a file that is not there warns and uses the built-in catalog;
a file that does not parse, or names a target other than composer, npm,
docker, git and netrc, stops the command.

`--source FILE` loads `VAR=value` lines first (variables already set win).
`--target` limits a run to some tools; `--project DIR` writes a project's
`auth.json` and `.npmrc` instead of the user ones (docker, git and netrc are
always per-user).

## Where each file goes

Each file is found the way its own tool finds it, honouring the tool's own
variables:

| Target | Resolution, first match wins |
|---|---|
| composer | `$COMPOSER_HOME` → Windows `%APPDATA%\Composer` → `~/.composer` if present → `$XDG_CONFIG_HOME/composer` (`~/.config/composer`). `--project DIR` → `DIR/auth.json`. |
| npm | `$NPM_CONFIG_USERCONFIG` → `~/.npmrc`. `--project DIR` → `DIR/.npmrc`. |
| docker | `$DOCKER_CONFIG/config.json` → `~/.docker/config.json` |
| git | `~/.git-credentials` → `$XDG_CONFIG_HOME/git/credentials`, whichever already exists |
| netrc | `$NETRC` → `~/.netrc` (`~/_netrc` on Windows) |

## Try it without touching your real files

`--dev-root DIR` (or `$AUTHAMA_DEV_ROOT`) puts every target file under
`DIR/home`, hides `COMPOSER_HOME`, `NPM_CONFIG_USERCONFIG`, `DOCKER_CONFIG`,
`NETRC`, `XDG_CONFIG_HOME` and `APPDATA` so nothing can redirect a write, and
refuses any write outside the sandbox. `docker login` is refused there too,
because it would reach the real Docker credential store.

```sh
authama --dev-root /tmp/sandbox token set --target git --server github.com --username me
authama --dev-root /tmp/sandbox token list
```

## Self-hosted and enterprise hosts

Use your own host as the key:

```json
{ "composer": { "git.acme.corp": { "service": "gitlab", "env": "ACME_GITLAB_TOKEN" } } }
```

Composer only sends a GitHub or GitLab token to a self-hosted host listed in the
project's `composer.json`; the menu reminds you after saving one:

```json
{ "config": { "gitlab-domains": ["git.acme.corp"] } }
```

## Where tokens come from

```sh
export GH_TOKEN=changeme                                      # plain shell
authama token apply --source .env                            # VAR=value file (set variables win)
export GH_TOKEN="$(op read op://Dev/GitHub/token)"           # 1Password
export GH_TOKEN="$(security find-generic-password -s gh -w)" # macOS Keychain
```

Values are trimmed of a trailing newline or spaces, a common cause of "valid
token, still 401".

## CI

```yaml
# GitHub Actions - Composer through the environment, no file to go stale
- run: eval "$(authama token apply --target composer --exports)"
  env: { GH_TOKEN: "${{ secrets.COMPOSER_TOKEN }}" }
- run: composer install --no-interaction
```

```sh
NPM_TOKEN="$TOKEN" authama token apply --target npm --project . # a project .npmrc
npm ci
```

`--exports` prints `COMPOSER_AUTH` as one shell export instead of writing
`auth.json`; the other targets have no environment channel and say so.
`--dry-run` prints what would be written as JSON, secrets masked, and writes
nothing. Do not leave a stale `auth.json` next to `COMPOSER_AUTH`: the file wins,
and `token audit` notes it.

Keep project credential files out of git:

```sh
printf 'auth.json\n.npmrc\n' >> .gitignore
authama token audit --project .     # fails if .gitignore misses one
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `HTTP 401` despite a token | A stale `auth.json` overrides `COMPOSER_AUTH`: `authama token audit`, then `token apply` again or delete the file. |
| "required env var(s) not set" | The variables your config names are not set: export them, or `token apply --source .env`. `token status` lists every missing one. |
| Token "valid" but rejected | A trailing newline or space in the value; authama trims it, but check where the value came from. |
| Private GitLab or GitHub Enterprise token ignored by Composer | Add the host to `gitlab-domains` / `github-domains` in `composer.json`. |
| Forgot what is stored where | `authama token list` (every token with its file, masked) or `token show TARGET HOST`. |
| `token copy` fails in a container or over SSH | There is no clipboard there: use `token show --reveal` or run `copy` on your desktop. |
| A file is world/group-readable | `authama doctor --fix` (or `chmod 600`). |

## Shell aliases

```sh
authama token shell-aliases               # status
authama token shell-aliases --install     # add or update the block in your rc file
authama token shell-aliases --uninstall
eval "$(authama token shell-aliases --print)"
```

| Alias | Runs |
|---|---|
| `atm` | `authama token` |
| `atm-ui` | `authama tui` |
| `atm-doctor` | `authama token audit` |
| `atm-init` | `authama token init` |

The block goes in your interactive rc file (`~/.zshrc`, `~/.bashrc`, else
`~/.profile`); `--login` uses `~/.zprofile` / `~/.bash_profile` instead, and
`--file PATH` any file. bash on macOS opens login shells, which read
`~/.bash_profile` but not `~/.bashrc`: install with `--login`, or source
`~/.bashrc` from `~/.bash_profile`. An auth-token-manager block is replaced, not
duplicated. The rc file is copied to `<file>.authama.bak` first unless `--no-backup`.

## Also in

- [`doctor`](doctor.md) checks the token files' modes (a warning; `--strict` fails on it), and `doctor --fix` repairs them.
- [`tui`](tui.md) has an **Auth tokens** group: list, status, audit, copy, add or replace, apply, delete.
- [`serve`](serve.md) has a **Tokens** page: the list, the status and audit, and previewed `delete` and `apply`.

<!-- nav-footer -->

---

[← Docs index](../../README.md#documentation) · [Migration from auth-token-manager](../migration.md) · [Configuration](../configuration.md)
