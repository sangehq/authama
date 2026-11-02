# Command reference

<!-- Generated from the command tree by TestCLIReferenceIsCurrent in
     src/internal/cli/clireference_test.go. Do not edit by hand:
     UPDATE_CLI_REFERENCE=1 go test ./internal/cli/ -run TestCLIReferenceIsCurrent -->

Every `authama` command, with its flags and examples, exactly as `--help` prints
them. `sshmgr`, the deprecated former name, accepts the same commands. The feature
pages under [docs/tools](../tools/) explain what each one is for; this page is the
complete list. [Choosing between similar commands](#choosing-between-similar-commands)
is at the end.

Every command also takes the global flag:

```
      --dev-root string   run against a sandbox directory instead of ~/.ssh (also $AUTHAMA_DEV_ROOT, or the legacy $SSHMGR_DEV_ROOT)
      --no-dotenv         do not read <config home>/.env this run (also $AUTHAMA_NO_DOTENV=1)
```

## Contents

- [`authama doctor`](#authama-doctor) - Diagnose deps, perms, agent, known_hosts, drift, dangling keys, and token-file perms
- [`authama history`](#authama-history) - List the changes authama has made, newest first
  - [`authama history show`](#authama-history-show) - Show one operation: its events and what it changed in the manifest
- [`authama init`](#authama-init) - Create/converge the per-user home
- [`authama migrate`](#authama-migrate) - Copy a pre-0.1.1 home (ssh-manager, sshmgr) into the authama home; the old one is kept and marked
- [`authama serve`](#authama-serve) - Open the web UI on this machine
- [`authama settings`](#authama-settings) - Your settings files (.env and authama.json): where they are, what is in effect, validate, create
  - [`authama settings init`](#authama-settings-init) - Create a starter authama.json (schema reference and version; every setting at its default)
  - [`authama settings path`](#authama-settings-path) - Print where the config home, state home, .env and authama.json are
  - [`authama settings show`](#authama-settings-show) - Print authama.json, or with --effective every setting's value and where it came from
  - [`authama settings validate`](#authama-settings-validate) - Check authama.json (or FILE) against the schema, and .env's syntax and permissions
- [`authama ssh`](#authama-ssh) - SSH keys, profiles, hosts, config and trust (every former top-level verb; the bare top-level spelling still works, hidden)
  - [`authama ssh audit`](#authama-ssh-audit) - Deployment, expiry, and hygiene report
  - [`authama ssh bundle`](#authama-ssh-bundle) - Write an encrypted backup of keys + state (read back with restore)
  - [`authama ssh clean`](#authama-ssh-clean) - Remove stale known_hosts pins, stale records and write residue
  - [`authama ssh config`](#authama-ssh-config) - Render, check, or show the SSH config
    - [`authama ssh config check`](#authama-ssh-config-check) - Verify the config matches the manifest (read-only; exit non-zero on drift)
    - [`authama ssh config export`](#authama-ssh-config-export) - Write the config as one file for another tool (never touches ~/.ssh)
    - [`authama ssh config render`](#authama-ssh-config-render) - Render the config files from the manifest
    - [`authama ssh config show`](#authama-ssh-config-show) - Print the rendered config, or ssh -G for one alias
  - [`authama ssh deploy`](#authama-ssh-deploy) - Install a public key on its target
  - [`authama ssh diff`](#authama-ssh-diff) - Preview what reconcile would change (manifest vs. ~/.ssh)
  - [`authama ssh expiry`](#authama-ssh-expiry) - Per-key rotation-age table
  - [`authama ssh git`](#authama-ssh-git) - Show or switch checkouts' GitHub remotes between profile SSH, plain SSH and HTTPS
    - [`authama ssh git status`](#authama-ssh-git-status) - Show each checkout's remote mode, owner and profile
    - [`authama ssh git use`](#authama-ssh-git-use) - Point checkouts' origin at a mode, after proving the new URL with ls-remote
  - [`authama ssh host`](#authama-ssh-host) - Manage a host within a profile (add/edit/delete/apply)
    - [`authama ssh host add`](#authama-ssh-host-add) - Add a host to a profile
    - [`authama ssh host apply`](#authama-ssh-host-apply) - Add or update hosts from compact specs (one, many, or a file)
    - [`authama ssh host delete`](#authama-ssh-host-delete) - Delete a host, its config block and its pins
    - [`authama ssh host edit`](#authama-ssh-host-edit) - Edit a host
  - [`authama ssh import`](#authama-ssh-import) - Onboard an existing ~/.ssh into the manifest
  - [`authama ssh key`](#authama-ssh-key) - Manage a profile's keys (add/list/delete)
    - [`authama ssh key add`](#authama-ssh-key-add) - Declare a key on a profile and mint it
    - [`authama ssh key delete`](#authama-ssh-key-delete) - Delete a key (refuses while a host uses it)
    - [`authama ssh key list`](#authama-ssh-key-list) - Every key with its fingerprint, expiry, hosts and state
    - [`authama ssh key regen-pub`](#authama-ssh-key-regen-pub) - Rebuild public keys from their private key or 1Password item
  - [`authama ssh keygen`](#authama-ssh-keygen) - Generate a profile's or host's keys
  - [`authama ssh knownhosts`](#authama-ssh-knownhosts) - Pin host keys into the single ~/.ssh/known_hosts trust store
    - [`authama ssh knownhosts init`](#authama-ssh-knownhosts-init) - Pin reachable hosts into known_hosts (TOFU; fingerprints reported)
    - [`authama ssh knownhosts pin`](#authama-ssh-knownhosts-pin) - Seed known_hosts via ssh-keyscan, with confirmation
  - [`authama ssh list`](#authama-ssh-list) - Filterable tree across profiles
  - [`authama ssh load`](#authama-ssh-load) - Add a profile's keys to the agent
  - [`authama ssh net`](#authama-ssh-net) - Per-host connection status + VPN indicator
  - [`authama ssh notify`](#authama-ssh-notify) - Manage the scheduled expiry notifier
    - [`authama ssh notify install`](#authama-ssh-notify-install) - Install the scheduled expiry notifier
    - [`authama ssh notify test`](#authama-ssh-notify-test) - Fire a test notification
  - [`authama ssh profile`](#authama-ssh-profile) - Manage a profile (add/edit/delete)
    - [`authama ssh profile add`](#authama-ssh-profile-add) - Add a profile
    - [`authama ssh profile delete`](#authama-ssh-profile-delete) - Delete a profile, its config blocks and its host pins
    - [`authama ssh profile edit`](#authama-ssh-profile-edit) - Edit a profile
  - [`authama ssh providers`](#authama-ssh-providers) - List the active provider catalog + credential state
  - [`authama ssh reconcile`](#authama-ssh-reconcile) - Build ~/.ssh from the manifest
  - [`authama ssh recover`](#authama-ssh-recover) - Print a break-glass snippet for regaining access when a key is lost
  - [`authama ssh restore`](#authama-ssh-restore) - Restore keys + state from an encrypted bundle (see bundle)
  - [`authama ssh rollback`](#authama-ssh-rollback) - Undo a rotation: put a key's previous version back
  - [`authama ssh rotate`](#authama-ssh-rotate) - Zero-downtime staged key rotation
  - [`authama ssh show`](#authama-ssh-show) - Everything about a profile, host or key: files, config blocks, pins (the inventory view)
  - [`authama ssh snapshots`](#authama-ssh-snapshots) - List/restore/prune local ~/.ssh backups
    - [`authama ssh snapshots list`](#authama-ssh-snapshots-list) - List local ~/.ssh snapshots (oldest -> newest)
    - [`authama ssh snapshots prune`](#authama-ssh-snapshots-prune) - Prune old snapshots, keeping the most recent N
    - [`authama ssh snapshots restore`](#authama-ssh-snapshots-restore) - Restore ~/.ssh config from a snapshot (private keys are left as they are)
  - [`authama ssh termius`](#authama-ssh-termius) - Export hosts for the Termius app
    - [`authama ssh termius export`](#authama-ssh-termius-export) - Write a Termius CSV import of every host, and list the key to attach to each
    - [`authama ssh termius keys`](#authama-ssh-termius-keys) - List the Termius key label for each host, or copy one host's private key to the clipboard
  - [`authama ssh validate`](#authama-ssh-validate) - Check keypairs parse, match, and have safe perms
  - [`authama ssh view`](#authama-ssh-view) - One host as ssh will use it: resolved config, key and deployment status (the connect view)
  - [`authama ssh vm`](#authama-ssh-vm) - Local VMs as SSH hosts: list them, add one, and connect through the hypervisor
    - [`authama ssh vm add`](#authama-ssh-vm-add) - Add a VM as a host: its address comes from the hypervisor at connect time
    - [`authama ssh vm docker-context`](#authama-ssh-vm-docker-context) - Create or update a Docker context ssh://<alias> for a VM host
    - [`authama ssh vm list`](#authama-ssh-vm-list) - List every hypervisor's VMs, and which manifest host uses each
- [`authama token`](#authama-token) - Package-manager and git auth tokens: Composer, npm, Docker, git, netrc
  - [`authama token apply`](#authama-token-apply) - Resolve the token config from the environment and write each tool's credential file
  - [`authama token audit`](#authama-token-audit) - Check the credential files are owner-only and .gitignore covers them; write nothing
  - [`authama token copy`](#authama-token-copy) - Copy one installed token to the clipboard without printing it
  - [`authama token delete`](#authama-token-delete) - Delete one installed token from its tool's credential file
  - [`authama token examples`](#authama-token-examples) - Show what each tool's credential file looks like, with placeholder values
  - [`authama token import`](#authama-token-import) - Copy an auth-token-manager (or any) token config to tokens.json, so the old location stops being read
  - [`authama token init`](#authama-token-init) - Write a starter token config (never over an existing file)
  - [`authama token list`](#authama-token-list) - List the tokens installed in the credential files (secrets masked)
  - [`authama token providers`](#authama-token-providers) - List the Composer services and the known registries and git hosts a token can name
  - [`authama token set`](#authama-token-set) - Add or replace one token in its tool's credential file (the secret is read from stdin or a hidden prompt)
  - [`authama token shell-aliases`](#authama-token-shell-aliases) - Show, install or remove the atm shell aliases (atm, atm-ui, atm-doctor, atm-init)
  - [`authama token show`](#authama-token-show) - Everything about one installed token: tool, host, kind, user and file (secret masked)
  - [`authama token status`](#authama-token-status) - Which config is in use, where each tool's file is, and which variables are missing; write nothing
- [`authama tui`](#authama-tui) - Interactive menu over the manager
- [`authama version`](#authama-version) - Print the authama version

## authama doctor

Diagnose deps, perms, agent, known_hosts, drift, dangling keys, and token-file perms.

```
authama doctor [flags]
```

Examples:

```sh
  authama doctor
  authama doctor --fix
  authama doctor --strict --json   # CI: fail on anything worth fixing
```

Flags:

```
      --fix      auto-fix perms first
      --json     machine-readable output (scripting)
      --strict   for CI: treat every dangling-key state as a failure, not only the blocking ones, and fail on a token file others can read
```

## authama history

List the operations that changed something - every command that took the lock -
newest first, with how each ended and what it was about.

The first run imports what was already on disk: audit.log, the dates inventory.json
records, and the snapshots. Those show as source "backfill".

--since and --until take a date (2026-10-27), a time (2026-10-27T14:00:00Z) or an
age (36h, 7d).

```
authama history [flags]
```

Subcommands:

- [`show`](#authama-history-show) - Show one operation: its events and what it changed in the manifest

Examples:

```sh
  authama history
  authama history --since 7d --command "host add"
  authama history --host web1 --json
```

Flags:

```
      --command string   only this command ("host add"), or a group of them ("host")
      --host string      only operations about this host alias
      --json             one JSON object per operation, its events nested
      --key string       only operations about this key
      --limit int        at most this many operations (0 for all) (default 50)
      --profile string   only operations about this profile
      --since string     only operations started at or after this
      --source string    only operations from cli, tui, web or backfill
      --until string     only operations started at or before this
```

## authama history show

Show one operation: its events and what it changed in the manifest.

```
authama history show <op_id>
```

Examples:

```sh
  authama history show 224f5bf1   # an OP id from history
```

## authama init

Create/converge the per-user home.

```
authama init [flags]
```

Examples:

```sh
  authama init
  authama init --force --backup   # reset manifest, inventory and .env, keeping copies
```

Flags:

```
      --backup   with --force, copy the old files into the state home's .state/ before overwriting
  -f, --force    overwrite manifest/inventory/.env with fresh defaults
```

## authama migrate

Copy a pre-0.1.1 home (ssh-manager, sshmgr) into the authama home; the old one is kept and marked.

```
authama migrate [flags]
```

Examples:

```sh
  authama migrate
  authama migrate --force --yes
```

Flags:

```
  -f, --force   if both the legacy and standard home exist, back up the current home and replace it
  -y, --yes     migrate without confirming (implied when there is no terminal)
```

## authama serve

Open the web UI on this machine: an overview, the profiles, hosts and keys, the
snapshots, the operation history, and the doctor and audit reports.

Changes - adding, editing and deleting profiles, hosts and keys, and re-rendering
the config - are previewed first: the page shows exactly what each would write,
and confirming makes it only if nothing moved since. The lock is held only while
a change is checked and made, so a running server does not hold up a command.
--read-only serves no change at all.

Listens on 127.0.0.1, port 7822 by default. If that port is taken it takes the
next free one and prints the address it bound; --port pins a port and fails if it
is taken. The printed address carries a one-time token that signs the browser in;
without it every page answers 401.

A non-loopback --addr is refused unless --allow-remote is passed. Prefer an SSH
tunnel: ssh -L 7822:127.0.0.1:7822 <host>.

```
authama serve [flags]
```

Examples:

```sh
  authama serve
  authama serve --read-only --no-open
  authama serve --port 8765 --token-file ~/.cache/authama-url
```

Flags:

```
      --addr string              address to listen on, host or host:port (a port here pins it, like --port) (default "127.0.0.1")
      --allow-host stringArray   extra Host header to accept with --allow-remote, e.g. a proxy's name (repeatable)
      --allow-remote             allow a non-loopback --addr (plain HTTP; prefer an SSH tunnel)
      --no-open                  do not open a browser
      --port int                 port to listen on; when not given and the default is taken, the next free port is used (default 7822)
      --read-only                serve no route that changes anything, and show no form
      --token-file string        also write the sign-in address to this file, mode 0600, for scripts
```

## authama settings

authama reads two files from its config home at start-up:

  .env           KEY=value lines: tokens, and any AUTHAMA_* setting
  authama.json   settings, checked against a schema (https://raw.githubusercontent.com/simtabi/authama/main/docs/schema/authama.schema.json)

Precedence: command-line flags > environment > .env > authama.json > defaults.
A variable already in the environment is never replaced by a file; --no-dotenv
or $AUTHAMA_NO_DOTENV=1 skips .env for a run.

```
authama settings
```

Subcommands:

- [`init`](#authama-settings-init) - Create a starter authama.json (schema reference and version; every setting at its default)
- [`path`](#authama-settings-path) - Print where the config home, state home, .env and authama.json are
- [`show`](#authama-settings-show) - Print authama.json, or with --effective every setting's value and where it came from
- [`validate`](#authama-settings-validate) - Check authama.json (or FILE) against the schema, and .env's syntax and permissions

## authama settings init

Create a starter authama.json (schema reference and version; every setting at its default).

```
authama settings init [flags]
```

Examples:

```sh
  authama settings init
  authama settings init --force
```

Flags:

```
  -f, --force   replace an existing authama.json
```

## authama settings path

Print where the config home, state home, .env and authama.json are.

```
authama settings path [flags]
```

Examples:

```sh
  authama settings path
  authama settings path --json
```

Flags:

```
      --json   machine-readable output (scripting)
```

## authama settings show

Print authama.json, or with --effective every setting's value and where it came from.

```
authama settings show [flags]
```

Examples:

```sh
  authama settings show
  authama settings show --effective   # every setting, its value and where it came from
```

Flags:

```
      --effective   every setting with its value in effect and its source
      --json        machine-readable output (with --effective)
```

## authama settings validate

Check authama.json (or FILE) against the schema, and .env's syntax and permissions.

```
authama settings validate [FILE]
```

Examples:

```sh
  authama settings validate
  authama settings validate ./authama.json
```

## authama ssh

SSH keys, profiles, hosts, config and trust (every former top-level verb; the bare top-level spelling still works, hidden).

```
authama ssh
```

Subcommands:

- [`audit`](#authama-ssh-audit) - Deployment, expiry, and hygiene report
- [`bundle`](#authama-ssh-bundle) - Write an encrypted backup of keys + state (read back with restore)
- [`clean`](#authama-ssh-clean) - Remove stale known_hosts pins, stale records and write residue
- [`config`](#authama-ssh-config) - Render, check, or show the SSH config
- [`deploy`](#authama-ssh-deploy) - Install a public key on its target
- [`diff`](#authama-ssh-diff) - Preview what reconcile would change (manifest vs. ~/.ssh)
- [`expiry`](#authama-ssh-expiry) - Per-key rotation-age table
- [`git`](#authama-ssh-git) - Show or switch checkouts' GitHub remotes between profile SSH, plain SSH and HTTPS
- [`host`](#authama-ssh-host) - Manage a host within a profile (add/edit/delete/apply)
- [`import`](#authama-ssh-import) - Onboard an existing ~/.ssh into the manifest
- [`key`](#authama-ssh-key) - Manage a profile's keys (add/list/delete)
- [`keygen`](#authama-ssh-keygen) - Generate a profile's or host's keys
- [`knownhosts`](#authama-ssh-knownhosts) - Pin host keys into the single ~/.ssh/known_hosts trust store
- [`list`](#authama-ssh-list) - Filterable tree across profiles
- [`load`](#authama-ssh-load) - Add a profile's keys to the agent
- [`net`](#authama-ssh-net) - Per-host connection status + VPN indicator
- [`notify`](#authama-ssh-notify) - Manage the scheduled expiry notifier
- [`profile`](#authama-ssh-profile) - Manage a profile (add/edit/delete)
- [`providers`](#authama-ssh-providers) - List the active provider catalog + credential state
- [`reconcile`](#authama-ssh-reconcile) - Build ~/.ssh from the manifest
- [`recover`](#authama-ssh-recover) - Print a break-glass snippet for regaining access when a key is lost
- [`restore`](#authama-ssh-restore) - Restore keys + state from an encrypted bundle (see bundle)
- [`rollback`](#authama-ssh-rollback) - Undo a rotation: put a key's previous version back
- [`rotate`](#authama-ssh-rotate) - Zero-downtime staged key rotation
- [`show`](#authama-ssh-show) - Everything about a profile, host or key: files, config blocks, pins (the inventory view)
- [`snapshots`](#authama-ssh-snapshots) - List/restore/prune local ~/.ssh backups
- [`termius`](#authama-ssh-termius) - Export hosts for the Termius app
- [`validate`](#authama-ssh-validate) - Check keypairs parse, match, and have safe perms
- [`view`](#authama-ssh-view) - One host as ssh will use it: resolved config, key and deployment status (the connect view)
- [`vm`](#authama-ssh-vm) - Local VMs as SSH hosts: list them, add one, and connect through the hypervisor

## authama ssh audit

Deployment, expiry, and hygiene report.

```
authama ssh audit [flags]
```

Examples:

```sh
  authama ssh audit
  authama ssh audit --notify
```

Flags:

```
      --notify   also fire the cadence-gated desktop alert
```

## authama ssh bundle

Encrypted backup of keys + state.

Old bundles are kept, not pruned: they are encrypted, and silently deleting the
only copy of a key would be worse than letting them accumulate. Pass --keep to
trim the destination once the new bundle is written.

```
authama ssh bundle [flags]
```

Examples:

```sh
  authama ssh bundle
  authama ssh bundle --recipient age1... --output /Volumes/backup --keep 5
```

Flags:

```
      --keep int           after bundling, keep only the newest N bundles (0 keeps all)
  -o, --output string      destination dir (else config-dir/dist)
  -r, --recipient string   age recipient (else $AUTHAMA_AGE_RECIPIENT)
```

## authama ssh clean

Remove stale known_hosts pins, stale inventory records and write residue.

--adopt first tags the untagged pins that match a manifest host, putting
them under authama's management so they are pruned when their host goes
away. It is opt-in: an untagged pin is presumed to be yours.

```
authama ssh clean [flags]
```

Examples:

```sh
  authama ssh clean --dry-run
  authama ssh clean --adopt --yes
```

Flags:

```
      --adopt     put untagged pins matching a manifest host under authama's management
      --dry-run   report what would change, touch nothing
  -y, --yes       clean without confirming (implied when there is no terminal)
```

## authama ssh config

Render, check, or show the SSH config.

```
authama ssh config
```

Subcommands:

- [`check`](#authama-ssh-config-check) - Verify the config matches the manifest (read-only; exit non-zero on drift)
- [`export`](#authama-ssh-config-export) - Write the config as one file for another tool (never touches ~/.ssh)
- [`render`](#authama-ssh-config-render) - Render the config files from the manifest
- [`show`](#authama-ssh-config-show) - Print the rendered config, or ssh -G for one alias

## authama ssh config check

Verify the config matches the manifest (read-only; exit non-zero on drift).

```
authama ssh config check
```

Examples:

```sh
  authama ssh config check   # exit 1 when the files drift from the manifest
```

## authama ssh config export

Write a copy of the managed config for a tool that cannot read the split layout.

--flat puts every host inline, with absolute key paths and no Include or Match, in
<config home>/exports/ssh_config (or --out, or - for stdout). The files
`config render` writes under ~/.ssh are left exactly as they are: no target inside ~/.ssh is accepted.

```
authama ssh config export [flags]
```

Examples:

```sh
  authama ssh config export --flat --out -
  authama ssh config export --out ./ssh_config
```

Flags:

```
      --flat         every host inline in one file, absolute paths, no Include
  -o, --out string   where to write (default <config home>/exports/ssh_config; - for stdout)
```

## authama ssh config render

Render the config files from the manifest.

```
authama ssh config render [flags]
```

Examples:

```sh
  authama ssh config render --dry-run
  authama ssh config render --yes
```

Flags:

```
  -n, --dry-run   preview changes without writing
  -y, --yes       render without confirming (implied when there is no terminal)
```

## authama ssh config show

Print the rendered config, or ssh -G for one alias.

```
authama ssh config show [alias]
```

Examples:

```sh
  authama ssh config show
  authama ssh config show web1   # what ssh -G resolves for one alias
```

## authama ssh deploy

Install a public key on its target.

```
authama ssh deploy <[profile/]key> [target] [flags]
```

Examples:

```sh
  authama ssh deploy work/github
  authama ssh deploy web1 --yes
```

Flags:

```
  -y, --yes   deploy without confirming (implied when there is no terminal)
```

## authama ssh diff

Preview what reconcile would change (manifest vs. ~/.ssh).

```
authama ssh diff
```

Examples:

```sh
  authama ssh diff
```

## authama ssh expiry

Per-key rotation-age table.

```
authama ssh expiry
```

Examples:

```sh
  authama ssh expiry
```

## authama ssh git

Each checkout's origin reaches GitHub one of three ways:

  profile  git@github-<profile>:owner/repo.git  the profile's own key (default)
  ssh      git@github.com:owner/repo.git         the key bound to Host github.com
  https    https://github.com/owner/repo.git     your git credential helper

The owner picks the profile through defaults.git.orgs, then defaults.git.profile.

```
authama ssh git
```

Subcommands:

- [`status`](#authama-ssh-git-status) - Show each checkout's remote mode, owner and profile
- [`use`](#authama-ssh-git-use) - Point checkouts' origin at a mode, after proving the new URL with ls-remote

## authama ssh git status

Show each checkout's remote mode, owner and profile.

```
authama ssh git status [path...]
```

Examples:

```sh
  authama ssh git status
  authama ssh git status ~/code/acme ~/code/personal
```

## authama ssh git use

Point each checkout's origin at a mode. The mode is the first argument when it
names one, else --mode, else $AUTHAMA_GIT_MODE, else defaults.git.mode, else profile.
A new URL is checked with git ls-remote before origin is rewritten, so a checkout
never ends up with a remote it cannot authenticate to. Non-GitHub origins are skipped.

```
authama ssh git use [profile|ssh|https] [path...] [flags]
```

Examples:

```sh
  authama ssh git use profile --dry-run
  authama ssh git use https ~/code/acme/api --yes
  authama ssh git use ssh --profile work
```

Flags:

```
      --dry-run          show what would change, touch nothing
      --mode string      profile, ssh or https (overrides $AUTHAMA_GIT_MODE and the manifest)
      --profile string   use this profile's key instead of the owner's mapping
  -y, --yes              rewrite without confirming (implied when there is no terminal)
```

## authama ssh host

Manage a host within a profile (add/edit/delete/apply).

```
authama ssh host
```

Subcommands:

- [`add`](#authama-ssh-host-add) - Add a host to a profile
- [`apply`](#authama-ssh-host-apply) - Add or update hosts from compact specs (one, many, or a file)
- [`delete`](#authama-ssh-host-delete) - Delete a host, its config block and its pins
- [`edit`](#authama-ssh-host-edit) - Edit a host

## authama ssh host add

Add a host to a profile.

```
authama ssh host add <profile> <alias> [flags]
```

Examples:

```sh
  authama ssh host add work web1 --hostname web1.example.com --user deploy
  authama ssh host add work gh --hostname github.com --user git --provider github --token-env GH_TOKEN_WORK
```

Flags:

```
  -H, --hostname string    host to connect to
      --key-name string    use this key instead of the derived <profile>_<alias>-<algo> name
  -p, --port int           ssh port (default 22)
      --provider string    deployment adapter for this host (see 'authama ssh providers')
      --proxy string       reach the host through a proxy: cloudflared (Cloudflare Access; ssh runs 'cloudflared access ssh --hostname %h')
      --tag stringArray    repeatable
      --token-env string   env var holding the provider API token (e.g. GH_TOKEN_WORK)
  -u, --user string        ssh user
```

## authama ssh host apply

Add or update hosts from compact specs:

    <alias> <host>[:port] <user> [role]

A host that is not in the profile is added (as `host add`); one that is has
its hostname, user and port (when given) set (as `host edit`). The role, and
any --tag, are merged into the host's tags; existing tags are never removed.
Re-running the same specs is a no-op that reports every host unchanged.

Specs come from any mix of:
  - one spec as positional arguments after the profile
  - --host "<spec>", repeatable
  - -f FILE (- for stdin): one spec per line, blank lines and # comments
    skipped; or a JSON array of spec strings or
    {"alias","host","user","port","role","tags"} objects

The batch is checked by the manifest's own validation and saved at once, so one
bad spec changes nothing. --reconcile mints the new hosts' keys afterwards.

```
authama ssh host apply <profile> [<alias> <host>[:port] <user> [role]] [flags]
```

Examples:

```sh
  authama ssh host apply clientco web-01 203.0.113.21 ploi web
  authama ssh host apply clientco \
      --host "web-01 203.0.113.21 ploi web" \
      --host "db-01  203.0.113.22 ploi db" --tag prd --reconcile
  authama ssh host apply clientco -f hosts.txt --create-profile --category clients
  printf 'bastion 10.0.0.5:2222 admin\n' | authama ssh host apply clientco -f - --dry-run
```

Flags:

```
      --category string    category for a profile this creates (implies --create-profile)
      --create-profile     create the profile if it does not exist
      --dry-run            report what would change without writing
  -f, --file string        read specs from a file, one per line or a JSON array (- for stdin)
      --host stringArray   a spec "<alias> <host>[:port] <user> [role]" (repeatable)
      --no-pin             with --reconcile: don't auto-pin reachable hosts' known_hosts
      --reconcile          run reconcile afterwards to mint keys and re-render
      --tag stringArray    extra tag for every host in the batch (repeatable)
  -y, --yes                apply without confirming (implied when there is no terminal)
```

## authama ssh host delete

Delete a host, its config block and its pins.

The key the host used is reported, not assumed: another host may still
use it, or the profile may still declare it. --purge deletes its files
only in the remaining case, where the key left the manifest with the host
and nothing tracks it any more.

```
authama ssh host delete <profile> <alias> [flags]
```

Examples:

```sh
  authama ssh host delete work web1
  authama ssh host delete work web1 --purge --revoke --yes
```

Flags:

```
      --no-key-backup   purge without writing an encrypted backup first (the key is then unrecoverable)
      --purge           also delete the key files, if the host was the only thing naming the key
      --revoke          also revoke the deployed key from targets (with --yes)
  -y, --yes             skip confirmation prompts
```

## authama ssh host edit

Edit a host.

```
authama ssh host edit <profile> <alias> [flags]
```

Examples:

```sh
  authama ssh host edit work web1 --port 2222
  authama ssh host edit work web1 --tag prod --untag staging
```

Flags:

```
  -H, --hostname string     host to connect to
      --key-name string     use this key instead of the derived <profile>_<alias>-<algo> name
  -p, --port int            ssh port (default 22)
      --provider string     deployment adapter for this host (see 'authama ssh providers')
      --proxy string        reach the host through a proxy: cloudflared (Cloudflare Access; ssh runs 'cloudflared access ssh --hostname %h'); "none" removes it
      --tag stringArray     replace the host's tags with these (repeatable); --tag '' removes them all
      --token-env string    env var holding the provider API token (e.g. GH_TOKEN_WORK)
      --untag stringArray   remove this tag (repeatable; applied after --tag; a tag the host lacks is ignored)
  -u, --user string         ssh user
```

## authama ssh import

Onboard an existing ~/.ssh into the manifest.

```
authama ssh import [path] [flags]
```

Examples:

```sh
  authama ssh import --dry-run
  authama ssh import ~/old-laptop/.ssh --yes
```

Flags:

```
      --dry-run   parse and report without writing
  -f, --force     replace an existing non-empty manifest (backed up first)
  -y, --yes       import without confirming (implied when there is no terminal)
```

## authama ssh key

Manage a profile's keys (add/list/delete).

```
authama ssh key
```

Subcommands:

- [`add`](#authama-ssh-key-add) - Declare a key on a profile and mint it
- [`delete`](#authama-ssh-key-delete) - Delete a key (refuses while a host uses it)
- [`list`](#authama-ssh-key-list) - Every key with its fingerprint, expiry, hosts and state
- [`regen-pub`](#authama-ssh-key-regen-pub) - Rebuild public keys from their private key or 1Password item

## authama ssh key add

Declare a key on a profile and mint it.

--host wires the key to an existing host in the same profile, which is
what makes it a rendered IdentityFile. Without it the key is minted but
UNWIRED: it exists on disk and nothing uses it.

```
authama ssh key add <profile> <name> [flags]
```

Examples:

```sh
  authama ssh key add work deploy --type ed25519
  authama ssh key add work deploy --host web1 --passphrase
```

Flags:

```
      --host string             wire an existing host in this profile to the key
      --passphrase              protect the new key (prompts without echo)
      --rotate-after-days int   rotation interval for this key (default: the manifest's defaults.rotate_after_days)
      --type string             key algorithm (default: the manifest's defaults.key_type)
```

## authama ssh key delete

Delete a key (refuses while a host uses it).

Without --purge the manifest declaration and the inventory record go away
and the key files stay on disk, so an accidental delete costs nothing.
--purge deletes the files too, after writing an encrypted backup.

```
authama ssh key delete <profile/key> [flags]
```

Examples:

```sh
  authama ssh key delete work/deploy
  authama ssh key delete work/deploy --revoke --purge --yes
```

Flags:

```
      --no-key-backup   purge without writing an encrypted backup first (the key is then unrecoverable)
      --purge           also delete the key files from disk
      --revoke          revoke the deployed public key from its targets first
  -y, --yes             skip the confirmation prompt
```

## authama ssh key list

Every key with its fingerprint, expiry, hosts and state.

```
authama ssh key list [profile|alias|key]
```

Examples:

```sh
  authama ssh key list
  authama ssh key list work
```

## authama ssh key regen-pub

Rebuild each selected key's .pub from the key it belongs to: the private key
file (ssh-keygen -y), or the vault item for a profile whose key_store is
1password. Repairs a missing or wrong .pub; never changes a private key.

A .pub that was a different key is reported as rewritten, with a reminder to
redeploy. A private key with a passphrase is skipped, with the command to
derive it by hand.

```
authama ssh key regen-pub [<[profile/]key>|profile|alias ...] [--category <name>] [--all] [flags]
```

Examples:

```sh
  authama ssh key regen-pub work/deploy
  authama ssh key regen-pub --all --dry-run
```

Flags:

```
      --all               every key in the manifest
      --category string   every key in profiles of this category
      --dry-run           report what would change, and change nothing
  -y, --yes               skip the confirmation prompt
```

## authama ssh keygen

Generate a profile's or host's keys.

```
authama ssh keygen <profile|alias> [flags]
```

Examples:

```sh
  authama ssh keygen work
  authama ssh keygen web1 --force --yes
```

Flags:

```
  -f, --force           overwrite existing keys (prompts; replaced keys move to old/)
      --no-key-backup   overwrite without writing an encrypted backup first (any earlier predecessor is lost)
      --no-pin          don't auto-pin reachable hosts' known_hosts
      --passphrase      protect newly minted keys (prompts without echo)
  -y, --yes             answer yes to overwrite prompts
```

## authama ssh knownhosts

Pin host keys into the single ~/.ssh/known_hosts trust store.

```
authama ssh knownhosts
```

Subcommands:

- [`init`](#authama-ssh-knownhosts-init) - Pin reachable hosts into known_hosts (TOFU; fingerprints reported)
- [`pin`](#authama-ssh-knownhosts-pin) - Seed known_hosts via ssh-keyscan, with confirmation

## authama ssh knownhosts init

Pin reachable hosts into known_hosts (TOFU; fingerprints reported).

PROFILE and --all select which manifest hosts to scan, not a separate
file: every host is pinned into the one ~/.ssh/known_hosts store.

```
authama ssh knownhosts init [profile] [flags]
```

Examples:

```sh
  authama ssh knownhosts init work
  authama ssh knownhosts init --all --yes
```

Flags:

```
      --all     scan every profile's hosts
      --force   re-scan already-trusted hosts and add any new keys
  -y, --yes     pin without confirming (implied when there is no terminal)
```

## authama ssh knownhosts pin

Seed known_hosts via ssh-keyscan, with confirmation.

```
authama ssh knownhosts pin [host] [flags]
```

Examples:

```sh
  authama ssh knownhosts pin web1
  authama ssh knownhosts pin git.example.com --port 2222
  authama ssh knownhosts pin --all --yes
```

Flags:

```
      --all        pin every host in the manifest
  -p, --port int   port for an unmanaged host (default 22)
  -y, --yes        trust scanned keys without prompting
```

## authama ssh list

Filterable tree across profiles.

```
authama ssh list [flags]
```

Examples:

```sh
  authama ssh list
  authama ssh list --profile work --tag prod
  authama ssh list --type vcs
```

Flags:

```
      --profile string    filter to one profile
      --provider string   filter by provider
      --tag string        filter by host tag
      --type string       provider category, e.g. vcs
```

## authama ssh load

Add a profile's keys to the agent.

```
authama ssh load <profile>
```

Examples:

```sh
  authama ssh load work
```

## authama ssh net

Per-host connection status + VPN indicator.

```
authama ssh net [selector]
```

Examples:

```sh
  authama ssh net
  authama ssh net work
```

## authama ssh notify

Manage the scheduled expiry notifier.

```
authama ssh notify
```

Subcommands:

- [`install`](#authama-ssh-notify-install) - Install the scheduled expiry notifier
- [`test`](#authama-ssh-notify-test) - Fire a test notification

## authama ssh notify install

Install the scheduled expiry notifier.

```
authama ssh notify install [flags]
```

Examples:

```sh
  authama ssh notify install --yes
```

Flags:

```
  -y, --yes   install without confirming (implied when there is no terminal)
```

## authama ssh notify test

Fire a test notification.

```
authama ssh notify test
```

Examples:

```sh
  authama ssh notify test
```

## authama ssh profile

Manage a profile (add/edit/delete).

```
authama ssh profile
```

Subcommands:

- [`add`](#authama-ssh-profile-add) - Add a profile
- [`delete`](#authama-ssh-profile-delete) - Delete a profile, its config blocks and its host pins
- [`edit`](#authama-ssh-profile-edit) - Edit a profile

## authama ssh profile add

Add a profile.

```
authama ssh profile add <name> [flags]
```

Examples:

```sh
  authama ssh profile add work
  authama ssh profile add personal --shared --key-name personal --category home
```

Flags:

```
      --category string      file the profile under profiles/<category>/ and render its hosts into profiles/<category>/config
      --key-comment string   comment template for keys minted from now on: {profile} {alias} {key} {date} {device} (default {profile}/{alias} {date})
      --key-name string      profile key name (shared scope)
      --key-scope string     per_service | shared (shared: one key, --key-name, for every host) (default "per_service")
      --shared               shorthand for --key-scope shared (--shared=false: per_service)
```

## authama ssh profile delete

Delete a profile, its config blocks and its host pins.

The manifest, the inventory records, the rendered Host blocks and any
known_hosts pin no surviving host still needs all go in one step. Key
files stay on disk unless --purge, which deletes them after writing an
encrypted backup.

```
authama ssh profile delete <name> [flags]
```

Examples:

```sh
  authama ssh profile delete old
  authama ssh profile delete old --purge --revoke --yes
```

Flags:

```
      --no-key-backup   purge without writing an encrypted backup first (the keys are then unrecoverable)
      --purge           also delete the profile's key files and directory
      --revoke          also revoke deployed keys from targets (with --yes)
  -y, --yes             skip confirmation prompts
```

## authama ssh profile edit

Edit a profile.

```
authama ssh profile edit <name> [flags]
```

Examples:

```sh
  authama ssh profile edit work --category clients
  authama ssh profile edit work --key-scope per_service
```

Flags:

```
      --category string      move the profile under profiles/<category>/ (its key directory and inventory records follow); "" returns it to the flat layout
      --key-comment string   comment template for keys minted from now on: {profile} {alias} {key} {date} {device} (default {profile}/{alias} {date}); "" inherits defaults.key_comment
      --key-name string      the shared key every host in this profile uses (key_scope: shared)
      --key-scope string     per_service | shared
      --shared               shorthand for --key-scope shared (--shared=false: per_service)
```

## authama ssh providers

List the active provider catalog + credential state.

```
authama ssh providers [flags]
```

Examples:

```sh
  authama ssh providers
  authama ssh providers --export
```

Flags:

```
      --export   write the default catalog to <home>/providers.json to customize
      --force    overwrite an existing file (with --export)
```

## authama ssh reconcile

Build ~/.ssh from the manifest.

```
authama ssh reconcile [flags]
```

Examples:

```sh
  authama ssh reconcile --dry-run
  authama ssh reconcile --yes
```

Flags:

```
      --dry-run      preview without writing
      --no-pin       don't auto-pin reachable hosts' known_hosts
      --passphrase   protect newly minted keys (prompts without echo)
  -y, --yes          apply without confirming (implied when there is no terminal)
```

## authama ssh recover

Print a break-glass snippet for regaining access when a key is lost.

```
authama ssh recover [[profile/]key]
```

Examples:

```sh
  authama ssh recover
  authama ssh recover work/deploy
```

## authama ssh restore

Restore keys + state from an encrypted bundle (see bundle).

```
authama ssh restore <bundle.age> [flags]
```

Examples:

```sh
  authama ssh restore ~/backup/authama-20261101-094319.age --identity ~/.age/key.txt
```

Flags:

```
  -i, --identity string   age identity file (else $AUTHAMA_AGE_IDENTITY_FILE)
  -y, --yes               skip the confirmation prompt
```

## authama ssh rollback

Undo a rotation: put a key's previous version back.

```
authama ssh rollback <[profile/]key> [flags]
```

Examples:

```sh
  authama ssh rollback work/deploy
```

Flags:

```
  -y, --yes   skip the confirmation prompt
```

## authama ssh rotate

Zero-downtime staged key rotation.

The key being replaced is archived to the profile's old/ dir, but only one
predecessor is kept - so rotating again discards the one before it for good.
An encrypted bundle is written first unless --no-key-backup is given.

Name one key, several, whole profiles or host aliases, every key in a
category, or --all. Each key rotates on its own: one that fails is left
untouched and the rest still rotate. A key no host uses is skipped, since
there is nothing to deploy it to.

```
authama ssh rotate [<[profile/]key>|profile|alias ...] [--category <name>] [--all] [flags]
```

Examples:

```sh
  authama ssh rotate work/deploy
  authama ssh rotate --all --dry-run
  authama ssh rotate --category clients --yes
```

Flags:

```
      --all                rotate every key a host uses
      --allow-unverified   commit even if a target can't auto-verify
      --category string    rotate every key in profiles of this category
      --dry-run            list the keys that would rotate, and change nothing
      --no-key-backup      rotate without writing an encrypted backup first (the earlier predecessor is lost)
      --passphrase         protect the rotated-in key (prompts without echo)
  -y, --yes                skip the confirmation prompt
```

## authama ssh show

Everything about a profile, host or key: files, config blocks, pins (the inventory view).

```
authama ssh show <profile|alias|profile/key>
```

Examples:

```sh
  authama ssh show work
  authama ssh show web1
  authama ssh show work/deploy
```

## authama ssh snapshots

List/restore/prune local ~/.ssh backups.

```
authama ssh snapshots
```

Subcommands:

- [`list`](#authama-ssh-snapshots-list) - List local ~/.ssh snapshots (oldest -> newest)
- [`prune`](#authama-ssh-snapshots-prune) - Prune old snapshots, keeping the most recent N
- [`restore`](#authama-ssh-snapshots-restore) - Restore ~/.ssh config from a snapshot (private keys are left as they are)

## authama ssh snapshots list

List local ~/.ssh snapshots (oldest -> newest).

```
authama ssh snapshots list
```

Examples:

```sh
  authama ssh snapshots list
```

## authama ssh snapshots prune

Prune old snapshots, keeping the most recent N.

```
authama ssh snapshots prune [flags]
```

Examples:

```sh
  authama ssh snapshots prune --keep 5
```

Flags:

```
      --keep int   how many to retain (default 10)
```

## authama ssh snapshots restore

Restore ~/.ssh config from a snapshot.

Snapshots hold config, known_hosts and public keys - never private keys, since
they are plaintext archives taken on every mutating command. Restoring puts those
files back and leaves everything else on disk untouched, so a key minted after the
snapshot survives the rollback. Use `authama ssh bundle` for encrypted key backups.

```
authama ssh snapshots restore [snapshot] [flags]
```

Examples:

```sh
  authama ssh snapshots restore   # the newest
  authama ssh snapshots restore ssh-20261101-094319.tar.gz --yes
```

Flags:

```
  -y, --yes   skip the confirmation prompt
```

## authama ssh termius

Termius keeps its hosts in its own encrypted vault and offers no command line or
API to add them, and its SSH-config importer does not follow Include, so it cannot
see the split config. `termius export` writes the CSV its importer reads instead.

```
authama ssh termius
```

Subcommands:

- [`export`](#authama-ssh-termius-export) - Write a Termius CSV import of every host, and list the key to attach to each
- [`keys`](#authama-ssh-termius-keys) - List the Termius key label for each host, or copy one host's private key to the clipboard

## authama ssh termius export

Write a Termius CSV import of every host, and list the key to attach to each.

```
authama ssh termius export [flags]
```

Examples:

```sh
  authama ssh termius export --open
  authama ssh termius export --include-git --skip bastion --out ./hosts.csv
```

Flags:

```
      --include-git        also export git hosts (user git, or a github or gitlab provider), which are not shells
      --open               macOS: open Termius and show the file in Finder
  -o, --out string         where to write (default <config home>/exports/termius-hosts.csv; - for stdout)
      --skip stringArray   leave out this alias (repeatable)
```

## authama ssh termius keys

The Termius CSV carries no keys: each one is added in Termius under Keychain > New Key
(Label + Private key) and then chosen on the host. With no ALIAS this lists every
exported host with the label to use and the key file. With an ALIAS it copies that
host's private key to the clipboard (pbcopy, wl-copy/xclip/xsel, or clip) and prints
the label; the key itself is never printed unless --print is given.

```
authama ssh termius keys [ALIAS] [flags]
```

Examples:

```sh
  authama ssh termius keys
  authama ssh termius keys db-01 --clear-after 60s
  authama ssh termius keys --clear
```

Flags:

```
      --clear                  empty the clipboard and exit
      --clear-after duration   after copying, wait this long (e.g. 60s) and empty the clipboard
      --include-git            also list git hosts, as 'termius export --include-git' does (an ALIAS finds a git host without it)
      --print                  print the private key to stdout instead of copying it (warns on stderr)
      --skip stringArray       leave out this alias, as 'termius export --skip' does (repeatable)
```

## authama ssh validate

Validate managed keypairs: each key parses, the public matches the private, and perms are correct. selector filters by profile, key name, or profile/key; a key name shared by several profiles validates all of them. Omit it to validate every managed key. Exits non-zero if any key fails.

```
authama ssh validate [selector]
```

Examples:

```sh
  authama ssh validate
  authama ssh validate work
```

## authama ssh view

One host as ssh will use it: resolved config, key and deployment status (the connect view).

```
authama ssh view <profile|alias>
```

Examples:

```sh
  authama ssh view web1
```

## authama ssh vm

A VM's IP changes, so a VM host renders ProxyCommand "<authama>" vm proxy <alias>:
each connection asks the hypervisor for the VM's current address. Supported:
parallels, vmpal, vmware, utm, lima, orbstack, podman. OrbStack and Podman run their own SSH and keys;
they are listed, and get a Docker context, but authama does not take their keys over.

```
authama ssh vm
```

Subcommands:

- [`add`](#authama-ssh-vm-add) - Add a VM as a host: its address comes from the hypervisor at connect time
- [`docker-context`](#authama-ssh-vm-docker-context) - Create or update a Docker context ssh://<alias> for a VM host
- [`list`](#authama-ssh-vm-list) - List every hypervisor's VMs, and which manifest host uses each

## authama ssh vm add

Add a VM as a host: its address comes from the hypervisor at connect time.

```
authama ssh vm add <profile> <alias> [flags]
```

Examples:

```sh
  authama ssh vm add lab dev --hypervisor orbstack --vm ubuntu --user me
```

Flags:

```
      --hypervisor string   parallels, vmpal, vmware, utm, lima, orbstack, podman
      --key-name string     use this key instead of the derived <profile>_<alias>-<algo> name
  -u, --user string         guest user
      --vm string           the VM's name as the hypervisor lists it (see 'authama ssh vm list')
  -y, --yes                 add without confirming (implied when there is no terminal)
```

## authama ssh vm docker-context

Create or update a Docker context ssh://<alias> for a VM host.

```
authama ssh vm docker-context <alias>
```

Examples:

```sh
  authama ssh vm docker-context dev
```

## authama ssh vm list

List every hypervisor's VMs, and which manifest host uses each.

```
authama ssh vm list
```

Examples:

```sh
  authama ssh vm list
```

## authama token

Builds the credential files Composer, npm, Docker, git and curl read, from a
config that names environment variables (never secrets), and lists, copies or
removes what is installed. Files are written atomically and owner-only.

Config: --config, $AUTHAMA_TOKENS_CONFIG, ./authama-tokens.json, or tokens.json in the config home.

```
authama token
```

Subcommands:

- [`apply`](#authama-token-apply) - Resolve the token config from the environment and write each tool's credential file
- [`audit`](#authama-token-audit) - Check the credential files are owner-only and .gitignore covers them; write nothing
- [`copy`](#authama-token-copy) - Copy one installed token to the clipboard without printing it
- [`delete`](#authama-token-delete) - Delete one installed token from its tool's credential file
- [`examples`](#authama-token-examples) - Show what each tool's credential file looks like, with placeholder values
- [`import`](#authama-token-import) - Copy an auth-token-manager (or any) token config to tokens.json, so the old location stops being read
- [`init`](#authama-token-init) - Write a starter token config (never over an existing file)
- [`list`](#authama-token-list) - List the tokens installed in the credential files (secrets masked)
- [`providers`](#authama-token-providers) - List the Composer services and the known registries and git hosts a token can name
- [`set`](#authama-token-set) - Add or replace one token in its tool's credential file (the secret is read from stdin or a hidden prompt)
- [`shell-aliases`](#authama-token-shell-aliases) - Show, install or remove the atm shell aliases (atm, atm-ui, atm-doctor, atm-init)
- [`show`](#authama-token-show) - Everything about one installed token: tool, host, kind, user and file (secret masked)
- [`status`](#authama-token-status) - Which config is in use, where each tool's file is, and which variables are missing; write nothing

## authama token apply

Reads the token config, fills each entry from the environment variables it names,
and merges the result into each tool's file - entries for other hosts are kept.
--dry-run prints what would be written, secrets masked; --exports prints
COMPOSER_AUTH as a shell export for CI instead of writing anything.

```
authama token apply [flags]
```

Examples:

```sh
  authama token apply
  authama token apply --dry-run --target composer
  authama token apply --source .env --project .
  eval "$(authama token apply --exports)"
```

Flags:

```
      --config string             the token config file (default: discovered)
      --creds-store-mode string   when Docker keeps a registry in a credential store: login (docker login --password-stdin) or refuse
      --dry-run                   print the resolved credentials as JSON, secrets masked; write nothing
      --exports                   print COMPOSER_AUTH as a shell export instead of writing files
      --no-env-hint               do not print the CI hint after writing Composer's file
      --project string            write the project's auth.json and .npmrc instead of the user ones
      --source string             load VAR=value lines from this env file first (set variables win)
      --target strings            only these tools (repeatable: composer, npm, docker, git, netrc)
```

## authama token audit

For each tool (all five, the --target ones, or those a --config uses): is its
file there, and is it owner-only? With --project, the project's auth.json and
.npmrc, and whether the project's .gitignore covers every secret file. Exits
non-zero when anything needs fixing; `authama doctor --fix` tightens the modes.
`token status` is the read-only overview that never fails.

```
authama token audit [flags]
```

Examples:

```sh
  authama token audit
  authama token audit --project . --target composer
```

Flags:

```
      --config string    check the tools this token config uses
      --project string   check the project's files and .gitignore
      --target strings   only these tools (repeatable: composer, npm, docker, git, netrc)
```

## authama token copy

Puts HOST's secret from TARGET's credential file on the clipboard, through
pbcopy, wl-copy, xclip, xsel or clip, and prints only what it copied. Nothing
is written to the terminal, the history or a file.

```
authama token copy TARGET HOST [flags]
```

Examples:

```sh
  authama token copy npm registry.npmjs.org
```

Flags:

```
      --project string   read the project's auth.json or .npmrc instead of the user one
```

## authama token delete

Removes the entry for HOST from TARGET's credential file; entries for other
hosts are kept. At a terminal it asks first (--yes skips the question); without
one it goes ahead, as every other delete does.

```
authama token delete TARGET HOST [flags]
```

Examples:

```sh
  authama token delete npm registry.npmjs.org
  authama token delete composer github.com --project . --yes
```

Flags:

```
      --project string   edit the project's auth.json or .npmrc instead of the user one
  -y, --yes              skip the confirmation prompt
```

## authama token examples

Prints every tool's file filled with placeholders, and writes nothing. With
--project it writes the project-scoped ones (auth.json, .npmrc) into that
directory; the others live in your home directory, where placeholders would
replace real credentials, so they are never written.

```
authama token examples [flags]
```

Examples:

```sh
  authama token examples --target docker
  authama token examples --json
  authama token examples --project ./demo
```

Flags:

```
      --json             print the placeholders as JSON, masked, instead of as files
      --project string   write the placeholder auth.json and .npmrc into this directory
      --target strings   only these tools (repeatable: composer, npm, docker, git, netrc)
```

## authama token import

Without FILE it finds auth-token-manager's config where that tool looked
($AUTH_TOKEN_MANAGER_CONFIG, $COMPOSER_AUTH_CONFIG, ./auth-token-manager.json,
~/.config/auth-token-manager.json). The file is parsed first, never copied
over an existing tokens.json, and left in place: delete it when you are happy.
The format is unchanged, so nothing is converted.

```
authama token import [FILE] [flags]
```

Examples:

```sh
  authama token import
  authama token import ./team-tokens.json --dry-run
```

Flags:

```
      --dry-run   check the file parses and say where it would go; write nothing
```

## authama token init

Write a starter token config (never over an existing file).

```
authama token init [FILE] [flags]
```

Examples:

```sh
  authama token init
  authama token init --target composer --target npm ./authama-tokens.json
```

Flags:

```
      --target strings   start from these tools' templates (repeatable: composer, npm, docker, git, netrc)
```

## authama token list

List the tokens installed in the credential files (secrets masked).

```
authama token list [flags]
```

Examples:

```sh
  authama token list
  authama token list --target npm --target git
  authama token list --project . --json
```

Flags:

```
      --json             print the list as JSON
      --project string   read the project's auth.json and .npmrc instead of the user ones
      --reveal           show the secret values (asks first; --yes when not at a terminal)
      --target strings   only these tools (repeatable: composer, npm, docker, git, netrc)
  -y, --yes              reveal without asking
```

## authama token providers

The token catalog: every Composer service `token set --service` and the
config's "service" key accept, with the auth mechanism each maps to and the
fields it needs; then the well-known hosts for npm, Docker and git (any other
host works too: these are shortcuts, not a whitelist).

```
authama token providers
```

Examples:

```sh
  authama token providers
```

## authama token set

Adds one credential, or replaces the one already saved for that host (so it is
also how a token is rotated or edited). --service picks a Composer service (see
`token providers`); otherwise give --target. Token tools read a token; http-basic,
docker, git and netrc also need --username and read a password or token. The secret is read
from standard input when it is piped, else from a prompt that does not echo; it
is never a flag, so it stays out of shell history and the process list.

```
authama token set [flags]
```

Examples:

```sh
  printf %s "$GH_TOKEN" | authama token set --service github --server github.com
  authama token set --target npm --server npm.pkg.github.com --scope @acme
  authama token set --target git --server github.com --username octocat
```

Flags:

```
      --creds-store-mode string   when Docker keeps a registry in a credential store: login (docker login --password-stdin) or refuse
      --project string            write the project's auth.json or .npmrc instead of the user one
      --scope string              npm only: bind an @org scope to this registry
      --server string             the registry or git server, e.g. github.com or registry.npmjs.org
      --service string            a Composer service, e.g. github or gitlab (see token providers)
      --target string             the tool: npm, docker, git or netrc (Composer uses --service)
      --username string           the user name, for http-basic, docker, git and netrc
```

## authama token shell-aliases

The aliases auth-token-manager installed, pointed at this tool: atm runs
`authama token`, atm-ui the menu, atm-doctor `token audit`,
atm-init `token init`. They live in a marked block in your shell's rc file,
which --install replaces (an auth-token-manager block included) and --uninstall
removes. Without a flag it reports what the file holds.

```
authama token shell-aliases [flags]
```

Examples:

```sh
  authama token shell-aliases
  authama token shell-aliases --install
  eval "$(authama token shell-aliases --print)"
```

Flags:

```
      --file string   the startup file (default: your shell's rc file)
      --install       add or update the alias block
      --login         use the login profile (.zprofile, .bash_profile) instead of the rc file
      --no-backup     do not copy the file to <file>.authama.bak first
      --print         print the alias lines, for eval
      --quiet         print nothing on success
      --uninstall     remove the alias block
```

## authama token show

Prints one entry of `token list` in full. The secret stays masked unless
--reveal is given, which asks first at a terminal and needs --yes without one.

```
authama token show TARGET HOST [flags]
```

Examples:

```sh
  authama token show npm registry.npmjs.org
  authama token show git github.com --json
```

Flags:

```
      --json             print the entry as JSON (the same fields as token list --json)
      --project string   read the project's auth.json or .npmrc instead of the user one
      --reveal           show the secret value (asks first; --yes when not at a terminal)
  -y, --yes              reveal without asking
```

## authama token status

A read-only overview: the token config that would be used (and why), each tool's
credential file with its mode, how many tokens it holds, how many entries the
config has for it and how many of those resolve from the environment now. The
warnings name the entries `token apply` would skip, and why (usually an unset
variable). It always exits 0; `token audit` is the check that fails.

```
authama token status [flags]
```

Examples:

```sh
  authama token status
  authama token status --project . --json
```

Flags:

```
      --config string    the token config file (default: discovered)
      --json             machine-readable output (scripting)
      --project string   report the project's auth.json and .npmrc instead of the user ones
```

## authama tui

Interactive menu over the manager.

```
authama tui
```

Examples:

```sh
  authama tui
```

## authama version

Print the authama version.

```
authama version
```

Examples:

```sh
  authama version
```

## Choosing between similar commands

Some names appear in both groups, or sound alike. Where a word appears in both
`ssh` and `token`, it means the same thing in both (a test in names_test.go keeps it that way).

| You want to… | Use | Not |
|---|---|---|
| see everything that is installed | `ssh list` (keys), `token list` (tokens) | |
| look at one profile, host or key in full | `ssh show NAME` | |
| look at one token in full | `token show TARGET HOST` | |
| see one host as ssh will connect to it | `ssh view HOST` (resolved config, key, deployment) | `ssh show`, which lists files, blocks and pins |
| preview a change to `~/.ssh` | `ssh diff`, or `ssh reconcile --dry-run` | `ssh config check`, which only validates the config text |
| preview writing token files | `token apply --dry-run` | |
| a read-only overview that never fails | `token status`, `ssh list`, `ssh expiry` | |
| a check that fails CI when something is wrong | `doctor --strict` (both groups), `ssh config check`, `ssh validate`, `token audit` | |
| undo a key rotation | `ssh rollback` (the key's previous version) | `ssh restore` |
| put a whole machine back from a backup | `ssh restore` (an encrypted `ssh bundle`) | `ssh rollback` |
| undo the last write to `~/.ssh` | `ssh snapshots restore` | |
| get back into a host after losing the key | `ssh recover` (prints a break-glass snippet) | |
| replace or rotate a token | `token set` again for the same host | |
| move an old config in | `token import` (tokens), `ssh import` (an existing `~/.ssh`), `migrate` (an older layout of this tool) | |
