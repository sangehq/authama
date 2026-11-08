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
  },
  "reserved_keys": {                     // optional (0.1.4): loose ~/.ssh keys kept on purpose
    "docker-tunnel": {
      "for": "devbox",                    // free text, shown by doctor
      "note": "Docker devbox VM, kept for later",
      "fingerprint": "SHA256:…",          // of the .pub when reserved; doctor notices a swap
      "since": "2026-11-02"
    }
  }
}
```

Hosts need not be written by hand: `authama ssh host add|edit` change one field set at a
time, and `authama ssh host apply <profile> <alias> <host>[:port] <user> [role]` (or many
with `--host`/`-f`) adds or updates `alias`, `hostname`, `user`, `port` and `tags` in one
step. See [Host specs](recipes/host-specs.md).

### `reserved_keys`

Written by `authama ssh key reserve` and removed by `key unreserve` (and by `vm adopt-key`/`vm sync` when
they adopt the key); you rarely edit it by hand. Each name is a key file directly in `~/.ssh` (not a
`.pub`, not one of ssh's own files). The section is omitted when empty, so a manifest without
reservations is unchanged. Older authama versions (0.1.3 and earlier) refuse a manifest that has it
(unknown fields are an error), so unreserve first if you go back. See
[Keeping a key for later](tools/vm.md#keeping-a-key-for-later).

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
| `key_root` | `profiles` | the directory under `~/.ssh` that holds the profile directories; change it with `authama ssh key root <dir>`, which moves the keys (see [Where the keys live](#key-root)) |
| `git.mode` | `profile` | default GitHub remote mode for `authama ssh git use`: `profile`, `ssh` or `https` |
| `git.profile` | unset | the default identity: its GitHub key is bound to a rendered `Host github.com` (ssh mode), and it is used for owners `git.orgs` does not list. Unset, no `Host github.com` is rendered. The block is rendered once: in the profile's category file (`profiles/<category>/config`) when it has a category, otherwise in the root block; skipped if a host is already aliased `github.com` |
| `git.orgs` | `{}` | GitHub owner → profile, for `authama ssh git` (see [tools/git.md](tools/git.md)) |
| `export.selections` | unset | export selections saved by name in the manifest, so they travel with it (see [tools/export.md](tools/export.md#saved-selections)); omitted when there are none |

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
SSH and are not hosts); `name` is the VM as the hypervisor lists it, spaces allowed. A custom hypervisor declared in
`authama.json`'s `hypervisors` section is accepted too. See [tools/vm.md](tools/vm.md).

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

## <a name="naming-servers"></a>Naming servers

A server's alias says whose it is, which environment, what it does and which one. The rules are in
the manifest, so they can be changed, and authama can name a server by them:

```
<code>-<env>-<role>-<nn>          simbi-prd-web-00   simbi-prd-db-00   dani-prd-web-00
```

| Part | Meaning | Examples |
|---|---|---|
| `code` | the organisation's code name, set on its profile (`code`) | `simbi` (Simtabi), `dani` (AdelsaIQ) |
| `env` | the environment | `prd`, `stg`, `dev` |
| `role` | what the server does | `web`, `db`, `cache`, `worker`, `vpn` |
| `nn` | two digits, from `00`: the next server of the same code, environment and role | `00`, `01` |

The code name is only in the alias: the server goes in the organisation's own profile (`simtabi` for
`simbi-*`, `adelsaiq` for `dani-*`), in the category that says what the organisation is to you
(`orgs` for your own, `clients` for clients'), tagged with its role and environment so the web UI,
`list` and the exports can filter by them.

### Let authama name it

```bash
authama ssh profile edit simtabi --code simbi                 # once per organisation
authama ssh host add simtabi --env prd --role web -H 203.0.113.10 -u ploi
authama ssh host add simtabi --env prd --role web -H 203.0.113.11 -u ploi
authama ssh reconcile --yes
```

The first is `simbi-prd-web-00`, the second `simbi-prd-web-01`: the lowest number no server of the
same code, environment and role has. Both are tagged `web` and `prd`. Naming a host yourself works as
before: `authama ssh host add simtabi simbi-prd-db-00 -H 203.0.113.20 -u ploi --tag db --tag prd`.

### The rules

`defaults.naming` holds them for every profile, and a profile's own `naming` overrides them field by
field; every field is optional.

```json
"defaults": {
  "naming": {
    "alias": "{code}-{env}-{role}-{nn}",
    "key_name": "{profile}_{alias}-{algo}",
    "envs": ["prd", "stg", "dev"],
    "roles": ["web", "db", "cache", "worker", "vpn"],
    "enforce": "warn",
    "start": 0,
    "code_fallback": "none"
  }
},
"profiles": {
  "simtabi":  { "category": "orgs", "code": "simbi", "hosts": [] },
  "adelsaiq": { "category": "clients", "code": "dani",
                "naming": { "alias": "{code}-{role}-{nn}", "start": 1 }, "hosts": [] }
}
```

| Field | Default | Meaning |
|---|---|---|
| `alias` | `{code}-{env}-{role}-{nn}` | the alias template, from `{code}`, `{env}`, `{role}` and a number |
| `key_name` | `{profile}_{alias}-{algo}` | the key-name template (see [Key names](#key-names)) |
| `envs`, `roles` | any | the values `{env}` and `{role}` may take; listing them also makes reading an alias exact |
| `enforce` | `off` | what adding a host whose alias breaks the rules does: `off`, `warn` (added, with the reason printed) or `error` (refused) |
| `start` | `0` | the first number: `0` (`00`) or `1` (`01`) |
| `code_fallback` | `none` | the `{code}` of a profile with no `code` of its own: `none`, or `profile` (its name, lowercased to letters and digits) |

In a template:

- `{nn}` is the number, two digits; `{nnn}` three. A template must have one, so a second server of
  the same kind can be named.
- `{env?}` (any part but the number) is **optional**: when it has no value it is left out with the
  separator before it, or after it for the first part, so `{code}-{env?}-{role}-{nn}` names both
  `simbi-prd-web-00` and `simbi-web-00`. Two optional parts in a row are refused, since an alias
  with only one of them could not be read back.
- Parts are joined by literal text, and only lowercase letters, digits, `-` and `.` keep the alias a
  valid host name.

```bash
authama ssh naming show                                     # the rules in effect, each profile's code and own rules
authama ssh naming check                                    # the servers that do not follow them (--strict for CI)
authama ssh naming set --envs prd,stg,dev --enforce warn    # every profile
authama ssh naming set --profile adelsaiq --alias '{code}-{role}-{nn}' --start 1
authama ssh naming set --code-fallback profile
authama ssh naming key-template '{profile}_{code}_{env}_{role}_{nn}-{algo}'
```

`naming check` only reports: a server named before the rules keeps its name, because an alias is in
`~/.ssh/config`, in shell history and in other tools' imports, and renaming one is a change of its
own. VM hosts are devboxes, not servers, and are never held to the rules.

The web UI's **Naming** page (under **SSH**) shows the same: the rules in effect, each profile's code
and own rules, and what `naming check` reports. Its forms change the rules for every profile or for
one, the key template, and a profile's code, each previewed first. A rules form writes only the
fields you change, so its preview reads as the `naming set` you would type.

### Every setup

| Setup | How |
|---|---|
| An organisation with a code | `profile edit <p> --code <code>`; aliases start with it |
| A profile with no code | the default template needs one, so set it, or `naming set --code-fallback profile` to use the profile's name, or give the profile a template without `{code}` |
| A convention with no code at all | `naming set --alias '{env}-{role}-{nn}'` |
| A client with a convention of its own | `naming set --profile <client> --alias ...`, over the defaults |
| One environment only, so no env in the name | `{env?}`: leave out `--env` and it drops out |
| Numbering from 01, or three digits | `--start 1`; `{nnn}` |
| Keys named with the code, when there is one | `naming key-template '{profile}_{code?}_{alias}-{algo}'` |
| Hosts named by hand | `host add <p> <alias>` as ever; `enforce` decides whether a stray name warns or is refused |
| A shared key for a whole profile | `key_scope: shared`; templates do not apply to it |

### No change renames a key

Every change a key name can depend on (setting or clearing a profile's `code`, `naming set` at either
level, `naming key-template`) first works out every existing host's key name, makes the change, and
writes the old name into the manifest as the `key_name` of each host whose name would otherwise
change, and lists them (`kept its key name: acme/web1`). So no key already on disk is renamed or left
behind; only hosts added afterwards follow the new rules. Editing those fields by hand in the
manifest skips this step and would re-point existing hosts at keys that do not exist.

### Key names

By default a key is named `<profile>_<alias>-<algo>`, `simtabi_simbi-prd-web-00-ed25519`: the
underscore ends the profile, and the alias keeps its hyphens, so it is clear where it starts and
ends. A template can name keys from the alias's parts instead, such as
`{profile}_{code}_{env}_{role}_{nn}-{algo}` for `simtabi_simbi_prd_web_00-ed25519`. A key-name
template starts with `{profile}_` and ends with `-{algo}`, which is how a key's profile and type are
read back, and needs at least one part that is always there between them. A host whose alias lacks a
part the template needs (an alias named before the rules, or a profile with no code under `{code}`)
keeps the default name; `{code?}` leaves the code out instead.

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

## <a name="key-root"></a>Where the keys live

The profile directories sit under one directory in `~/.ssh`, the key root. It is `profiles` unless
`defaults.key_root` names another, so a profile's keys are at `~/.ssh/<root>/<profile>/` (or
`~/.ssh/<root>/<category>/<profile>/`), and every rendered `IdentityFile`, category `Include`,
`old/` archive, snapshot, audit and backup follows it.

```bash
authama ssh key root          # ~/.ssh/profiles/
authama ssh key root keys     # moves everything to ~/.ssh/keys/ and renders the config again
authama ssh key root profiles # and back
```

Moving is one rename of the whole tree: private keys, `.pub` files, `old/` archives and category
config files. The inventory follows, `~/.ssh/config` is rendered again so ssh finds every key at its
new path, and `~/.ssh` is snapshotted first. Adopted keys stay where they are.

| Rule | Why |
|---|---|
| One directory name, such as `keys` or `id` | A nested root (`a/b`) is refused; it would complicate every scan for no gain. |
| Not `.` | Profile directories loose in `~/.ssh` would sit beside your own folders, and a scan could not tell them apart. |
| Not hidden, and not one of ssh's own names (`config`, `known_hosts*`, `authorized_keys*`, `id_*`, `*.pub`) | Hidden directories are where authama stages files; the rest belong to ssh. |
| The target may not already exist beside the current root | Merge the two by hand first; nothing is overwritten. |
| No reserved key may share its name | A reserved key is a file directly in `~/.ssh`. |

Set the root with the command rather than by editing the manifest: an edit alone leaves the keys
where they were, and `authama doctor` reports them (`stranded_key_root`) with the two commands that
bring them over. A backup bundle names its members under `ssh/profiles/` whatever the root, and a
restore lays them down under the root the bundle's manifest names. See
[Keep your keys in a directory of your choosing](recipes/choose-where-keys-live.md).

## <a name="adopted-keys"></a>Keys where you already keep them

A key does not have to live under the key root. A declared key with a `path` is used where it is:

```json
"personal": {
  "keys": [{ "name": "github", "path": "id_ed25519" }],
  "hosts": [{ "alias": "gh", "hostname": "github.com", "user": "git", "key_name": "github" }]
}
```

`path` is relative to `~/.ssh` (`id_ed25519`, `work/deploy_key`). The host renders
`IdentityFile ~/.ssh/id_ed25519`, and deploy, rotation, the audit and the web UI treat the key like
any other, but authama never moves, renames or deletes that file:

- Rotation and `keygen --force` archive the outgoing pair in the profile's own `old/` (under the
  key's name, `profiles/personal/old/github`) and write the successor to the same path, so other
  tools that read `~/.ssh/id_ed25519` keep working.
- `key delete --purge` and `profile delete --purge` leave an adopted file alone; only the archives
  authama made are removed.
- The key audit counts it as tracked, not as a loose key.

The path must be inside `~/.ssh` (no `..`, no symlink that leads out), not a `.pub`, not a file ssh
owns (`config`, `known_hosts`, `authorized_keys`), not in a hidden directory, and adopted by one key
only. A profile that keeps its keys in 1Password cannot adopt a file.

`authama ssh key adopt <profile/key> <path> [--host <alias>]` writes the declaration and records
the key, after checking the file is a private key.

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
its value and where it came from (secret values are never printed). The web UI's
**Settings** page shows the same, with the files and their validation.

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
AUTHAMA_VM_HYPERVISORS="parallels,vmpal"         # the hypervisors you use
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
  "notifications": { "desktop": true },
  "vm":      { "hypervisors": "parallels,vmpal", "prune_gone": true, "repin_changed": false },
  "web":     { "addr": "127.0.0.1", "port": 7822, "base_path": "/" }
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
| `export.save_to` | `AUTHAMA_EXPORT_SAVE_TO` | `user` (where a saved export selection goes: `user` or `manifest`; see [tools/export.md](tools/export.md#saved-selections)) |
| `tokens.docker_creds_store` | `AUTHAMA_DOCKER_CREDS_STORE` | ask at a terminal, else refuse |
| `notifications.desktop` | `AUTHAMA_NOTIFICATIONS` | `true` |
| `vm.hypervisors` | `AUTHAMA_VM_HYPERVISORS` | every built-in driver and every custom one |
| `vm.multiplex` | `AUTHAMA_VM_MULTIPLEX` | `true` (a VM host shares one SSH connection for 10 minutes; ignored on Windows; see [tools/vm.md](tools/vm.md#shared-connections)) |
| `vm.prune_gone` | `AUTHAMA_VM_PRUNE_GONE` | `true` (`ssh vm sync` removes the host of a VM that is gone) |
| `vm.repin_changed` | `AUTHAMA_VM_REPIN` | `false` (`ssh vm sync` reports a changed guest host key) |
| `web.addr` | `AUTHAMA_WEB_ADDR` | `127.0.0.1` (a non-loopback address still needs `--allow-remote`) |
| `web.port` | `AUTHAMA_WEB_PORT` | `7822` (the next free port is used when it is taken; only `--port` makes that an error) |
| `web.menu_export` | `AUTHAMA_WEB_MENU_EXPORT` | `true` (show Export in the menu; off hides the item, and the page and each list's download stay) |
| `web.base_path` | `AUTHAMA_WEB_BASE_PATH` | `/` (serve the UI under a path such as `/authama`; see [tools/serve.md](tools/serve.md#address)) |
| `web.auto_restart` | `AUTHAMA_WEB_AUTO_RESTART` | `true` (restart `serve` by itself when its binary is rebuilt, and reload open pages into the new build; see [installation.md](installation.md#updating)) |

Two sections are not settings but declarations: `hypervisors` adds a command driver for a VM tool
authama has no driver for, and `devboxes` names the VM hosts `ssh vm sync` keeps (a devbox without `vm` only keeps its key reserved). Both are described,
with examples, in [tools/vm.md](tools/vm.md#choosing-your-hypervisors).

Paths may start with `~/`. A secret never goes in `authama.json`; put it in
`.env`. Only the environment can set `AUTHAMA_CONFIG_DIR`, `AUTHAMA_DEV_ROOT`,
`AUTHAMA_NO_DOTENV` and `AUTHAMA_OWNER_STORE` (`file` keeps the [owner binding](tools/owner.md)'s
sealing key in an owner-only file rather than the OS credential store).

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

Releases before 0.1.1 read `SSH_MANAGER_*` and `SSHMGR_*` names. authama read
them too, with a deprecation notice, until 2026-11-07; it ignores them now, so a
value set only under an old name has no effect. Rename it: `SSH_MANAGER_HOME`
and `SSH_MANAGER_CONFIG_DIR` → `AUTHAMA_CONFIG_DIR`, `SSHMGR_DEV_ROOT` →
`AUTHAMA_DEV_ROOT`, `SSHMGR_GIT_MODE` → `AUTHAMA_GIT_MODE`, and
`SSH_MANAGER_<NAME>` → `AUTHAMA_<NAME>` for the rest. See [Migration](migration.md).

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
