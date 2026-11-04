# Feature catalog

The complete, tested surface of authama (Authentication Access Manager): SSH
management under `authama ssh`, auth tokens under `authama token`, and the
shared commands at the top level. SSH verbs are listed by their short name
(`reconcile` is `authama ssh reconcile`; the top-level spelling still works). Every row is exercised by the test suite:
**U** = unit tests (`go test ./...`), **E** = end-to-end smoke (`make e2e`), **F** =
per-command feature check (`make feature-check`, which exercises every command in
a sandbox). Run all three with `make test e2e feature-check`.

## At a glance

- **One manifest, reproducible `~/.ssh`.** Profiles (who you are) hold hosts
  (where you go); `reconcile` mints missing keys and renders a managed block of
  `~/.ssh/config`, leaving everything outside it alone.
- **Isolated identities.** Per-profile keys, `IdentitiesOnly`, and profile-prefixed
  aliases keep two GitHub accounts on one machine from acting as each other.
- **Key delivery.** `deploy` installs a public key through `ssh-copy-id`, `gh`,
  `glab`, a cloud provider's REST API (DigitalOcean, Hetzner, Vultr, Linode,
  Scaleway, …) or a manual paste step.
- **Safe rotation.** Staged, verified, zero-downtime `rotate` with rollback,
  expiry reminders, and snapshots of `~/.ssh` before every write.
- **Trust and reachability.** Host-key pinning (`knownhosts`), VPN-gated hosts
  (`net`), Cloudflare Access proxies, and local VMs addressed by name (`vm`).
- **Self-maintaining VM hosts.** `vm sync` (daily, or from `doctor --fix`) re-pins, re-installs keys,
  refreshes Docker contexts and drops hosts whose VM is gone, with backups; the hypervisors you use
  are configured, any other VM tool plugs in as a command driver, and `vm adopt-key` adopts a loose key; `key reserve` (or a devbox declared without a `vm`) keeps one for later.
- **Backups and recovery.** `age`-encrypted bundles, restore onto a new machine,
  and a break-glass `recover` script.
- **Auth tokens.** `auth.json`, `.npmrc`, Docker `config.json`,
  `.git-credentials` and `.netrc` built from environment variables, owner-only and
  atomic, with `audit` and masked listing.
- **Three front ends.** The CLI, an interactive `tui`, and a local web UI (`serve`)
  that previews every change; every change lands in `history`.

The mental model: the **manifest** in `~/.config/authama/manifest.json` is the single source
of truth; `~/.ssh` is generated, reproducible output. Profiles model *identity*
(work · personal · acme · ...), not technology, and everything for an identity
lives isolated under `~/.ssh/profiles/<profile>/`, or `~/.ssh/profiles/<category>/<profile>/`
when the profile has a category.

## Home & setup

| Command | What it does | Tests |
|---|---|---|
| `init` | Create/converge the per-user homes (`~/.config/authama` for config, `~/.local/state/authama` for state): every run (re)creates the dir structure (`log/`, `snapshots/`, `.state/`) + re-asserts perms (dirs 700, secrets 600) and seeds any **missing** starter files (manifest, inventory, `.env`) - never clobbering existing ones. `providers.json` is not seeded (the shipped catalog is used unless you create your own). | U·E·F |
| `init --force` | Overwrite the seed files with fresh defaults, **in place** (no backup). | U·F |
| `init --force --backup` | Same, but first copy the old files into `~/.local/state/authama/.state/init-backup-<ts>/` (the state home). | U·F |
| `migrate [--force] [-y]` | Copy a pre-0.1.1 home (`~/.config/ssh-manager`, `%APPDATA%\ssh-manager` on Windows, `~/.config/sshmgr`, `~/.sshmgr`) into the **resolved** home and mark the old one `MIGRATED-TO-AUTHAMA.txt` (`$AUTHAMA_CONFIG_DIR` override-aware; auto-migration handles the first run on any command; this resolves the stranded both-exist case `doctor` warns about - `--force` backs up the current home and replaces it). | U |
| `settings path [--json]` | Print the config home, state home, `.env` and `authama.json` locations and whether each loaded. | U |
| `settings show [--effective] [--json]` | Print `authama.json`, or with `--effective` every setting's value and where it came from (flag, environment, `.env`, `authama.json`, default). Secret values are never printed. | U |
| `settings validate [FILE]` | Check `authama.json` (or FILE) against its schema and `.env`'s syntax and permissions; lists every problem, exit ≠0 on any. | U |
| `settings init [--force]` | Create a starter `authama.json` (only `$schema` and `version`; every setting stays at its default until you add it). | U |
| `doctor` | Verify the environment: deps, perms, agent, known_hosts, ≤1-old-key invariant, config drift, orphan/duplicate keys, alias collisions, and the modes of the token credential files (a loose one is a warning; `--strict` fails on it). Prints the **resolved home**, `~/.ssh`, and the active provider-catalog source; warns about a stranded legacy home. | U·E·F |
| `doctor --fix` | Re-assert canonical perms on every tool-managed path (`~/.ssh` tree + home secrets + token credential files). | U·F |
| `doctor --json` | Machine-readable report for scripting/monitoring. | U |
| `doctor --strict` | Exit non-zero on any warning, dangling-key states included (for CI); never reports clean when there are warnings. | U |
| `version` / `--version` | Print the version. | F |

