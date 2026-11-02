# Configuration

authama's own settings live in two files in the config home, `.env` and
`authama.json`, described [below](#settings-env-and-authamajson). Everything SSH
lives in the **manifest** (`~/.config/authama/manifest.json`) - the single
source of truth. You never hand-edit authama's part of `~/.ssh/config`; you edit the
manifest (or use the `host`/`profile` verbs) and re-render. A JSON Schema for the
manifest and inventory lives in `src/config/schema/` (and matches the Go models in `src/internal/core`,
which reject unknown keys). See [installation.md](installation.md) for how the home
is resolved. Auth tokens have their own config, described [below](#auth-tokens).

## authama owns only a marked block of `~/.ssh/config`

`reconcile` / `config render` rewrite **only** the region between

```
# Managed by authama - do not edit (run: authama ssh config render)
...
# End of authama-managed block - content outside it is preserved
```

A block written by an earlier release (`# Managed by ssh-manager ...` or
`# Managed by sshmgr ...`) is recognised as the managed block, and the next
render rewrites it under the new markers in place.

Anything above or below that block is left untouched, so edits injected by other
tools survive a re-render. In particular, an OrbStack preamble (which must sit at
the very top of `ssh_config`, before any `Host` block) is kept at the top. OrbStack
appends its Include to the *end* of the file; a render moves that one known
`Include ~/.orbstack/ssh/config` (and the comment lines above it) to the top -
no other foreign line is moved:

```
# Added by OrbStack: 'orb' SSH host for Linux machines
Include ~/.orbstack/ssh/config

# Managed by authama - do not edit (run: authama ssh config render)

Match all
Include ~/.ssh/profiles/internal/config   # only when a profile has a category
...
Host github-alice                          # uncategorized profiles render inline
    ...
Host *
    ...
# End of authama-managed block - content outside it is preserved
```

To add your own static SSH directives, put them outside the managed block (above
or below it) - they will be preserved. `~/.ssh/config` is also snapshotted before
every mutating command, so a render is always reversible via `snapshots restore`.

## Multiple identities on one host: prefix aliases to avoid collisions

When the same host serves more than one identity - the classic case is a
**personal vs. work GitHub account**, both on `github.com` - give every host a
**distinct, profile-prefixed alias** rather than using the bare hostname:

```jsonc
"personal": { "hosts": [
  { "alias": "github-personal", "hostname": "github.com", "user": "git",
    "provider": "github", "key_name": "personal_github-ed25519" } ] },
"acme":     { "hosts": [
  { "alias": "github-acme",  "hostname": "github.com", "user": "git",
    "provider": "github", "token_env": "GH_TOKEN_ACME",
    "key_name": "acme_github-ed25519" } ] }
```

Why prefix:

- An `alias` is the `Host` block name and must be unique across the whole config.
  If one identity claims the bare `github.com`, then `git@github.com:...` *always*
  resolves to that identity - the other one can never use a `github.com` remote
  (a silent collision). Distinct prefixed aliases make each identity explicit.
- `key_name` is already profile-prefixed by authama's `<profile>_<service>-<algo>`
  convention (e.g. `personal_github-ed25519`, `acme_github-ed25519`), so the key
  *files* never collide. Apply the same discipline to the **alias** so the SSH
  *Host blocks* don't either.

Point each repo's remote at the right alias (the SSH user stays `git` - GitHub
requires it; it's the *alias* that selects the identity):

```sh
git remote set-url origin git@github-personal:me/my-repo.git
git remote set-url origin git@github-acme:acme/app.git
# or, to rewrite a host globally for one account, use git's insteadOf:
git config --global url."git@github-acme:acme/".insteadOf "git@github.com:acme/"
```

`IdentitiesOnly yes` plus the per-profile `IdentityFile`/`UserKnownHostsFile` then
guarantees each alias offers only its own key and trusts only its own host keys.

> **The SSH `user` for GitHub/GitLab/Bitbucket must be `git`.** The account is
> identified by the **key**, not the SSH username - so a custom `user` (e.g.
> `alice_git`) breaks any plain `git@host:...` remote. Prefix the **alias** to
> separate identities; never prefix the `user`. (For real login servers like a
> VPS or a university host, `user` *is* the real account, e.g. `ploi`, `researcher`.)

## Manifest reference

```jsonc
{
  "version": 1,
  "defaults": { /* see below */ },
  "profiles": {
    "work": {
      "category": "others",               // optional: profiles/others/work/ + profiles/others/config
      "key_store": "file",                // or "1password" (then op_vault is required)
      "op_vault": null,                   // the 1Password vault holding this profile's keys
      "key_scope": "per_service",         // or "shared"
      "key_name": null,                    // set only when key_scope == "shared"
      "key_comment": "alice-{device}-{date}", // optional: overrides defaults.key_comment
      "hosts": [
        {
          "alias": "hpc",                  // the ssh Host alias you type
          "hostname": "hpc.example.edu",    // real host
          "user": "researcher",
          "port": 443,                      // default 22
          "provider": "generic-ssh",       // adapter name from providers.json (optional)
          "token_env": "GH_TOKEN_WORK",    // env var holding this host's provider token (optional)
          "key_name": "work_hpc-ed25519",  // omit on shared-scope profiles
          "tags": ["app"],                  // free-form, used by `list --tag`
          "requires_vpn": false,            // true if this host is only reachable over a VPN
          "vpn_name": null,                 // which VPN (shown in the reachability hint)
          "vpn_url": null,                  // where to connect that VPN (shown in the hint)
          "raw_options": {"ProxyJump": "bastion"}   // any ssh option the schema doesn't model
        }
      ]
    }
  }
}
```

Hosts need not be written by hand: `authama ssh host add|edit` change one field set at a
time, and `authama ssh host apply <profile> <alias> <host>[:port] <user> [role]` (or many
with `--host`/`-f`) adds or updates `alias`, `hostname`, `user`, `port` and `tags` in one
step. See [Host specs](recipes/host-specs.md).

### `defaults`

| Field | Default | Meaning |
|---|---|---|
| `key_type` | `ed25519` | ssh-keygen type for new keys (`ed25519`, `ed25519-sk`, `rsa`, ...) |
| `key_scope` | `per_service` | default scope for profiles that don't set one |
| `rotate_after_days` | `365` | key lifetime → drives `expiry` (ok / due_soon / overdue) |
| `warn_before_days` | `[30,14,7,1]` | when the expiry banner starts warning |
| `expiry_check.enabled` | `true` | run the expiry check at all |
| `expiry_check.debounce_hours` | `24` | min gap between inline banners |
| `expiry_check.desktop_notify` | `true` | allow the scheduled desktop notification |
| `global_options` | `{}` | ssh options emitted under `Host *` (e.g. `AddKeysToAgent`, macOS `UseKeychain`) |
| `key_comment` | `{profile}/{alias} {date}` | comment template for new keys (see [Key comments](#key-comments)) |
| `onepassword_agent` | platform default | the 1Password SSH agent socket for `key_store: 1password` profiles (see [Key stores](#key-stores)) |
| `git.mode` | `profile` | default GitHub remote mode for `authama ssh git use`: `profile`, `ssh` or `https` |
| `git.profile` | unset | the default identity: its GitHub key is bound to a rendered `Host github.com` (ssh mode), and it is used for owners `git.orgs` does not list. Unset, no `Host github.com` is rendered. The block is rendered once: in the profile's category file (`profiles/<category>/config`) when it has a category, otherwise in the root block; skipped if a host is already aliased `github.com` |
| `git.orgs` | `{}` | GitHub owner → profile, for `authama ssh git` (see [tools/git.md](tools/git.md)) |

All three `git` fields are optional and the key is omitted when unset, so a manifest without them renders
and saves exactly as before. `ssh` mode needs `git.profile`, and no host may use the alias `github.com`
while it is set.

### `vm`

A host with `vm` is a local VM. authama renders `ProxyCommand "<authama>" vm proxy <alias>` for it (one of
the two ProxyCommands authama writes, with `proxy` below; `raw_options` still may not set one), and resolves the
VM's address through its hypervisor on each connection.

```jsonc
{"alias": "devbox", "hostname": "devbox", "user": "dev",
 "vm": {"hypervisor": "parallels", "name": "Ubuntu 24.04"}}
```

`hypervisor` is one of `parallels`, `vmpal`, `vmware`, `utm`, `lima` (OrbStack and Podman keep their own
SSH and are not hosts); `name` is the VM as the hypervisor lists it, spaces allowed. See
[tools/vm.md](tools/vm.md).

### `proxy`

A host with `proxy: {"kind": "cloudflared"}` is reached through Cloudflare Access. authama renders
`ProxyCommand "<cloudflared>" access ssh --hostname %h`, so `ssh <alias>` signs in through the browser and
reaches the host with no SSH port open to the internet. `hostname` is the Access hostname.

```jsonc
{"alias": "app-web", "hostname": "ssh.example.com", "user": "admin",
 "proxy": {"kind": "cloudflared"}}
```

Set it with `authama ssh host add|edit ... --proxy cloudflared` (`--proxy none` removes it). A host is a VM or
proxied, not both. `doctor` reports a proxied host when `cloudflared` is not installed, and the Termius
export skips it (Termius ignores ProxyCommand). See
[Reach a server through Cloudflare Access](recipes/cloudflare-access.md).

### `provider` / `token_env`

A host's `provider` names an adapter in `~/.config/authama/providers.json` (GitHub/GitLab,
the cloud-VPS REST adapters, a `kind: rest` provider, `generic-ssh`, or a
web-panel/manual entry). `token_env` overrides which env var that host's adapter
reads for its credential - so several accounts on one provider stay separate. See
[tools/providers.md](tools/providers.md) and [tools/vps.md](tools/vps.md).

Mark a host that's only reachable over a VPN with `requires_vpn: true` (and an optional `vpn_name`); `authama ssh net` and every network action then warn you to connect the VPN instead of hanging. See [tools/network.md](tools/network.md).

## Profiles model identity, not technology - everything lives under the profile

A profile is *who you're being* (work · personal · acme · development ·
school), not *what kind of service* a host is. **Everything for an identity lives
under `~/.ssh/profiles/<profile>/`** (or `profiles/<category>/<profile>/`) - its
keys, and nothing else - and nothing crosses profiles:

- Two GitHub accounts are two profiles sharing `hostname: github.com` via distinct
  aliases (`github.com` vs `github-acme`) and `IdentitiesOnly yes`, so each
  offers only its own key.
- Every host renders `UserKnownHostsFile ~/.ssh/known_hosts` - one hashed store,
  not one per profile. Identity isolation comes from `IdentityFile` plus a
  per-host `IdentitiesOnly yes`, not from where the trust store lives. A
  host key trusted as `personal` is **not** trusted as `acme` - even on the
  same `github.com` host. Pin host keys with `authama ssh knownhosts pin --all`.

## Categories

A profile may set `category` to file it one level deeper: its keys live under
`~/.ssh/profiles/<category>/<profile>/`, and its `Host` blocks render into
`~/.ssh/profiles/<category>/config`, which the root config Includes. Profiles
without one keep the flat layout and stay inline in the root config, so a
manifest that uses no categories renders exactly as before.

| Rule | Why |
|---|---|
| A category is a safe path segment | It becomes a directory name. |
| It may not share a name with an uncategorized profile | Both would own `profiles/<name>/`. |
| A categorized profile may not be named `config` or `old` | `profiles/<category>/config` is the category's file; `.../old/` is a rotation archive. |

Set it with `authama ssh profile add <name> --category <c>` or
`authama ssh profile edit <name> --category <c>`. The edit moves the key directory and
updates the inventory. See [Group profiles into categories](recipes/group-profiles-into-categories.md).

## Key stores

A profile's private keys live in one of two places, set by `key_store`:

| `key_store` | Private key | On disk | Rendered host |
|---|---|---|---|
| `file` (or omitted) | `profiles/…/<key>` | private key + `.pub` | `IdentityFile <key>` |
| `1password` | an SSH Key item titled `<key>` in `op_vault` | `.pub` only | `IdentityFile <key>.pub` + `IdentityAgent "<socket>"` |

With `1password`, `reconcile` creates the key inside 1Password with the
[`op` CLI](https://developer.1password.com/docs/cli/) and writes only its public
half. ssh then asks the 1Password agent for the key matching that `.pub`, so the
private key never touches disk. `rotate` generates replacements in the vault too.
Needs `op` (2.20 or later) signed in, and the agent turned on in the 1Password
app (Settings > Developer > Use the SSH agent); `doctor` checks both.

**The agent must serve the vault.** With no `agent.toml`, the 1Password SSH
agent offers only keys in your Personal, Private or Employee vault. Once
`~/.config/1Password/ssh/agent.toml` exists, it offers only what the file lists
(even an empty file overrides the default). For any other vault, or whenever the
file exists, list the vault as a whole:

```toml
[[ssh-keys]]
vault = "Acme"
```

A whole-vault entry, not one per item: a rotation's `<key>.staged` item has to be
served to be verified. `doctor` checks this for every vault a profile names.

The agent socket defaults to the platform's path (macOS
`~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock`, Linux
`~/.1password/agent.sock`); set `defaults.onepassword_agent` to override it. On
Windows no `IdentityAgent` is rendered, because Microsoft OpenSSH uses one fixed
pipe for every host.

Two rules are enforced when the manifest loads:

- **Key names are unique within a vault.** The item is titled with the key name,
  so two 1Password profiles sharing a vault and a key name would share one key.
- **Keys are `ed25519`.** 1Password generates nothing else, so another
  `key_type` on a 1Password profile is rejected rather than silently ignored.

The store is per profile, not per host, so the hosts sharing a key cannot
disagree about where it lives. To move a profile into 1Password, set
`key_store` and run `authama ssh rotate` on each key: it generates the new key in the
vault and archives the old key file to `old/`. See
[Keep keys in 1Password](recipes/keep-keys-in-1password.md).

## Key scope

- `per_service` *(default)* - one key per host: smallest blast radius.
- `shared` - one key for a whole profile (set `key_name` on the profile, omit it
  on hosts). Opt in per profile where you accept the trade-off.

## Key naming - `<profile>_<service>-<algo>`

Exactly one underscore (the profile prefix); the remainder is kebab-case. Names
are **stable** (never carry a date - that lives in the key comment + inventory).
Hardware keys end `-ed25519-sk`.

When a key's type is a hardware one (`ed25519-sk`, `ecdsa-sk`) and no security key answers, authama mints the
software equivalent (`ed25519`) at the same path rather than failing, so a machine without the device can still
connect. It says so in three places: the key's comment ends `(sk-fallback)`, the inventory records the software
type, and `reconcile` or `rotate` prints a line naming the key. Insert the security key and `authama ssh rotate <key>`
to replace it with a hardware one.

## Key comments

The comment is the free text after the key in its `.pub` (`ssh-keygen -l` prints it, and servers and panels
show it next to the key). authama writes it once, when it mints the key, from a template: the profile's
`key_comment`, else `defaults.key_comment`, else `{profile}/{alias} {date}`, which is what every release before
the field wrote.

| Placeholder | Expands to |
|---|---|
| `{profile}` | the profile name |
| `{alias}` | the first host the key serves, or the key name for a key no host uses yet |
| `{key}` | the key name, e.g. `work_hpc-ed25519` |
| `{date}` | the day the key is minted, `YYYY-MM-DD` |
| `{device}` | this machine's short host name, lowercased (`mbp.local` → `mbp`) |

```sh
authama ssh profile add app --key-comment 'alice-{device}-{date}'   # alice-mbp-2026-10-27
authama ssh profile edit app --key-comment ''                         # back to defaults.key_comment
```

A changed template applies to keys minted from then on, by `reconcile`, `key add` or `rotate`; an existing key
keeps the comment it was made with. An unknown placeholder or a control character is refused when the manifest
loads. A key in 1Password carries no comment from authama: 1Password generates it.

## Unmodeled ssh options - `raw_options`

For an option the schema doesn't model, add a host-level `raw_options` block
rather than hand-editing the file:

```json
{"alias": "hpc", "hostname": "hpc.example.edu", "user": "researcher", "port": 443,
 "key_name": "work_hpc-ed25519",
 "raw_options": {"ProxyJump": "bastion.hpc.edu"}}
```

Because the manifest is rendered verbatim into `~/.ssh/config` and its names become
filesystem paths, values are validated on load: names (profile/alias/key_name) may
not contain `/`, `\`, `..`, a leading `-`, whitespace, glob `*?`, or control
characters; `hostname`/`user` may not contain whitespace, newlines, or a leading
`-`; and `raw_options` (and `global_options`) reject command/code-executing keys
(`ProxyCommand`, `LocalCommand`, `PermitLocalCommand`, `RemoteCommand`, `Match`,
`Include`, `KnownHostsCommand`, `PKCS11Provider`, `SecurityKeyProvider`).
`ProxyJump` (a host, not a command) is allowed. `key_scope` must be `per_service`
or `shared`.

## <a name="settings-env-and-authamajson"></a>Settings: `.env` and `authama.json`

### Where things live

| | Default | Override |
|---|---|---|
| Config home | `~/.config/authama` on every OS (`%USERPROFILE%\.config\authama` on Windows) | `$AUTHAMA_CONFIG_DIR`, else `$XDG_CONFIG_HOME/authama` (an absolute value only) |
| State home | `~/.local/state/authama` | `$AUTHAMA_STATE_DIR`, else `$XDG_STATE_HOME/authama`; with `$AUTHAMA_CONFIG_DIR` set, the config home itself |
| `.env` | `<config home>/.env` | - |
| `authama.json` | `<config home>/authama.json` | - |

The config home holds what you write or back up: `.env`, `authama.json`, the
manifest, inventory, providers, token config and the age identity. The state
home holds what authama writes for itself: `log/audit.log`, `log/history.db`,
`snapshots/` and `dist/`. `authama settings path` prints all four locations and
whether each file loaded.

### Precedence

```
command-line flags  >  environment  >  .env  >  authama.json  >  built-in defaults
```

At startup (every command except `help`, `version` and `completion`) authama
reads `.env`, then `authama.json`, and sets only what is still unset - a
variable already in the environment, under its current or a legacy name, is never
replaced by a file. `authama settings show --effective` lists every setting with
its value and where it came from (secret values are never printed).

### `.env`

Secrets and references (the age recipient, provider tokens, the variables the
token commands read) and any `AUTHAMA_*` setting. `authama init` seeds it at mode
0600 from the shipped `.env-example`; it is gitignored and **excluded from the
encrypted bundle**.

```sh
# comments and blank lines are ignored
AUTHAMA_SNAPSHOT_RETAIN=20
export GH_TOKEN="cmd:keyring get authama gh"   # an `export ` prefix is accepted
TENANTS="landlord panel api socket queue"      # quote values with spaces or special characters
SINGLE='kept $literally'                        # single quotes: no escapes
```

- `KEY=value`, with an optional `export ` prefix. Double-quoted values support
  `\n`, `\t`, `\"` and `\\`; single-quoted values are literal; an unquoted value
  ends at ` #`.
- A malformed line is skipped with a warning naming its line number, never its
  content.
- A missing file is fine. A file readable by group or others is **not loaded**:
  authama says so and suggests `chmod 600` (or `authama doctor --fix`). Windows
  has no POSIX mode bits, so the check does not apply there.
- `AUTHAMA_CONFIG_DIR`, `AUTHAMA_DEV_ROOT` and `AUTHAMA_NO_DOTENV` decide which
  file is read, so they are honoured only from the real environment; in `.env`
  they are ignored with a warning.
- `--no-dotenv` or `AUTHAMA_NO_DOTENV=1` skips the file for that run.

Earlier releases did not load the file, and the docs suggested
`set -a; . ~/.config/ssh-manager/.env; set +a` in a shell profile. That still
works (exported variables win over the file) but is no longer needed; drop it so
the shell does not hold your tokens. `authama token apply --source FILE` still
reads any other `VAR=value` file for the token commands.

### `authama.json`

Non-secret settings, validated against
[`docs/schema/authama.schema.json`](schema/authama.schema.json) (JSON Schema
2020-12). Unknown keys, wrong types and out-of-range values are errors: a command
stops and lists every problem, and `authama settings validate` checks the file on
demand. `authama settings init` writes a starter (only `$schema` and `version`), so
every setting stays at its default until you add it.

```json
{
  "$schema": "https://raw.githubusercontent.com/simtabi/authama/main/docs/schema/authama.schema.json",
  "version": 1,
  "paths":   { "state_dir": "~/.local/state/authama", "age_identity_file": "~/.config/authama/age-identity.txt" },
  "backup":  { "snapshot_retain": 10, "old_key_max_age_days": 90 },
  "ssh":     { "auto_pin": true },
  "history": { "enabled": true },
  "git":     { "mode": "profile" },
  "tokens":  { "docker_creds_store": "refuse" },
  "notifications": { "desktop": true }
}
```

| Key | Environment variable | Default |
|---|---|---|
| `paths.state_dir` | `AUTHAMA_STATE_DIR` | `$XDG_STATE_HOME/authama` or `~/.local/state/authama` |
| `paths.age_identity_file` | `AUTHAMA_AGE_IDENTITY_FILE` | unset (`ssh restore` needs this or `--identity`; the web UI's restore needs this) |
| `paths.tokens_config` | `AUTHAMA_TOKENS_CONFIG` | see [Auth tokens](#auth-tokens) |
| `paths.tokens_catalog` | `AUTHAMA_TOKENS_CATALOG` | the built-in catalog |
| `backup.age_recipient` | `AUTHAMA_AGE_RECIPIENT` | unset (`ssh bundle` needs it) |
| `backup.snapshot_retain` | `AUTHAMA_SNAPSHOT_RETAIN` | `10` (at least 1) |
| `backup.old_key_max_age_days` | `AUTHAMA_OLD_KEY_MAX_AGE_DAYS` | `90` (`0` turns the check off) |
| `ssh.auto_pin` | `AUTHAMA_AUTO_PIN` | `true` |
| `history.enabled` | `AUTHAMA_HISTORY` | `true` |
| `git.mode` | `AUTHAMA_GIT_MODE` | the manifest's `defaults.git.mode`, else `profile` |
| `tokens.docker_creds_store` | `AUTHAMA_DOCKER_CREDS_STORE` | ask at a terminal, else refuse |
| `notifications.desktop` | `AUTHAMA_NOTIFICATIONS` | `true` |

Paths may start with `~/`. A secret never goes in `authama.json`; put it in
`.env`. Only the environment can set `AUTHAMA_CONFIG_DIR`, `AUTHAMA_DEV_ROOT`
and `AUTHAMA_NO_DOTENV`.

### The `settings` commands

`settings` is its own group so it cannot be confused with `ssh config`, which
renders `~/.ssh/config`:

```sh
authama settings path              # config home, state home, .env, authama.json
authama settings show --effective  # every setting, its value and its source
authama settings validate          # schema-check authama.json, syntax- and mode-check .env
authama settings init              # write a starter authama.json
```

### Old variable names

Most `AUTHAMA_*` names above replaced an earlier one, which is still read with a
one-line deprecation notice on stderr: `SSH_MANAGER_HOME` and
`SSH_MANAGER_CONFIG_DIR` (now `AUTHAMA_CONFIG_DIR`), `SSHMGR_DEV_ROOT`,
`SSHMGR_GIT_MODE`, and `SSH_MANAGER_<NAME>` for the rest (for example
`SSH_MANAGER_AGE_RECIPIENT` → `AUTHAMA_AGE_RECIPIENT`). When both spellings are
set, the `AUTHAMA_*` one wins. See [Migration](migration.md).

### Token values

To avoid storing a token at rest, set its value to `cmd:<command>` - the command
runs at use-time and its trimmed stdout is the token (so it integrates with any
secret manager and the secret never touches disk):

```sh
GH_TOKEN="op://Private/GitHub/token"                    # 1Password secret reference
GH_TOKEN="cmd:op read op://Private/GitHub/token"        # the same, spelled out
GH_TOKEN="cmd:keyring get authama gh_token"             # OS keyring
```

A value starting `op://` is read with `op read`, the whole reference as one
argument, so item and field names may contain spaces.

The command runs once per process (memoized) via the argv-only subprocess
chokepoint - no shell, no injection. A failed/empty/timed-out command yields no
token (the provider falls back to the manual paste path). Note: a literal token
that itself begins with `cmd:` would be treated as a command - real OAuth/PAT
tokens never do, but if needed, fetch such a value via `cmd:` instead.

## <a name="auth-tokens"></a>Auth tokens

The token config is a separate file, so the manifest stays about SSH. It names
environment variables, never secrets: `authama token apply` reads the variables
and writes each tool's credential file. It is found in this order:

1. `--config FILE`
2. `$AUTHAMA_TOKENS_CONFIG` (or `paths.tokens_config` in `authama.json`)
3. `./authama-tokens.json` (a project's own)
4. `<home>/tokens.json` (`authama token init` writes a starter one)
5. auth-token-manager's locations, read with a deprecation notice until
   `authama token import` moves the file: `$AUTH_TOKEN_MANAGER_CONFIG`,
   `$COMPOSER_AUTH_CONFIG`, `./auth-token-manager.json`,
   `~/.config/auth-token-manager.json`
6. otherwise a built-in default: `GH_TOKEN` as Composer's github.com token

The file's shape, each entry kind, and `_settings.docker_creds_store` (`login`
or `refuse` for registries Docker keeps in a credential store;
`--docker-creds-store` and `$AUTHAMA_DOCKER_CREDS_STORE` take precedence over it) are in
[`token`](tools/tokens.md). The files it writes are the tools' own, at their
usual paths, honouring `COMPOSER_HOME`, `NPM_CONFIG_USERCONFIG`,
`DOCKER_CONFIG`, `NETRC` and `XDG_CONFIG_HOME` (and `APPDATA` on Windows).
`$AUTHAMA_TOKENS_CATALOG` (or auth-token-manager's
`$AUTH_TOKEN_MANAGER_CATALOG`) replaces the built-in service catalog; see
[`token` › A custom catalog](tools/tokens.md#a-custom-catalog).

## Operation history

Every command that changes something is recorded in `log/history.db` in the
state home (`~/.local/state/authama/log/history.db` by default), beside
`audit.log`. `authama history` reads it; token writes and removals are recorded too,
with the target and host but never the value. Set `AUTHAMA_HISTORY=0` (or `"history": {"enabled": false}` in `authama.json`) to record nothing; `audit.log` is
written either way. See [`history`](tools/history.md).

## Dev mode

`--dev-root <dir>` (or `$AUTHAMA_DEV_ROOT`) points the whole tool at a scratch
directory: `<dir>/ssh` stands in for `~/.ssh` and `<dir>/config` for the config
and state homes (its `.env` and `authama.json` are the ones loaded), and `<dir>/home` for your home directory, where the token commands write
`auth.json`, `.npmrc` and the rest. The tools' own path variables
(`COMPOSER_HOME`, `NPM_CONFIG_USERCONFIG`, `DOCKER_CONFIG`, `NETRC`,
`XDG_CONFIG_HOME`, `APPDATA`) are ignored in dev mode, and `docker login` is
refused, since it would write outside the sandbox. It is one root rather than
several overrides, so a run cannot end up reading a sandbox and writing your real tree. See
[Work against a sandbox](recipes/dev-mode.md).

---

[← Docs index](../README.md#documentation)
