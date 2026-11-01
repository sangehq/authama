# `token` - package-manager and git auth tokens

`authama token` writes the credential files Composer, npm, Docker, git and curl
read, lists what is installed, and copies or removes one entry. It is the
auth-token-manager tool, ported into authama (see the
[migration guide](../migration.md)).

The config names **environment variables, never secrets**, so it is safe to
commit. Every write merges into the existing file, atomically, owner-only
(`0600`, or a user-only ACL on Windows).

| Command | What it does |
|---|---|
| `token sync` | Resolve the config from the environment and write each tool's file. The everyday command, and the one CI runs. |
| `token store` | Save one token. The secret comes from stdin or a hidden prompt, never an argument. |
| `token inventory` | List installed tokens, secrets masked. `--reveal` shows them after asking (`--yes` without a terminal); `--json` for scripts. |
| `token clip TARGET HOST` | Copy one token to the clipboard without printing it. |
| `token forget TARGET HOST` | Delete one entry from its tool's file. |
| `token checkup` | Check the files are owner-only and a project's `.gitignore` covers them. Writes nothing. |
| `token skeleton` | Show what each tool's file looks like, with placeholder values. |
| `token scaffold [FILE]` | Write a starter config, never over an existing file. |
| `token services` | List the Composer services a config can name. |
| `token adopt-legacy` | Copy an auth-token-manager config to `tokens.json`, so the old locations stop being read. |
| `token shell-aliases` | Show, install or remove the `atm` shell aliases. |

The targets are `composer`, `npm`, `docker`, `git` and `netrc`; each has a page:
[Composer](tokens/composer.md) · [npm](tokens/npm.md) · [Docker](tokens/docker.md) ·
[git](tokens/git.md) · [netrc](tokens/netrc.md).

## Everyday use

```sh
authama token scaffold                         # a starter tokens.json in the config home
$EDITOR ~/.config/ssh-manager/tokens.json      # keep the hosts you use
GH_TOKEN=$(op read op://Private/GitHub/token) authama token sync
authama token inventory
```

One token, typed once:

```sh
authama token store --service github --server github.com          # Composer
authama token store --target npm --server npm.pkg.github.com --scope @acme
authama token store --target git --server github.com --username octocat
printf '%s\n' "$TOKEN" | authama token store --target netrc --server api.heroku.com --username me@example.com
```

## The config

Found in this order, first match wins:

1. `--config FILE`
2. `$SSH_MANAGER_TOKENS_CONFIG`
3. `./authama-tokens.json` in the current directory
4. `tokens.json` in the config home (`~/.config/ssh-manager/tokens.json`)
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

`--dev-root DIR` (or `$SSHMGR_DEV_ROOT`) puts every target file under
`DIR/home`, hides `COMPOSER_HOME`, `NPM_CONFIG_USERCONFIG`, `DOCKER_CONFIG`,
`NETRC`, `XDG_CONFIG_HOME` and `APPDATA` so nothing can redirect a write, and
refuses any write outside the sandbox. `docker login` is refused there too,
because it would reach the real Docker credential store.

```sh
authama --dev-root /tmp/sandbox token store --target git --server github.com --username me
authama --dev-root /tmp/sandbox token inventory
```

## CI

```sh
eval "$(GH_TOKEN=$TOKEN authama token sync --target composer --exports)"   # COMPOSER_AUTH, no file
NPM_TOKEN=$TOKEN authama token sync --target npm --project .
```

`--exports` prints `COMPOSER_AUTH` as one shell export instead of writing
`auth.json`; the other targets have no environment channel and say so.
`--preview` prints what would be written as JSON, secrets masked, and writes
nothing.

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
| `atm-doctor` | `authama token checkup` |
| `atm-init` | `authama token scaffold` |

The rc file is copied to `<file>.authama.bak` first unless `--no-backup`.

## Also in

- [`doctor`](doctor.md) checks the token files' modes, and `doctor --fix` repairs them.
- [`tui`](tui.md) has an **Auth tokens** group: inventory, checkup, clip, store, sync, forget.
- [`serve`](serve.md) has a **Tokens** page: the inventory, the checkup, and previewed `forget` and `sync`.

<!-- nav-footer -->

---

[← Docs index](../../README.md#documentation) · [Migration from auth-token-manager](../migration.md) · [Configuration](../configuration.md)