Home resolution: `$AUTHAMA_CONFIG_DIR` (or the pre-0.1.1 `$SSH_MANAGER_HOME`) if
set, else `$XDG_CONFIG_HOME/authama`, else `~/.config/authama` - the same on every
OS, Windows included. State (logs, history, snapshots, bundles) goes to
`$AUTHAMA_STATE_DIR`, else `$XDG_STATE_HOME/authama`, else
`~/.local/state/authama`. A pre-0.1.1 home is copied in on the first run. No
project-local mode. See [installation.md](installation.md) for the full layout.

## Build & verify `~/.ssh`

| Command | What it does | Tests |
|---|---|---|
| `reconcile [--no-pin] [--passphrase] [-y]` | Make `~/.ssh` match the manifest: build the profile tree, **mint missing keys**, render config, set perms, and **auto-pin each profile's `known_hosts`** for reachable hosts (`--no-pin` or `AUTHAMA_AUTO_PIN=0` to skip). Snapshots `~/.ssh` first. | U·E·F |
| `reconcile --dry-run` | Preview the plan; write nothing. | E·F |
| `keygen <profile\|host> [--no-pin] [--no-key-backup]` | Targeted key generation; warns + **skips** keys that already exist; auto-pins the affected profiles' `known_hosts` for reachable hosts. | U·E·F |
| `keygen ... --force [--yes]` | Overwrite existing keys (prompted; `~/.ssh` snapshotted first). | U·E·F |
| `keygen ... --passphrase` | Protect new keys with a passphrase (off by default, so omitting the flag is the "no" answer; prompts without echo on a TTY). | U |
| `config check` | Verify the on-disk config byte-for-byte matches a fresh render, and that no `Host` pattern is defined twice across the managed files (`DUPLICATE`); exit ≠0 on either. | U·E·F |
| `config render [-n\|--dry-run] [-y]` | Re-render config files from the manifest (one renderer, shared with `check`/`reconcile`). | U·E·F |
| `config show [alias]` | Print resolved config, or `ssh -G` for one alias. | U·F |
| `config export --flat [--out PATH\|-]` | Write every host inline in one file, absolute paths, no `Include` (for tools that read one `ssh_config`). Never touches `~/.ssh`; refuses any target inside `~/.ssh`. | U·F |
| `termius export [--out PATH\|-] [--skip ALIAS] [--include-git] [--open]` | Write a Termius CSV import (group = profile, label = alias) and list the key to attach to each host; `--open` opens Termius on macOS. | U·F |
| `termius keys [ALIAS] [--clear-after DUR] [--clear] [--print] [--include-git] [--skip ALIAS]` | The CSV carries no keys: list each host's Termius key label and key path, or copy one host's private key to the clipboard (never printed unless `--print`) with the label to paste into Keychain → New Key. `--include-git` widens the list; a named git host is found without it. | U·F |
| `import [path] [--force] [--dry-run] [-y]` | Onboard an existing `~/.ssh` (adopt keys + hosts) into the manifest + inventory. Refuses to replace a non-empty manifest without `--force` (which backs it up first). | U·F |
| `diff` | Preview manifest vs. on-disk reality (config + which keys are missing/present); what `reconcile` would change. `config check` is the exit-code-only version. | U·F |

## Keys

| Command | What it does | Tests |
|---|---|---|
| `key list [selector]` | Every key with its fingerprint, expiry, the hosts using it, and its state — including keys nothing references. | U |
| `key regen-pub [selectors...] [--category] [--all] [--dry-run]` | Rebuild `.pub` files from the private key or the 1Password item; reports unchanged / created / rewritten / skipped. Never changes a private key. | U |
| `key add <profile> <name> [--type] [--rotate-after-days] [--host] [--passphrase]` | Declare a key on a profile and mint it. Without `--host` the key is minted **unwired**: it exists and nothing uses it, which `doctor` then reports. | U |
| `key reserve <name\|path> [--for] [--note] [-n] [-y]` | Keep a loose `~/.ssh` key on purpose (for a devbox you have not made yet): `doctor` lists it as reserved instead of loose. Recorded in the manifest's `reserved_keys` with the public key's fingerprint; the key file is never touched. `vm adopt-key` and `vm sync` release the reservation when they adopt it. | U·E |
| `key unreserve <name> [-y]` | Drop a reservation; the key is loose again. | U·E |
| `key delete <profile/key> [--purge] [--revoke] [--yes] [--no-key-backup]` | Remove the declaration and the inventory record. **Refuses while any host still resolves to the key** — a `Host` block pointing at a missing `IdentityFile` fails as a bare "Permission denied (publickey)" with nothing naming the cause. `--purge` also deletes the files. | U |
| `clean [--dry-run] [--adopt] [-y]` | Prune stale `known_hosts` pins, stale inventory records and write residue. Only removes pins tagged `authama` (or `sshmgr`, from releases before 0.1.2); `--adopt` opts an untagged pin that matches a manifest host into management, since an untagged pin is presumed to be yours. | U |

## Inspect

| Command | What it does | Tests |
|---|---|---|
| `show <profile\|alias\|profile/key>` | Everything about one thing at once: the manifest entry, the key files and their modes, the rendered config block, and the pins. An auth failure is usually a disagreement between those four, and this is where you see it. | U |
| `list [--profile/--provider/--type/--tag]` | Filterable tree across profiles. | U·E·F |
| `view <profile\|alias>` | The connect view (compare `show`, the inventory view): resolved host config + key + deployment status; shows a **VPN reminder** for `requires_vpn` hosts. | U·F |
| `validate [key\|profile]` | Check each keypair: both parse, public key is *derived from* the private (`ssh-keygen -y`), perms correct; encrypted keys are noted not failed. Exit ≠0 on failure. | U·E·F |
| `providers [--export [--force]]` | List the active provider catalog (your `<home>/providers.json` if present, else the shipped default) + whether each credential is set. `--export` writes an editable copy into the home. | U·E·F |
| `net [selector]` | Per-host connection status + a VPN/tunnel indicator. Exit ≠0 if a `requires_vpn` host is unreachable. | U·F |
| `expiry` | Per-key rotation-age table (ok / due_soon / overdue), from each key's stored `expires_on`. | U·E·F |
| `audit [--notify]` | Where each key is deployed + expiry + hygiene + recent activity (optionally fire a desktop alert). | U·E·F |
| `serve [--addr/--port/--allow-remote/--allow-host/--read-only/--no-open/--token-file]` (or a bare `authama` on a terminal) | The web UI on `127.0.0.1:7822`: overview, profiles, hosts, keys, VMs, auth tokens, snapshots and their diffs, history, doctor, audit, settings. Makes the everyday changes, each previewed on a throwaway copy and made only if nothing moved since: profiles and hosts (one at a time or from specs), categories, deletes with or without key files, reconcile, keygen, key add, render, clean, prune, snapshot restore, bundles, doctor `--fix`, VM sync, key adoption and the daily-sync schedule, Docker contexts, notifications, token sync and forget, and a starter `authama.json`. Runs deploy, rotate (one key or many), rollback and bundle restore as jobs, with their output streamed. Downloads the flat `ssh_config` and the Termius CSV. Signs in with the token in the printed address. | U |
| `history [--host/--profile/--key/--since/--until/--command/--source/--limit/--json]`, `history show <op>` | Every change authama has made (SSH or token), newest first, from `log/history.db`: command, result, snapshot, audit events, and the manifest before and after. The first run imports `audit.log`, the inventory's dates and the snapshots. | U |

## Deploy, rotate, network/VPN

| Command | What it does | Tests |
|---|---|---|
| `deploy <key> [target] [-y]` | Install the public key on its target via the host's provider (GitHub/GitLab CLI, cloud-VPS REST, `generic-ssh` `ssh-copy-id`, or web-panel/manual) and record it. **Exits ≠0 if a target is unreachable.** | U·E·F |
| `rotate <key> [--yes] [--allow-unverified] [--passphrase] [--no-key-backup]` | Zero-downtime staged rotation: stage → deploy → verify → archive (≤1 predecessor). Aborts cleanly (pulling the staged key back) if a target can't be verified or is unreachable. A hosting-panel target (Ploi, Forge, cPanel) waits at a terminal for the pasted key, then logs in with it before committing. | U·E·F |
| `rotate [selectors...] [--category] [--all] [--dry-run]` | Rotate a batch: several keys, profiles or aliases, a category, or every key a host uses. One confirmation, backup and snapshot; each key rotates on its own and a failure leaves only that key untouched. | U |
| `rollback <key> [--yes]` | Restore the single `/old/` predecessor (re-deploy is best-effort; skips unreachable hosts). | U·E·F |
| `load <profile>` | Add the profile's keys to the agent (Keychain on macOS). | U·F |

**Network/VPN awareness** is woven through every host-touching action. A host can be
marked `requires_vpn` (+ optional `vpn_name`, `vpn_url`). `deploy`/`rotate` run a
bounded SSH-level reachability probe first, so a down or VPN-gated host (including a
`:443` host that accepts TCP but never speaks SSH) **fails fast** with
*"connect the VPN at `<url>` and retry"* instead of hanging - and every `ssh` /
`ssh-copy-id` is hard-timeout-bounded. See [tools/network.md](tools/network.md).

## Backup & recovery

| Command | What it does | Tests |
|---|---|---|
| `bundle [-r recipient] [-o dir] [--keep N]` | `age`-encrypted, off-machine backup of keys + manifest + inventory + providers. **Never** includes `.env`. | U·E·F |
| `restore <bundle> [-i identity] [--yes]` | Decrypt and lay the **same** keys back (true recovery on a new machine). | U·E·F |
| `snapshots list\|restore\|prune [--keep N]` | Local, reversible `~/.ssh` backups (every mutating command snapshots first; last 10 kept). `restore` takes `--yes`. | U·E·F |
| `recover [key]` | Break-glass: with a key, a tailored shell snippet to paste into a locked-out console; without, the full interactive `fixkeys` tool (reads `/dev/tty`). | U·F |

## Editing, trust, scheduling, UI

| Command | What it does | Tests |
|---|---|---|
| `profile add <name> [--key-scope\|--shared] [--key-name] [--key-comment] [--category]` | Add a profile, optionally filed under `profiles/<category>/`. `--shared` is shorthand for `--key-scope shared`. A profile keeps its keys on disk, or in 1Password with `key_store: 1password` (see [Key stores](configuration.md#key-stores)). | U·F |
| `profile edit <name> [--key-scope\|--shared] [--key-name] [--key-comment] [--category]` | Change a profile in place. `--key-comment` sets the comment template for keys minted from now on (see [Key comments](configuration.md#key-comments)). `--category` moves its key directory under `profiles/<category>/` (or back, with `""`) and re-points its inventory records. | U |
| `profile delete <name> [--yes] [--revoke] [--purge] [--no-key-backup]` | Remove a profile and its hosts; `--revoke` pulls its keys off their targets. | U·F |
| `host add <profile> <alias> [-H] [-u] [-p] [--provider] [--token-env] [--key-name] [--tag] [--proxy cloudflared]` | Add a host to a profile (alias is **positional**). `--proxy cloudflared` reaches it through Cloudflare Access. | U·F |
| `host edit <profile> <alias> [-H] [-u] [-p] [--provider] [--token-env] [--key-name] [--proxy cloudflared\|none] [--tag]... [--untag]...` | Change a host in place. `--tag` replaces its tags (`--tag ''` removes them all); `--untag` removes one, after any `--tag`. Without either, tags are left alone. | U·F |
| `host apply <profile> [<alias> <host>[:port] <user> [role]] [--host SPEC]... [-f/--file FILE\|-] [--tag] [--create-profile] [--category] [--reconcile] [--no-pin] [--dry-run] [-y]` | Add-or-update hosts from compact specs: one positional, repeatable `--host`, or a file (one per line, `#` comments, or a JSON array). New aliases are added as `host add`, existing ones edited as `host edit`; role and `--tag` merge into tags. Reports each host added/updated/unchanged; the batch is validated and saved at once. See [Host specs](recipes/host-specs.md). | U·F |
| `host delete <profile> <alias> [--yes] [--revoke] [--purge] [--no-key-backup]` | Remove a host; `--revoke` pulls the key off its targets, `--purge` deletes the key files if nothing else names them. | U·F |
| `knownhosts init [PROFILE] [--all] [--force] [-y]` | Initialize the trust store (create the file + pin reachable hosts, TOFU, fingerprints reported) for one profile or, with `--all`, every host in the manifest. There is one hashed store, so a host used by two profiles is pinned once. | U·F |
| `knownhosts pin [HOST] [-p PORT] [--all] [-y]` | Pin into the one `known_hosts` via `ssh-keyscan`, showing each fingerprint and asking first. A host that returns no SSH key fails the command (named, with the reason) instead of reporting `pinned 0`. | U·F |
| `git status [PATH...]` | Each checkout's GitHub remote mode (profile, ssh, https), owner/repo and profile. | U·F |
| `git use [MODE] [PATH...] [--mode] [--profile] [--dry-run] [--yes]` | Point checkouts' origin at profile SSH, plain SSH or HTTPS; the new URL must pass `git ls-remote` before origin is rewritten. | U·F |
| `vm list` | Every installed hypervisor's VMs, and which manifest host uses each; missing hypervisors say so. | U·F |
| `vm add PROFILE ALIAS --hypervisor H --vm NAME -u USER [--key-name] [-y]` | Add a local VM as a host; its address is resolved at connect time through `ProxyCommand "<authama>" vm proxy ALIAS`. | U·F |
| `vm docker-context ALIAS` | Create or update a Docker context `ssh://ALIAS` for a VM host. | U·F |
| `vm sync [-n] [-y] [-q] [--no-probe] [--repin] [--install-schedule\|--remove-schedule]` | Keep every VM host matching its VM: pin, re-install a missing guest key, refresh its Docker context; remove the host and context of a VM that is gone (backed up first); leave suspended and stopped VMs alone. | U·F |
| `vm adopt-key [KEY [PROFILE [ALIAS]]] [--hypervisor] [--vm] [-u] [--move] [--docker-context] [-n] [-y]` | Copy a loose `~/.ssh` key into a profile as a new VM host's key; asks for what is missing at a terminal. | U·F |
| `notify install\|test [-y]` | Scheduled desktop expiry reminders (launchd / systemd-user-or-cron / schtasks). | U·F |
| `tui` | A numbered menu over the whole facade, SSH and auth tokens. | U |

Cross-cutting flags: `--yes`/`-y` on destructive verbs (with `--revoke` on deletes);
`--passphrase` on key generation (a plain flag: leaving it off is the "no" answer; the Python version's
`--no-passphrase` is still accepted, hidden, on `keygen`, `reconcile` and `rotate`).

## Auth tokens

`authama token …` builds the credential files Composer, npm, Docker, git and curl
read, from a config that names environment variables and never holds a secret.
Ported from auth-token-manager; see [tools/tokens.md](tools/tokens.md) and the
[migration guide](migration.md).

| Command | What it does | Tests |
|---|---|---|
| `token apply [--target]... [--config] [--project] [--source] [--creds-store-mode] [--no-env-hint]` | Resolve the config from the environment and write each tool's file: `auth.json`, `.npmrc`, `.docker/config.json`, `.git-credentials`, `.netrc`. Atomic, owner-only, merged into what is there (other entries kept), a refused malformed file left untouched. | U·E·F |
| `token apply --dry-run` / `--exports` | The resolved credentials as masked JSON, or `COMPOSER_AUTH` as a shell export; nothing written. | U·F |
| `token set --target\|--service --server [--username] [--scope] [--project]` | Add one token, or replace the one for that host (how a token is rotated or edited), from stdin or a hidden prompt. | U·E·F |
| `token list [--target]... [--project] [--json] [--reveal [-y]]` | What is installed, secrets masked unless revealed. | U·E·F |
| `token show TARGET HOST [--project] [--json] [--reveal [-y]]` | One installed entry in full: tool, host, kind, user, file. | U·E |
| `token status [--config] [--project] [--json]` | Which config is used and why, each file and its mode, entries per tool and how many resolve now, with the variables `apply` would skip. Never fails. | U·E |
| `token copy TARGET HOST [--project]` | Copy one token to the clipboard without printing it. | U·F |
| `token delete TARGET HOST [--project] [-y]` | Delete one token from its file; asks first at a terminal. | U·E·F |
| `token audit [--target]... [--config] [--project]` | Owner-only modes and `.gitignore` coverage; writes nothing; non-zero when something needs fixing. | U·E·F |
| `token examples [--target]... [--project] [--json]` | Each file's shape with placeholder values. | U·F |
| `token init [FILE] [--target]...` | A starter config, never over an existing file. | U·F |
| `token providers` | The Composer services a config can name, and the well-known npm, Docker and git hosts. | U·F |
| `token import [FILE] [--dry-run]` | Copy an auth-token-manager (or any) token config to `tokens.json`, after parsing it. | U·F |
| `token shell-aliases [--install\|--uninstall\|--print] [--file] [--login] [--no-backup] [--quiet]` | The `atm`, `atm-ui`, `atm-doctor`, `atm-init` aliases. | U·F |

Docker registries kept in a credential store are logged in with
`docker login --password-stdin` or refused, never written inline where Docker
would ignore them. Values that could inject a second credential (a newline in a
token, whitespace in a host) are refused before anything is written. Under
`--dev-root` every file lands in the sandbox and `docker login` is refused.

## Providers

Config-driven (`providers.json`) and pluggable: GitHub & GitLab (CLI + token, cloud
and enterprise/self-hosted); Bitbucket, Gitea, Codeberg, Forgejo, Gogs, SourceHut,
Azure DevOps, AWS CodeCommit (web-panel); cloud-VPS account keys for DigitalOcean,
Vultr, Hetzner, Linode, Scaleway (REST); `generic-ssh`; and **any** REST key API with
no code via `kind: rest`. Details in [tools/providers.md](tools/providers.md) and
[tools/vps.md](tools/vps.md).

## Confirmation

Every verb that changes `~/.ssh` confirms first, and `--yes`/`-y` answers in
advance. `reconcile` prints its dry run as the question, so what is approved is
the actual change rather than the name of a command.

The question is only asked when standard input is a terminal. Without one the
command proceeds: there is nobody to ask, and refusing would make every existing
script a silent no-op.

## Safety guarantees (always on)

- **One renderer** drives `render`/`check`/`reconcile`; `check` compares byte-for-byte.
- **Atomic + locked** state writes (temp + `os.replace` under an advisory lock).
- **Load-bearing perms** (dirs 700, private keys + config 600, public + known_hosts 644),
  set on create and re-asserted by `doctor`/`reconcile`; secrets are owner-only the
  moment they appear.
- **Snapshot before mutate** - every mutating command backs up `~/.ssh` first.
- The `.env` is gitignored, 0600, and excluded from the encrypted bundle.
- **Token files are owner-only** (0600) from the moment they are written, secrets are
  never printed unless `--reveal` is confirmed, and history records which file and host
  changed, never the value.

---

[← Docs index](../README.md#documentation)
