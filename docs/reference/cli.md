# Command reference

<!-- Generated from the command tree by TestCLIReferenceIsCurrent in
     src/internal/cli/clireference_test.go. Do not edit by hand:
     UPDATE_CLI_REFERENCE=1 go test ./internal/cli/ -run TestCLIReferenceIsCurrent -->

Every `authama` command, with its flags and examples, exactly as `--help` prints
them. The feature pages under [docs/tools](../tools/) explain what each one is for; this page is the
complete list. [Choosing between similar commands](#choosing-between-similar-commands)
is at the end.

Every command also takes the global flag:

```
      --dev-root string          run against a sandbox directory instead of ~/.ssh (also $AUTHAMA_DEV_ROOT)
      --no-dotenv[=true|false]   do not read <config home>/.env this run (also $AUTHAMA_NO_DOTENV=1)
```

## Contents

- [`authama api`](#authama-api) - Manage the read API's tokens
  - [`authama api token`](#authama-api-token) - Make, list and revoke API tokens
    - [`authama api token create`](#authama-api-token-create) - Make an API token, shown once
    - [`authama api token list`](#authama-api-token-list) - List API tokens (never the tokens themselves)
    - [`authama api token revoke`](#authama-api-token-revoke) - Revoke an API token: it stops working at once
- [`authama checklist`](#authama-checklist) - Where each key and token is used, and what is left to rotate or revoke it completely
  - [`authama checklist revoking`](#authama-checklist-revoking) - Start a token's revocation checklist (--stop to go back to where it is used)
  - [`authama checklist show`](#authama-checklist-show) - One item's checklist: every step, what is done, and the command for the rest
  - [`authama checklist tick`](#authama-checklist-tick) - Mark a step authama cannot check as done
  - [`authama checklist untick`](#authama-checklist-untick) - Mark a ticked step as not done
- [`authama companion`](#authama-companion) - Answer a companion tool such as sange: what this build can do and where it keeps things
  - [`authama companion info`](#authama-companion-info) - Print the name, version, contract versions, capabilities and homes; changes nothing
- [`authama doctor`](#authama-doctor) - Diagnose deps, perms, agent, known_hosts, drift, dangling keys, and token-file perms
- [`authama events`](#authama-events) - Every change seen to a file under ~/.ssh, the config home or a token file, newest first
- [`authama export`](#authama-export) - Write any list as CSV, JSON, YAML, a PHP or JS array, Markdown or HTML, or hosts for an SSH client, filtered
  - [`authama export schedule`](#authama-export-schedule) - Write saved selections to a folder every day
    - [`authama export schedule add`](#authama-export-schedule-add) - Schedule a saved selection to be written to a folder every day
    - [`authama export schedule install`](#authama-export-schedule-install) - Register a daily job that writes the scheduled exports
    - [`authama export schedule list`](#authama-export-schedule-list) - List the export schedules
    - [`authama export schedule remove`](#authama-export-schedule-remove) - Stop writing a saved selection to its folder
    - [`authama export schedule run`](#authama-export-schedule-run) - Write every scheduled export now
    - [`authama export schedule uninstall`](#authama-export-schedule-uninstall) - Remove the daily job that writes the scheduled exports
  - [`authama export selections`](#authama-export-selections) - List, show, save and delete export selections saved by name
    - [`authama export selections delete`](#authama-export-selections-delete) - Delete a saved selection
    - [`authama export selections list`](#authama-export-selections-list) - List the saved selections, both stores
    - [`authama export selections save`](#authama-export-selections-save) - Save a selection by name, from a JSON file
    - [`authama export selections show`](#authama-export-selections-show) - Print a saved selection as JSON
- [`authama history`](#authama-history) - List the changes authama has made, newest first
  - [`authama history reset`](#authama-history-reset) - Start the history again: move the history and audit log aside (kept)
  - [`authama history show`](#authama-history-show) - Show one operation: its events and what it changed in the manifest
- [`authama init`](#authama-init) - Create/converge the per-user home
- [`authama man`](#authama-man) - Write a manual page (troff, section 1) for every command into DIR
- [`authama mcp`](#authama-mcp) - Serve the Model Context Protocol over stdio, for an AI client
- [`authama migrate`](#authama-migrate) - Copy a pre-0.1.1 home (ssh-manager, sshmgr) into the authama home; the old one is kept and marked
- [`authama owner`](#authama-owner) - Bind this home to you and this machine: a PIN, and a seal that notices outside changes
  - [`authama owner fingerprint`](#authama-owner-fingerprint) - Let a fingerprint stand in for the PIN at the terminal (Touch ID, Windows Hello, fprintd)
  - [`authama owner forget`](#authama-owner-forget) - Unbind the home: remove the key, the seal and the PIN, keep everything else
  - [`authama owner init`](#authama-owner-init) - Bind this home to you and this machine, and seal its state files
  - [`authama owner pin`](#authama-owner-pin) - Set, change, reset or remove the PIN
    - [`authama owner pin remove`](#authama-owner-pin-remove) - Turn the PIN off (asks for it first)
    - [`authama owner pin reset`](#authama-owner-pin-reset) - Replace a forgotten PIN; proves the OS account instead of asking for the old one
    - [`authama owner pin set`](#authama-owner-pin-set) - Set a PIN, or change the one set (asks for it first)
  - [`authama owner reseal`](#authama-owner-reseal) - Accept the state files as they are now, and seal them
  - [`authama owner status`](#authama-owner-status) - Show whether the home is bound, where its key is kept, and whether a PIN is set
  - [`authama owner verify`](#authama-owner-verify) - Check the state files against the seal, and name any changed outside authama
- [`authama repair`](#authama-repair) - Find and fix what is wrong with the authama home, ~/.ssh and the token files
- [`authama reset`](#authama-reset) - Factory reset: move the authama home aside (kept) and start again; ~/.ssh is not touched
- [`authama serve`](#authama-serve) - Open the web UI on this machine
- [`authama settings`](#authama-settings) - Your settings files (.env and authama.json): where they are, what is in effect, validate, create
  - [`authama settings backup-key`](#authama-settings-backup-key) - Make an age key for encrypted backups and set backup.age_recipient to it
  - [`authama settings init`](#authama-settings-init) - Create a starter authama.json (schema reference and version; every setting at its default)
  - [`authama settings path`](#authama-settings-path) - Print where the config home, state home, .env and authama.json are
  - [`authama settings set`](#authama-settings-set) - Write one setting into authama.json, keeping the rest
  - [`authama settings show`](#authama-settings-show) - Print authama.json, or with --effective every setting's value and where it came from
  - [`authama settings unset`](#authama-settings-unset) - Take one setting out of authama.json, so it goes back to its default
  - [`authama settings validate`](#authama-settings-validate) - Check authama.json (or FILE) against the schema, and .env's syntax and permissions
- [`authama ssh`](#authama-ssh) - SSH keys, profiles, hosts, config and trust
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
    - [`authama ssh git resolve`](#authama-ssh-git-resolve) - Show the profile, mode, URL, SSH alias and key a GitHub repository would use; changes nothing
    - [`authama ssh git status`](#authama-ssh-git-status) - Show each checkout's remote mode, owner and profile
    - [`authama ssh git use`](#authama-ssh-git-use) - Point checkouts' origin at a mode, after proving the new URL with ls-remote
  - [`authama ssh host`](#authama-ssh-host) - Manage a host within a profile (add/edit/delete/apply)
    - [`authama ssh host add`](#authama-ssh-host-add) - Add a host to a profile (or name it by the naming rules with --env and --role)
    - [`authama ssh host apply`](#authama-ssh-host-apply) - Add or update hosts from compact specs (one, many, or a file)
    - [`authama ssh host delete`](#authama-ssh-host-delete) - Delete a host, its config block and its pins
    - [`authama ssh host edit`](#authama-ssh-host-edit) - Edit a host
  - [`authama ssh import`](#authama-ssh-import) - Onboard an existing ssh setup - a config file or any folder of them - into the manifest
  - [`authama ssh key`](#authama-ssh-key) - Manage a profile's keys (add/list/delete)
    - [`authama ssh key add`](#authama-ssh-key-add) - Declare a key on a profile and mint it
    - [`authama ssh key adopt`](#authama-ssh-key-adopt) - Manage a key where it already is in ~/.ssh, without moving or renaming it
    - [`authama ssh key delete`](#authama-ssh-key-delete) - Delete a key (refuses while a host uses it)
    - [`authama ssh key lifetime`](#authama-ssh-key-lifetime) - Set how long a key lives before rotation is due, and count its expiry again
    - [`authama ssh key list`](#authama-ssh-key-list) - Every key with its fingerprint, expiry, hosts and state
    - [`authama ssh key regen-pub`](#authama-ssh-key-regen-pub) - Rebuild public keys from their private key or 1Password item
    - [`authama ssh key reserve`](#authama-ssh-key-reserve) - Keep a loose ~/.ssh key on purpose (doctor stops calling it loose)
    - [`authama ssh key revoke`](#authama-ssh-key-revoke) - Remove a key from every target it is deployed to, and never deploy it again
    - [`authama ssh key root`](#authama-ssh-key-root) - Show or move the directory under ~/.ssh that holds the profile directories
    - [`authama ssh key unreserve`](#authama-ssh-key-unreserve) - Drop a key's reservation (doctor lists it as loose again)
  - [`authama ssh keygen`](#authama-ssh-keygen) - Generate a profile's or host's keys
  - [`authama ssh knownhosts`](#authama-ssh-knownhosts) - Pin host keys into the single ~/.ssh/known_hosts trust store
    - [`authama ssh knownhosts init`](#authama-ssh-knownhosts-init) - Pin reachable hosts into known_hosts (TOFU; fingerprints reported)
    - [`authama ssh knownhosts pin`](#authama-ssh-knownhosts-pin) - Seed known_hosts via ssh-keyscan, with confirmation
  - [`authama ssh list`](#authama-ssh-list) - Filterable tree across profiles
  - [`authama ssh load`](#authama-ssh-load) - Add a profile's keys to the agent
  - [`authama ssh naming`](#authama-ssh-naming) - Show, check and set how servers and their keys are named
    - [`authama ssh naming check`](#authama-ssh-naming-check) - List the hosts whose alias does not follow the naming rules
    - [`authama ssh naming key-template`](#authama-ssh-naming-key-template) - Set how new keys are named, keeping every existing key's name
    - [`authama ssh naming set`](#authama-ssh-naming-set) - Set the naming rules, for every profile or for one
    - [`authama ssh naming show`](#authama-ssh-naming-show) - Show the naming rules in effect, and each profile's code and own rules
  - [`authama ssh net`](#authama-ssh-net) - Per-host connection status + VPN indicator
  - [`authama ssh notify`](#authama-ssh-notify) - Manage the scheduled expiry notifier and desktop notifications
    - [`authama ssh notify doctor`](#authama-ssh-notify-doctor) - Say whether desktop notifications work here, and why not
    - [`authama ssh notify install`](#authama-ssh-notify-install) - Install the scheduled expiry notifier
    - [`authama ssh notify ping`](#authama-ssh-notify-ping) - Hand a notification to the running web UI (authama serve), which shows it
    - [`authama ssh notify send`](#authama-ssh-notify-send) - Show a desktop notification from this process
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
  - [`authama ssh termius`](#authama-ssh-termius) - Termius: the key to attach to each host
    - [`authama ssh termius keys`](#authama-ssh-termius-keys) - List the Termius key label for each host, or copy one host's private key to the clipboard
  - [`authama ssh validate`](#authama-ssh-validate) - Check keypairs parse, match, and have safe perms
  - [`authama ssh view`](#authama-ssh-view) - One host as ssh will use it: resolved config, key and deployment status (the connect view)
  - [`authama ssh vm`](#authama-ssh-vm) - Local VMs as SSH hosts: list, add, adopt a key, keep in sync, connect through the hypervisor
    - [`authama ssh vm add`](#authama-ssh-vm-add) - Add a VM as a host: its address comes from the hypervisor at connect time
    - [`authama ssh vm adopt-key`](#authama-ssh-vm-adopt-key) - Bring a loose ~/.ssh key under management as a devbox VM host's key
    - [`authama ssh vm docker-context`](#authama-ssh-vm-docker-context) - Create or update a Docker context ssh://<alias> for a VM host
    - [`authama ssh vm list`](#authama-ssh-vm-list) - List every hypervisor's VMs, and which manifest host uses each
    - [`authama ssh vm sync`](#authama-ssh-vm-sync) - Keep VM hosts in step with their hypervisors: pins, guest keys, Docker contexts, gone VMs
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
- [`authama version`](#authama-version) - Print the authama version
- [`authama webhook`](#authama-webhook) - Send events to your endpoints as they happen
  - [`authama webhook add`](#authama-webhook-add) - Add an endpoint; its signing secret is shown once
  - [`authama webhook deliver`](#authama-webhook-deliver) - Send every endpoint the events due to it now
  - [`authama webhook disable`](#authama-webhook-disable) - Stop sending to an endpoint; it keeps its place
  - [`authama webhook enable`](#authama-webhook-enable) - Send to an endpoint again, from where it stopped
  - [`authama webhook list`](#authama-webhook-list) - List endpoints and how their delivery stands (never their secrets)
  - [`authama webhook remove`](#authama-webhook-remove) - Remove an endpoint and its secret
  - [`authama webhook test`](#authama-webhook-test) - Send one webhook.test event now

## authama api

The read API is served by `authama serve` at /api/v1/ when api.enabled is on (it is off by default).
Programs call it with an API token: `Authorization: Bearer ama_…`. A token reads only, and is shown
once when it is made; authama keeps only its hash.

```
authama api
```

Subcommands:

- [`token`](#authama-api-token) - Make, list and revoke API tokens

## authama api token

Make, list and revoke API tokens.

```
authama api token
```

Subcommands:

- [`create`](#authama-api-token-create) - Make an API token, shown once
- [`list`](#authama-api-token-list) - List API tokens (never the tokens themselves)
- [`revoke`](#authama-api-token-revoke) - Revoke an API token: it stops working at once

## authama api token create

Make an API token, shown once.

```
authama api token create <name> [flags]
```

Examples:

```sh
  authama api token create ci   # prints the token once
  authama api token create dashboard --expires 0   # never expires
```

Flags:

```
      --expires int   days until it expires; 0 for never (default 90)
```

## authama api token list

List API tokens (never the tokens themselves).

```
authama api token list [flags]
```

Aliases: `ls`

Examples:

```sh
  authama api token list
  authama api token list --json
```

Flags:

```
      --json[=true|false]   a JSON array
```

## authama api token revoke

Revoke an API token: it stops working at once.

```
authama api token revoke <name>
```

Examples:

```sh
  authama api token revoke ci
```

## authama checklist

A checklist per key and per token. While a key is in use it lists where the key is:
its files, the hosts that use it, each target it is installed on. Once it is revoked, or a
rotation replaces it, it lists what still has to go: targets it could not be removed from,
the private key, ssh-agent, encrypted bundles, and copies authama cannot see.

A token's revocation checklist starts with `authama checklist revoking <item>`.

Steps authama can check it works out each time. The rest (the token's issuer, Termius, a
CI secret) you tick by hand with `authama checklist tick`; ticks are kept in the state
directory. An item is named by its id from this list, or a key by profile/key.

```
authama checklist [flags]
```

Subcommands:

- [`revoking`](#authama-checklist-revoking) - Start a token's revocation checklist (--stop to go back to where it is used)
- [`show`](#authama-checklist-show) - One item's checklist: every step, what is done, and the command for the rest
- [`tick`](#authama-checklist-tick) - Mark a step authama cannot check as done
- [`untick`](#authama-checklist-untick) - Mark a ticked step as not done

Examples:

```sh
  authama checklist
  authama checklist --open
  authama checklist --json
```

Flags:

```
      --json[=true|false]   print every checklist as JSON
      --open[=true|false]   only items with steps left
```

## authama checklist revoking

A token has no revoked state of its own: authama cannot tell whether its issuer still
accepts it. This switches its checklist to the steps for revoking it - at the issuer, in each
tool's file, in authama's .env, in CI - and --stop switches it back.

A key's checklist follows the key; revoke it with `authama ssh key revoke` or rotate it.

```
authama checklist revoking <token item> [flags]
```

Examples:

```sh
  authama checklist revoking token:npm/registry.npmjs.org
  authama checklist revoking token:npm/registry.npmjs.org --stop
```

Flags:

```
      --stop[=true|false]   stop the revocation checklist
```

## authama checklist show

One item's checklist: every step, what is done, and the command for the rest.

```
authama checklist show <item> [flags]
```

Examples:

```sh
  authama checklist show work/deploy
  authama checklist show token:npm/registry.npmjs.org --json
```

Flags:

```
      --json[=true|false]   print the checklist as JSON
```

## authama checklist tick

Mark a step authama cannot check as done. The step is the id `authama checklist show` prints after "by hand:".
Only steps authama cannot see for itself can be ticked.

```
authama checklist tick <item> <step>
```

Examples:

```sh
  authama checklist tick work/deploy copies
  authama checklist tick token:npm/registry.npmjs.org issuer
```

## authama checklist untick

Mark a ticked step as not done. The step is the id `authama checklist show` prints after "by hand:".
Only steps authama cannot see for itself can be ticked.

```
authama checklist untick <item> <step>
```

Examples:

```sh
  authama checklist untick work/deploy copies
```

## authama companion

Answer a companion tool such as sange: what this build can do and where it keeps things.

```
authama companion
```

Subcommands:

- [`info`](#authama-companion-info) - Print the name, version, contract versions, capabilities and homes; changes nothing

## authama companion info

The handshake of sange's companion contract. With --json, prints {"name", "version",
"contractVersions", "capabilities", "configHome", "stateHome"}. A capability is listed when
this build has it, whether or not a setting turns it on. Fields are only ever added.

```
authama companion info [flags]
```

Examples:

```sh
  authama companion info
  authama companion info --json
```

Flags:

```
      --json[=true|false]   machine-readable output (scripting); fields are only ever added
```

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
  -n, --dry-run[=true|false]   with --fix: show what it would do, change nothing
      --fix[=true|false]       fix first: perms, stale authama pins, and a VM sync (backs up before removing)
      --json[=true|false]      machine-readable output (scripting)
      --strict[=true|false]    for CI: treat every dangling-key state as a failure, not only the blocking ones, and fail on a token file others can read
  -y, --yes[=true|false]       with --fix: do the upkeep without confirming (implied when there is no terminal)
```

## authama events

authama tracks the files it is responsible for - everything under ~/.ssh, the config
home, and each tool's credential file - and records every change to them, whoever
made it: authama, an editor, a package manager. Each run scans first, then lists.
An event names the file and what changed (added, changed, removed, mode), never
its contents. Each line is chained to the one before it by a SHA-256 hash, so
--verify catches a line edited, removed or inserted after the fact, or a log
cut short. The web UI scans every 30 seconds while it runs.

```
authama events [flags]
```

Examples:

```sh
  authama events
  authama events --since 24h --area auth
  authama events --kind mode --json
  authama events --verify
```

Flags:

```
      --area string            only this area: ssh, config or auth
      --csv[=true|false]       one CSV row per event, header first
      --json[=true|false]      machine-readable output (scripting)
      --kind string            only this kind: added, changed, removed, mode or started
      --limit int              at most this many (newest first); 0 is all
      --no-scan[=true|false]   list what is recorded without scanning first
      --since duration         only events newer than this, e.g. 24h
      --verify[=true|false]    check the log's hash chain for edited, removed or inserted lines; exit non-zero if broken
```

## authama export

Write one of the home's lists, filtered and with the columns chosen, in any format: the
same files the web UI's Export page makes. Nothing in an export holds a secret.

The list is one of deployments, events, history, hosts, keys, profiles, snapshots, tokens, or a saved selection's name (`export selections`
keeps them), or --selection names a selection as a JSON file (the Export page's Copy link holds one). Filters narrow it: --profile, --key (a key's name or
profile/name), --env, --role, --tag, --status, --tool, --kind, --area, --source, --command,
--since and --until (a time or a prefix such as 2026-11), --include and --exclude text.
--columns chooses the columns, by field (--fields lists them; key.fingerprint follows the
host's key), each optionally renamed as field=heading. --sort orders the rows (-field
descends). --format is one of csv, html, js, json, jsonl, markdown, php, tsv, yaml.

Transforms run after the filters and before the columns, in this order: each --transform
(a step as JSON, the Export page's shape), each --template (as=text naming fields in
braces), --dedupe (keep the first row of each), then --group-by with --aggregate
(count, or op:field for sum, min, max, list or first) and --totals, or --pivot rows:column
(a row per value of rows, a column per value of column, cells by the first --aggregate,
count by default) with --totals. A column can name
the fields they write. The SSH client formats below take none.

Hosts also go out as an SSH client imports them: --format hosts-csv, iterm2, mobaxterm, putty, remmina, securecrt, ssh_config, termius, windows-terminal.
ssh_config is the managed config of the hosts kept, byte for byte what `ssh config export --flat`
writes when every host is kept; termius is the Termius CSV; hosts-csv is a generic CSV; the rest
are the clients' own files. Those formats write hosts in manifest order unless --sort is
given, ignore --columns, leave out git hosts unless --include-git and, except ssh_config,
windows-terminal and iterm2, hosts behind a ProxyCommand. remmina writes a zip of one file per
host. What was left out, and what is left to do in the client, goes to stderr.

The file goes to stdout, or to --out.

```
authama export [deployments|events|history|hosts|keys|profiles|snapshots|tokens|<saved selection>] [flags]
```

Subcommands:

- [`schedule`](#authama-export-schedule) - Write saved selections to a folder every day
- [`selections`](#authama-export-selections) - List, show, save and delete export selections saved by name

Examples:

```sh
  authama export hosts --profile simtabi --env prd --format json
  authama export keys --columns ref=key,fingerprint,expires_on --sort expires_on --format markdown
  authama export profiles --key simtabi_simbi-prd-web-00-ed25519 --narrow --format yaml
  authama export hosts --fields
  authama export hosts --template 'ssh=ssh {user}@{hostname} -p {port}' --columns alias,ssh
  authama export hosts --group-by profile --aggregate count,list:env --totals --format markdown
  authama export hosts --pivot profile:env --totals
  authama export --selection sel.json --format php --out hosts.php
  authama export prd-hosts --format json
  authama export hosts --profile simtabi --format ssh_config --out simtabi.ssh_config
  authama export hosts --env prd --format putty --out prd.reg
```

Flags:

```
      --aggregate strings          with --group-by or --pivot: count, or op:field[=as] for sum, min, max, list, first
      --area strings               events: only these areas (ssh, config, auth)
      --columns strings            the columns, by field, in order; field=heading renames one
      --command strings            history: only these commands, or their subcommands
      --compact[=true|false]       json, js, php: no indentation
      --dedupe strings             keep the first row of each distinct value of these fields
      --env strings                only these environments
      --exclude string             leave out rows containing this text, ignoring case
      --fields[=true|false]        list every field the list's rows have, after any transforms, and write nothing
      --format string              csv, tsv, json, jsonl, yaml, php, js, markdown or html; for hosts also hosts-csv, iterm2, mobaxterm, putty, remmina, securecrt, ssh_config, termius, windows-terminal (default "csv")
      --group-by strings           a row per distinct value of these fields
      --include string             only rows containing this text, ignoring case
      --include-git[=true|false]   host formats: also export git hosts (user git, or a github or gitlab provider)
      --key strings                only rows holding or using these keys (name or profile/name)
      --kind strings               events and tokens: only these kinds
      --limit int                  at most this many rows (0: all)
      --narrow[=true|false]        profiles: count and list only the keys --key chose
      --no-header[=true|false]     csv, tsv, markdown, html: leave the heading row out
  -o, --out string                 write to this file (mode 0600) instead of stdout
      --pivot string               rows:column - a row per value of the rows fields (comma-separated), a column per value of column
      --profile strings            only these profiles
      --role strings               only these roles
      --selection string           a selection as a JSON file (the Export page's), or a saved selection's name, instead of a list name
      --shape string               json, js, php, yaml: "table" writes {columns, rows} instead of one object per row
      --since string               only rows at or after this time (a prefix such as 2026-11 works)
      --sort strings               sort by these fields; -field descends
      --source strings             history: only these sources (cli, web, ...)
      --status strings             only these statuses (a host's status, a key's state)
      --tag strings                only rows with one of these tags
      --template stringArray       a column from a template: as={profile}/{alias}
      --tool strings               tokens: only these tools
      --totals[=true|false]        with --group-by: a last row over every group; with --pivot, also a total column
      --transform stringArray      a transform step as JSON, e.g. {"kind":"map","field":"status","values":{"needs-redeploy":"pending"}}
      --until string               only rows at or before this time
```

## authama export schedule

A schedule writes a saved selection to a folder, in one format, as <name>.<extension>,
replacing the file each time. `schedule install` registers one daily job with this
system's scheduler (launchd, systemd or cron, schtasks) that runs `schedule run`, which
writes every schedule; `schedule run` can be run by hand too. Schedules are yours
alone, kept under the state home: a folder is a path on this machine.

```
authama export schedule
```

Subcommands:

- [`add`](#authama-export-schedule-add) - Schedule a saved selection to be written to a folder every day
- [`install`](#authama-export-schedule-install) - Register a daily job that writes the scheduled exports
- [`list`](#authama-export-schedule-list) - List the export schedules
- [`remove`](#authama-export-schedule-remove) - Stop writing a saved selection to its folder
- [`run`](#authama-export-schedule-run) - Write every scheduled export now
- [`uninstall`](#authama-export-schedule-uninstall) - Remove the daily job that writes the scheduled exports

## authama export schedule add

Schedule the saved selection <selection> to be written to --to every day, in --format
(csv by default; the SSH client formats are not scheduled). Adding the same selection
and format again replaces the folder.

```
authama export schedule add <selection> [flags]
```

Examples:

```sh
  authama export schedule add prd-hosts --to ~/Reports
  authama export schedule add prd-hosts --format json --to ~/Reports
```

Flags:

```
      --format string   one of csv, html, js, json, jsonl, markdown, php, tsv, yaml (default "csv")
      --to string       the folder to write to (made, mode 0700, if it is not there)
```

## authama export schedule install

Register a daily job that writes the scheduled exports.

```
authama export schedule install
```

Examples:

```sh
  authama export schedule install
```

## authama export schedule list

List the export schedules.

```
authama export schedule list
```

Aliases: `ls`

Examples:

```sh
  authama export schedule list
```

## authama export schedule remove

Remove the schedules of <selection>: every format, or only --format. The files already
written stay where they are.

```
authama export schedule remove <selection> [flags]
```

Examples:

```sh
  authama export schedule remove prd-hosts
  authama export schedule remove prd-hosts --format json
```

Flags:

```
      --format string   only the schedule in this format
```

## authama export schedule run

Write every schedule's file now, as the daily job does. A schedule that fails is
reported and the rest still run; the command fails if any did.

```
authama export schedule run [flags]
```

Examples:

```sh
  authama export schedule run
```

Flags:

```
      --quiet[=true|false]   print nothing unless one fails
```

## authama export schedule uninstall

Remove the daily job that writes the scheduled exports.

```
authama export schedule uninstall
```

Examples:

```sh
  authama export schedule uninstall
```

## authama export selections

A selection saved by name is exported with `authama export <name>`. It lives in one of two
stores: user (a file of its own under the state home, private to this account; the default)
or manifest (defaults.export.selections, so it travels with the manifest). The setting
export.save_to changes where a save goes. Both stores are read; on a name in both, the
user's wins.

```
authama export selections
```

Subcommands:

- [`delete`](#authama-export-selections-delete) - Delete a saved selection
- [`list`](#authama-export-selections-list) - List the saved selections, both stores
- [`save`](#authama-export-selections-save) - Save a selection by name, from a JSON file
- [`show`](#authama-export-selections-show) - Print a saved selection as JSON

## authama export selections delete

Delete the saved selection <name> from the store that has it. When both stores have it,
--from says which; the other one stays.

```
authama export selections delete <name> [flags]
```

Aliases: `rm`

Examples:

```sh
  authama export selections delete prd-hosts
  authama export selections delete prd-hosts --from manifest
```

Flags:

```
      --from string        the store to delete it from, when both have it: user or manifest
  -y, --yes[=true|false]   delete from the manifest without confirming (implied when there is no terminal)
```

## authama export selections list

List the saved selections, both stores.

```
authama export selections list [flags]
```

Aliases: `ls`

Examples:

```sh
  authama export selections list
  authama export selections list --json
```

Flags:

```
      --json[=true|false]   print the selections as JSON
```

## authama export selections save

Save the selection in --from (a JSON file, such as the Export page's Copy link holds) as
<name>. --to chooses the store: user (yours alone) or manifest (travels with it); without
it, the export.save_to setting does, and that is user unless set. Saving under a name the
store already has replaces it. Export it with `authama export <name>`.

```
authama export selections save <name> [flags]
```

Examples:

```sh
  authama export selections save prd-hosts --from sel.json
  authama export selections save prd-hosts --from sel.json --to manifest
```

Flags:

```
      --from string        the selection, as a JSON file
      --to string          the store: user or manifest (default: the export.save_to setting, else user)
  -y, --yes[=true|false]   save to the manifest without confirming (implied when there is no terminal)
```

## authama export selections show

Print a saved selection as JSON.

```
authama export selections show <name>
```

Examples:

```sh
  authama export selections show prd-hosts
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

- [`reset`](#authama-history-reset) - Start the history again: move the history and audit log aside (kept)
- [`show`](#authama-history-show) - Show one operation: its events and what it changed in the manifest

Examples:

```sh
  authama history
  authama history --since 7d --command "host add"
  authama history --host web1 --json
```

Flags:

```
      --command string      only this command ("host add"), or a group of them ("host")
      --csv[=true|false]    one CSV row per operation, header first
      --host string         only operations about this host alias
      --json[=true|false]   one JSON object per operation, its events nested
      --key string          only operations about this key
      --limit int           at most this many operations (0 for all) (default 50)
      --profile string      only operations about this profile
      --since string        only operations started at or after this
      --source string       only operations from cli, tui, web or backfill
      --token string        only operations about this token: TOOL or TOOL/HOST, e.g. npm/registry.npmjs.org
      --until string        only operations started at or before this
```

## authama history reset

Renames the history database and the audit log to <name>.reset-<time>, so renaming
them back undoes it. Both go, because a new history imports the audit log the first
time it is opened. The next change starts a new history, which first lists what is on
disk now. Refused while history.permanent (AUTHAMA_HISTORY_PERMANENT) is on.

It asks you to type "history"; a script passes --confirm history.

```
authama history reset [flags]
```

Examples:

```sh
  authama history reset
  authama history reset --confirm history   # a script
```

Flags:

```
      --confirm string   type "history" to start the history again without being asked
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
  -f, --force[=true|false]       overwrite manifest/inventory/.env with fresh defaults, keeping a copy of each in the state home's .state/ first
      --no-backup[=true|false]   with --force, keep no copy of the files it overwrites
  -y, --yes[=true|false]         skip the confirmation prompt
```

## authama man

Writes one manual page per command, authama.1, authama-ssh.1, authama-ssh-deploy.1
and so on, with the sections the help prints: NAME, SYNOPSIS, DESCRIPTION,
COMMANDS, OPTIONS, EXAMPLES and SEE ALSO. DIR defaults to ./man; put it on
MANPATH, or copy the pages into a man1 directory, to read them with man(1).

```
authama man [DIR]
```

Examples:

```sh
  authama man
  authama man ~/.local/share/man/man1
```

## authama mcp

An AI client (Claude Code, Claude Desktop, an IDE) starts `authama mcp` and talks to it over stdio.
Its tools read only: profiles, keys, hosts, which auth tokens are set (never a secret), the history,
the event feed and doctor. It opens no port, so only the client that started it can reach it.
It is off by default: `authama settings set mcp.enabled true` first.

```
authama mcp
```

Examples:

```sh
  authama mcp   # started by an AI client, which talks to it over stdio
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
  -f, --force[=true|false]   if both the legacy and standard home exist, back up the current home and replace it
  -y, --yes[=true|false]     migrate without confirming (implied when there is no terminal)
```

## authama owner

Binds the authama home to the OS account and machine it runs on.

`owner init` keeps a sealing key in the OS's credential store (the macOS
Keychain, the Secret Service on Linux, DPAPI on Windows, else an owner-only
file) and seals the state files with it. authama reseals after each of its own
operations, so a file changed by anything else, or a home copied to another
account or machine, is named the next time authama runs.

`owner pin set` adds a PIN or passphrase. The web UI asks for it at sign-in and
before every change; the command line asks before revealing or copying a secret
and before a destructive command. A forgotten PIN is reset with `owner pin
reset`, which needs no old PIN, only this account on this machine.

```
authama owner
```

Subcommands:

- [`fingerprint`](#authama-owner-fingerprint) - Let a fingerprint stand in for the PIN at the terminal (Touch ID, Windows Hello, fprintd)
- [`forget`](#authama-owner-forget) - Unbind the home: remove the key, the seal and the PIN, keep everything else
- [`init`](#authama-owner-init) - Bind this home to you and this machine, and seal its state files
- [`pin`](#authama-owner-pin) - Set, change, reset or remove the PIN
- [`reseal`](#authama-owner-reseal) - Accept the state files as they are now, and seal them
- [`status`](#authama-owner-status) - Show whether the home is bound, where its key is kept, and whether a PIN is set
- [`verify`](#authama-owner-verify) - Check the state files against the seal, and name any changed outside authama

## authama owner fingerprint

The command line asks for a fingerprint first and falls back to the PIN: Touch ID
on a Mac (through the authama-touchid helper, built beside authama), Windows Hello
on Windows (through Windows PowerShell), fprintd on Linux. Not over SSH, where the
prompt would appear on a screen nobody at this terminal sees.

```
authama owner fingerprint <on|off>
```

Examples:

```sh
  authama owner fingerprint on
  authama owner fingerprint off
```

## authama owner forget

Unbind the home: remove the key, the seal and the PIN, keep everything else.

```
authama owner forget [flags]
```

Examples:

```sh
  authama owner forget
```

Flags:

```
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama owner init

Bind this home to you and this machine, and seal its state files.

```
authama owner init [flags]
```

Examples:

```sh
  authama owner init
  authama owner init --yes
```

Flags:

```
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama owner pin

Set, change, reset or remove the PIN.

```
authama owner pin
```

Subcommands:

- [`remove`](#authama-owner-pin-remove) - Turn the PIN off (asks for it first)
- [`reset`](#authama-owner-pin-reset) - Replace a forgotten PIN; proves the OS account instead of asking for the old one
- [`set`](#authama-owner-pin-set) - Set a PIN, or change the one set (asks for it first)

## authama owner pin remove

Turn the PIN off (asks for it first).

```
authama owner pin remove
```

Examples:

```sh
  authama owner pin remove
```

## authama owner pin reset

Needs no old PIN. It must read the sealing key from this account's own
credential store (on macOS the Keychain may ask for the login password), on the
machine the home was bound on, and it must run in a terminal. The reset is on
the record, and wrong-PIN lockouts are cleared.

```
authama owner pin reset
```

Examples:

```sh
  authama owner pin reset   # forgot it: proves the OS account instead
```

## authama owner pin set

A PIN is at least 6 characters: digits, or a passphrase of up to 128. A run such
as 123456 or one repeated character is refused. It is stored only as a salted
Argon2id hash.

```
authama owner pin set
```

Examples:

```sh
  authama owner pin set
```

## authama owner reseal

Use it after a change you made yourself outside authama - an edit to
tokens.json, a restored backup - once `owner verify` has shown what changed.

```
authama owner reseal [flags]
```

Examples:

```sh
  authama owner reseal   # after editing tokens.json by hand
```

Flags:

```
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama owner status

Show whether the home is bound, where its key is kept, and whether a PIN is set.

```
authama owner status
```

Examples:

```sh
  authama owner status
```

## authama owner verify

Exits 0 when the seal matches (or the home is not bound), and non-zero when a
file was changed, removed or added since authama last sealed it, when the seal
was made on another account or machine, or when it cannot be checked.

```
authama owner verify
```

Examples:

```sh
  authama owner verify   # exits non-zero when a file changed outside authama
```

## authama repair

Checks three areas and fixes what can be fixed safely:

  tool    missing config and state directories; a history database that fails
          its integrity check (renamed aside, never deleted); a manifest or
          inventory that does not load, and the owner seal (reported only)
  ssh     files a crashed command left; modes ssh or authama refuse; keys with
          no .pub; a ~/.ssh/config that no longer matches the manifest
  tokens  credential files others can read; a tokens.json that does not parse
          (reported only)

It lists what it found, asks, then fixes. --check lists only, and exits non-zero
when anything is wrong. Nothing is deleted.

```
authama repair [flags]
```

Examples:

```sh
  authama repair
  authama repair --check   # CI: exit non-zero when anything is wrong
  authama repair --area ssh --yes
```

Flags:

```
      --area strings         only these areas: tool, ssh, tokens (repeatable)
      --check[=true|false]   list what is wrong and change nothing; exit non-zero if anything is
  -y, --yes[=true|false]     skip the confirmation prompt
```

## authama reset

Renames the config home, and the state home when it is separate, to
<name>.reset-<time>, so renaming them back undoes it. Nothing is deleted, and
~/.ssh - keys, config, known_hosts - is not touched. A home bound to its owner
is unbound first (its key in the OS store is removed), so a restored home is
never bound to a key that no longer exists. With a PIN set, it asks for it.

It asks you to type "reset"; a script passes --confirm reset.

```
authama reset [flags]
```

Examples:

```sh
  authama reset
  authama reset --confirm reset   # a script
```

Flags:

```
      --confirm string   type "reset" here to skip the question
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

One server per home: when one is already running for this home, serve opens
it again instead of starting another, so launching twice - or double-clicking
the app - never piles servers up. --new starts another anyway; --status says
whether one is running and where; --stop stops it. --stop-all stops every
authama web UI answering on this machine's usual ports, whichever home or build;
--restart does that and starts a fresh one, which is how to load a new build.
A page from an older build says so, and can restart its server itself. When every port from the
default up is taken, any free port is used.

web.addr, web.port and web.base_path in authama.json (or AUTHAMA_WEB_ADDR,
AUTHAMA_WEB_PORT, AUTHAMA_WEB_BASE_PATH) change the defaults: a configured port
falls back to the next free one like the default, and web.base_path serves the
UI under a path such as /authama, for a reverse proxy. --addr and --port win.

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
      --addr string                 address to listen on, host or host:port (a port here pins it, like --port) (default "127.0.0.1")
      --allow-host stringArray      extra Host header to accept, e.g. the name a proxy on this machine forwards as; it widens the DNS-rebinding check, so name only hosts you control (repeatable)
      --allow-remote[=true|false]   allow a non-loopback --addr (plain HTTP; prefer an SSH tunnel)
      --new[=true|false]            start another server even when one is already running for this home
      --no-open[=true|false]        do not open a browser
      --port int                    port to listen on; when not given and the default is taken, the next free port is used (default 7822)
      --read-only[=true|false]      serve no route that changes anything, and show no form
      --restart[=true|false]        stop every authama web UI found, then start a fresh one: how to load a new build
      --status[=true|false]         say whether a server is running for this home, and its address
      --stop[=true|false]           stop the server running for this home
      --stop-all[=true|false]       stop every authama web UI answering on this machine's usual ports, whichever home or build
      --token-file string           also write the sign-in address to this file, mode 0600, for scripts
```

## authama settings

authama reads two files from its config home at start-up:

  .env           KEY=value lines: tokens, and any AUTHAMA_* setting
  authama.json   settings, checked against a schema (https://raw.githubusercontent.com/sangehq/authama/main/docs/schema/authama.schema.json)

Precedence: command-line flags > environment > .env > authama.json > defaults.
A variable already in the environment is never replaced by a file; --no-dotenv
or $AUTHAMA_NO_DOTENV=1 skips .env for a run.

```
authama settings
```

Subcommands:

- [`backup-key`](#authama-settings-backup-key) - Make an age key for encrypted backups and set backup.age_recipient to it
- [`init`](#authama-settings-init) - Create a starter authama.json (schema reference and version; every setting at its default)
- [`path`](#authama-settings-path) - Print where the config home, state home, .env and authama.json are
- [`set`](#authama-settings-set) - Write one setting into authama.json, keeping the rest
- [`show`](#authama-settings-show) - Print authama.json, or with --effective every setting's value and where it came from
- [`unset`](#authama-settings-unset) - Take one setting out of authama.json, so it goes back to its default
- [`validate`](#authama-settings-validate) - Check authama.json (or FILE) against the schema, and .env's syntax and permissions

## authama settings backup-key

Makes a new age key with age-keygen, keeps its identity owner-only where
paths.age_identity_file already points, else in the config home
(backup/age-identity.txt), and sets backup.age_recipient and
paths.age_identity_file to it in authama.json. Keep a copy of the identity file
somewhere safe: without it no backup can be decrypted.

```
authama settings backup-key
```

Examples:

```sh
  authama settings backup-key
```

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
  -f, --force[=true|false]   replace an existing authama.json
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
      --json[=true|false]   machine-readable output (scripting)
```

## authama settings set

Writes one setting, by its dotted key (backup.age_recipient, web.port), into
authama.json, creating the file when there is none. The value is checked against the
setting first. `authama settings show --effective` lists every key.

```
authama settings set <key> <value>
```

Examples:

```sh
  authama settings set backup.age_recipient age1...   # your age recipient
  authama settings set web.port 8000
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
      --effective[=true|false]   every setting with its value in effect and its source
      --json[=true|false]        machine-readable output (with --effective)
```

## authama settings unset

Take one setting out of authama.json, so it goes back to its default.

```
authama settings unset <key>
```

Examples:

```sh
  authama settings unset web.port
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

SSH keys, profiles, hosts, config and trust: everything authama does in ~/.ssh.

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
- [`import`](#authama-ssh-import) - Onboard an existing ssh setup - a config file or any folder of them - into the manifest
- [`key`](#authama-ssh-key) - Manage a profile's keys (add/list/delete)
- [`keygen`](#authama-ssh-keygen) - Generate a profile's or host's keys
- [`knownhosts`](#authama-ssh-knownhosts) - Pin host keys into the single ~/.ssh/known_hosts trust store
- [`list`](#authama-ssh-list) - Filterable tree across profiles
- [`load`](#authama-ssh-load) - Add a profile's keys to the agent
- [`naming`](#authama-ssh-naming) - Show, check and set how servers and their keys are named
- [`net`](#authama-ssh-net) - Per-host connection status + VPN indicator
- [`notify`](#authama-ssh-notify) - Manage the scheduled expiry notifier and desktop notifications
- [`profile`](#authama-ssh-profile) - Manage a profile (add/edit/delete)
- [`providers`](#authama-ssh-providers) - List the active provider catalog + credential state
- [`reconcile`](#authama-ssh-reconcile) - Build ~/.ssh from the manifest
- [`recover`](#authama-ssh-recover) - Print a break-glass snippet for regaining access when a key is lost
- [`restore`](#authama-ssh-restore) - Restore keys + state from an encrypted bundle (see bundle)
- [`rollback`](#authama-ssh-rollback) - Undo a rotation: put a key's previous version back
- [`rotate`](#authama-ssh-rotate) - Zero-downtime staged key rotation
- [`show`](#authama-ssh-show) - Everything about a profile, host or key: files, config blocks, pins (the inventory view)
- [`snapshots`](#authama-ssh-snapshots) - List/restore/prune local ~/.ssh backups
- [`termius`](#authama-ssh-termius) - Termius: the key to attach to each host
- [`validate`](#authama-ssh-validate) - Check keypairs parse, match, and have safe perms
- [`view`](#authama-ssh-view) - One host as ssh will use it: resolved config, key and deployment status (the connect view)
- [`vm`](#authama-ssh-vm) - Local VMs as SSH hosts: list, add, adopt a key, keep in sync, connect through the hypervisor

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
      --csv[=true|false]      print the deployments as CSV, one row per key and target, header first
      --notify[=true|false]   also fire the cadence-gated desktop alert
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
      --adopt[=true|false]     put untagged pins matching a manifest host under authama's management
      --dry-run[=true|false]   report what would change, touch nothing
  -y, --yes[=true|false]       clean without confirming (implied when there is no terminal)
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
      --flat[=true|false]   every host inline in one file, absolute paths, no Include
  -o, --out string          where to write (default <config home>/exports/ssh_config; - for stdout)
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
  -n, --dry-run[=true|false]   preview changes without writing
  -y, --yes[=true|false]       render without confirming (implied when there is no terminal)
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
  -y, --yes[=true|false]   deploy without confirming (implied when there is no terminal)
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

- [`resolve`](#authama-ssh-git-resolve) - Show the profile, mode, URL, SSH alias and key a GitHub repository would use; changes nothing
- [`status`](#authama-ssh-git-status) - Show each checkout's remote mode, owner and profile
- [`use`](#authama-ssh-git-use) - Point checkouts' origin at a mode, after proving the new URL with ls-remote

## authama ssh git resolve

Show how a GitHub repository would be reached, without a checkout and without changing
anything. The profile is --profile, else the owner's (defaults.git.orgs, then defaults.git.profile);
the mode is --mode, else $AUTHAMA_GIT_MODE, else defaults.git.mode, else profile.

With --json, prints {"version": 1, "profile", "mode", "url", "hostAlias",
"keyFingerprint"}, the names sange's companion contract uses. hostAlias and keyFingerprint are
empty in https mode, and keyFingerprint is empty for a key not made yet. Fields are only ever added.

```
authama ssh git resolve <owner>/<repo> [flags]
```

Examples:

```sh
  authama ssh git resolve sangehq/authama
  authama ssh git resolve sangehq/authama --json
```

Flags:

```
      --json[=true|false]   machine-readable output (scripting); fields are only ever added
      --mode string         profile, ssh or https (overrides $AUTHAMA_GIT_MODE and the manifest)
      --profile string      use this profile instead of the owner's mapping
```

## authama ssh git status

Show how each checkout's origin reaches GitHub. With --json, prints
{"version": 1, "default_mode": ..., "checkouts": [...]}: each checkout's path,
state (managed, no-origin or unmanaged), url, mode, owner, repo, profile and
profile_from (remote or owner). Fields are only ever added.

```
authama ssh git status [path...] [flags]
```

Examples:

```sh
  authama ssh git status
  authama ssh git status ~/code/acme ~/code/personal
```

Flags:

```
      --json[=true|false]   machine-readable output (scripting); fields are only ever added
```

## authama ssh git use

Point each checkout's origin at a mode. The mode is the first argument when it
names one, else --mode, else $AUTHAMA_GIT_MODE, else defaults.git.mode, else profile.
A new URL is checked with git ls-remote before origin is rewritten, so a checkout
never ends up with a remote it cannot authenticate to. Non-GitHub origins are skipped.

With --json, prints {"version": 1, "mode": ..., "dry_run": ..., "checkouts": [...]}:
each checkout's path, from, to, applied, skipped, error, owner, repo, profile and
profile_from (flag or owner). Fields are only ever added.

```
authama ssh git use [profile|ssh|https] [path...] [flags]
```

Examples:

```sh
  authama ssh git use profile --dry-run
  authama ssh git use https ~/code/acme/api --yes
  authama ssh git use ssh --profile work
  authama ssh git use profile ~/code/acme --dry-run --json
```

Flags:

```
      --dry-run[=true|false]   show what would change, touch nothing
      --json[=true|false]      machine-readable output (scripting); fields are only ever added
      --mode string            profile, ssh or https (overrides $AUTHAMA_GIT_MODE and the manifest)
      --profile string         use this profile's key instead of the owner's mapping
  -y, --yes[=true|false]       rewrite without confirming (implied when there is no terminal)
```

## authama ssh host

Manage a host within a profile (add/edit/delete/apply).

```
authama ssh host
```

Subcommands:

- [`add`](#authama-ssh-host-add) - Add a host to a profile (or name it by the naming rules with --env and --role)
- [`apply`](#authama-ssh-host-apply) - Add or update hosts from compact specs (one, many, or a file)
- [`delete`](#authama-ssh-host-delete) - Delete a host, its config block and its pins
- [`edit`](#authama-ssh-host-edit) - Edit a host

## authama ssh host add

Add a host to a profile.

Without <alias>, --env and --role name it by the naming rules
(defaults.naming.alias, {code}-{env}-{role}-{nn} unless set): the profile's code,
the environment, the role and the next free number, so a third web server of
simtabi in production is simbi-prd-web-02. The role and environment are added as
tags. With defaults.naming.enforce at error, an alias that breaks the rules is
refused; at warn it is added and the reason printed.

```
authama ssh host add <profile> [<alias>] [flags]
```

Examples:

```sh
  authama ssh host add work web1 --hostname web1.example.com --user deploy
  authama ssh host add work gh --hostname github.com --user git --provider github --token-env GH_TOKEN_WORK
```

Flags:

```
      --env string         with no <alias>: the environment the generated alias names (prd, stg, dev)
  -H, --hostname string    host to connect to
      --key-name string    use this key instead of the derived <profile>_<alias>-<algo> name
  -p, --port int           ssh port (default 22)
      --provider string    deployment adapter for this host (see 'authama ssh providers')
      --proxy string       reach the host through a proxy: cloudflared (Cloudflare Access; ssh runs 'cloudflared access ssh --hostname %h')
      --role string        with no <alias>: the role the generated alias names (web, db, ...)
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
      --category string               category for a profile this creates (implies --create-profile)
      --create-profile[=true|false]   create the profile if it does not exist
      --dry-run[=true|false]          report what would change without writing
  -f, --file string                   read specs from a file, one per line or a JSON array (- for stdin)
      --host stringArray              a spec "<alias> <host>[:port] <user> [role]" (repeatable)
      --no-pin[=true|false]           with --reconcile: don't auto-pin reachable hosts' known_hosts
      --reconcile[=true|false]        run reconcile afterwards to mint keys and re-render
      --tag stringArray               extra tag for every host in the batch (repeatable)
  -y, --yes[=true|false]              apply without confirming (implied when there is no terminal)
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

Aliases: `rm`

Examples:

```sh
  authama ssh host delete work web1
  authama ssh host delete work web1 --purge --revoke --yes
```

Flags:

```
      --no-key-backup[=true|false]   purge without writing an encrypted backup first (the key is then unrecoverable)
      --purge[=true|false]           also delete the key files, if the host was the only thing naming the key
      --revoke[=true|false]          also revoke the deployed key from targets (with --yes)
  -y, --yes[=true|false]             skip confirmation prompts
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

Reads an ssh setup into the manifest and inventory, replacing them (a non-empty
manifest needs --force, and both are backed up first). PATH defaults to
~/.ssh/config and may be a config file or a folder:

  a folder with a config file    that file, whose Includes pull in the rest
                                 (config.d/, profiles/<name>/config, …)
  a folder with ssh/config       that file, for a config home such as ~/.config
  any other folder               every file named config or ending .conf, up to
                                 four levels down, read in path order

Relative Includes resolve against the folder, as ssh resolves them against
~/.ssh. Importing another machine's copy of ~/.ssh, an IdentityFile under
~/.ssh/ is looked for in that copy, and its keys are adopted into the
profile directories here (~/.ssh/profiles/, or the directory
defaults.key_root names).

```
authama ssh import [PATH] [flags]
```

Examples:

```sh
  authama ssh import --dry-run
  authama ssh import ~/old-laptop/.ssh --yes
  authama ssh import ~/.config --dry-run
```

Flags:

```
      --dry-run[=true|false]   parse and report without writing
  -f, --force[=true|false]     replace an existing non-empty manifest (backed up first)
  -y, --yes[=true|false]       import without confirming (implied when there is no terminal)
```

## authama ssh key

Manage a profile's keys (add/list/delete).

```
authama ssh key
```

Subcommands:

- [`add`](#authama-ssh-key-add) - Declare a key on a profile and mint it
- [`adopt`](#authama-ssh-key-adopt) - Manage a key where it already is in ~/.ssh, without moving or renaming it
- [`delete`](#authama-ssh-key-delete) - Delete a key (refuses while a host uses it)
- [`lifetime`](#authama-ssh-key-lifetime) - Set how long a key lives before rotation is due, and count its expiry again
- [`list`](#authama-ssh-key-list) - Every key with its fingerprint, expiry, hosts and state
- [`regen-pub`](#authama-ssh-key-regen-pub) - Rebuild public keys from their private key or 1Password item
- [`reserve`](#authama-ssh-key-reserve) - Keep a loose ~/.ssh key on purpose (doctor stops calling it loose)
- [`revoke`](#authama-ssh-key-revoke) - Remove a key from every target it is deployed to, and never deploy it again
- [`root`](#authama-ssh-key-root) - Show or move the directory under ~/.ssh that holds the profile directories
- [`unreserve`](#authama-ssh-key-unreserve) - Drop a key's reservation (doctor lists it as loose again)

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
      --host string               wire an existing host in this profile to the key
      --passphrase[=true|false]   protect the new key (prompts without echo)
      --rotate-after-days int     rotation interval for this key (default: the manifest's defaults.rotate_after_days)
      --type string               key algorithm (default: the manifest's defaults.key_type)
```

## authama ssh key adopt

Declares an existing private key - ~/.ssh/id_ed25519, ~/.ssh/work/deploy_key -
as a profile's key, and records it. authama then renders, deploys, rotates and
audits it like any other, at its own path: it never moves, renames or deletes
the file. Rotation archives the outgoing pair in the profile's old/ and writes
the successor to the same path, so other tools keep finding it.

<path> is relative to ~/.ssh (a leading ~/.ssh/ is accepted). --host wires it
to a host in the profile.

```
authama ssh key adopt <profile/key> <path> [flags]
```

Examples:

```sh
  authama ssh key adopt personal/github ~/.ssh/id_ed25519 --host github-personal
  authama ssh key adopt work/deploy work/deploy_key
```

Flags:

```
      --host string        wire the key to this host in the profile
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama ssh key delete

Delete a key (refuses while a host uses it).

Without --purge the manifest declaration and the inventory record go away
and the key files stay on disk, so an accidental delete costs nothing.
--purge deletes the files too, after writing an encrypted backup.

Name several keys, profiles or host aliases, a --category or --all to delete
each key among them that no host uses; the ones a host uses are named and kept.

```
authama ssh key delete [<profile/key>|profile|alias ...] [--category <name>] [--all] [flags]
```

Aliases: `rm`

Examples:

```sh
  authama ssh key delete work/deploy
  authama ssh key delete work/deploy --revoke --purge --yes
```

Flags:

```
      --all[=true|false]             delete every key no host uses
      --category string              delete every key no host uses in profiles of this category
      --no-key-backup[=true|false]   purge without writing an encrypted backup first (the key is then unrecoverable)
      --purge[=true|false]           also delete the key files from disk
      --revoke[=true|false]          revoke the deployed public key from its targets first
  -y, --yes[=true|false]             skip the confirmation prompt
```

## authama ssh key lifetime

Sets a key's rotate_after_days and counts its expiry date again from when the
key was created. A preset names a common policy: annual (365 days, the usual
floor for SSH user keys), semiannual (180), quarterly (90) or monthly (30).
Anything from 1 to 1825 days is accepted.

--default sets the lifetime every key without its own inherits, and takes the
lifetime alone. --clear drops a key's own value, so it inherits the default.

```
authama ssh key lifetime <profile/key> <days|annual|semiannual|quarterly|monthly> [flags]
```

Examples:

```sh
  authama ssh key lifetime work/work_gh-ed25519 quarterly
  authama ssh key lifetime work/work_gh-ed25519 120
  authama ssh key lifetime --default annual
  authama ssh key lifetime work/work_gh-ed25519 --clear
```

Flags:

```
      --clear[=true|false]     drop the key's own lifetime, so it inherits the default
      --default[=true|false]   set the lifetime every key without its own inherits
  -y, --yes[=true|false]       skip the confirmation prompt
```

## authama ssh key list

Every key with its fingerprint, expiry, hosts and state.

```
authama ssh key list [profile|alias|key] [flags]
```

Aliases: `ls`

Examples:

```sh
  authama ssh key list
  authama ssh key list work
```

Flags:

```
      --csv[=true|false]   print the keys as CSV, one row each, header first
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
      --all[=true|false]       every key in the manifest
      --category string        every key in profiles of this category
      --dry-run[=true|false]   report what would change, and change nothing
  -y, --yes[=true|false]       skip the confirmation prompt
```

## authama ssh key reserve

reserve records that a key file directly in ~/.ssh - one no Host block uses - is kept
deliberately, for example for a devbox VM you have not made yet. doctor then lists it
under "reserved keys" as information instead of a loose key (and --strict passes),
and `authama ssh vm adopt-key` or `vm sync` release the reservation when they adopt it.

The argument is the file's name (docker-tunnel), ~/.ssh/docker-tunnel, or its .pub.
The reservation lives in manifest.json under reserved_keys, with the public key's
fingerprint, so doctor notices if the file is replaced or removed. The key file itself
is never read beyond its .pub, moved or changed. Reserving a reserved key updates it.

```
authama ssh key reserve <name|path> [flags]
```

Examples:

```sh
  authama ssh key reserve docker-tunnel --for devbox --note "Docker VM, later"
  authama ssh key reserve ~/.ssh/old-laptop --dry-run
```

Flags:

```
  -n, --dry-run[=true|false]   show what would be recorded; change nothing
      --for string             what the key is kept for, e.g. devbox (shown by doctor)
      --note string            a note to keep with it
  -y, --yes[=true|false]       do not ask for confirmation
```

## authama ssh key revoke

Asks each target the key is deployed to, through its host's provider, to remove
it: GitHub and GitLab through their CLIs, a cloud API, a server's
authorized_keys. The deployments it was removed from are dropped from its
record, and its fingerprint is recorded in revoked.json, so deploy and
rollback refuse it from then on. A target that cannot be reached, or whose
provider cannot remove keys, is named so you can remove it by hand.

The key's declaration, hosts and files stay. Mint its successor next:
`ssh keygen <profile/key> --force` now, or `ssh rotate <profile/key>` staged.

Name several keys, profiles or host aliases, a --category or --all to revoke
each minted key among them in turn; one that fails is named and the rest go on.

```
authama ssh key revoke [<profile/key>|profile|alias ...] [--category <name>] [--all] [flags]
```

Examples:

```sh
  authama ssh key revoke work/work_gh-ed25519 --reason "laptop lost"
  authama ssh key revoke work personal/personal_gh-ed25519 --yes
  authama ssh key revoke --category clients --yes
```

Flags:

```
      --all[=true|false]   revoke every minted key
      --category string    revoke every minted key in profiles of this category
      --reason string      why, recorded with the revocation (e.g. "laptop lost")
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama ssh key root

With no argument, prints the key root: the directory under ~/.ssh that holds
the profile directories, ~/.ssh/profiles/ unless defaults.key_root names
another.

With <dir>, makes ~/.ssh/<dir>/ the key root and moves the whole tree there in
one rename: private keys, .pub files, old/ archives and category config files.
The inventory follows and ~/.ssh/config is rendered again, so ssh finds every
key at its new path. Keys adopted where they are stay put. ~/.ssh is
snapshotted first.

<dir> is one directory name, not hidden and not one of ssh's own files.
"profiles" returns to the default. It refuses when ~/.ssh/<dir> already
exists beside the current root: merge the two by hand first.

```
authama ssh key root [<dir>] [flags]
```

Examples:

```sh
  authama ssh key root
  authama ssh key root keys
  authama ssh key root profiles --yes
```

Flags:

```
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama ssh key unreserve

Drop a key's reservation (doctor lists it as loose again).

```
authama ssh key unreserve <name> [flags]
```

Examples:

```sh
  authama ssh key unreserve docker-tunnel
```

Flags:

```
  -y, --yes[=true|false]   do not ask for confirmation
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
  -f, --force[=true|false]           overwrite existing keys (prompts; replaced keys move to old/)
      --no-key-backup[=true|false]   overwrite without writing an encrypted backup first (any earlier predecessor is lost)
      --no-pin[=true|false]          don't auto-pin reachable hosts' known_hosts
      --passphrase[=true|false]      protect newly minted keys (prompts without echo)
  -y, --yes[=true|false]             answer yes to overwrite prompts
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
      --all[=true|false]     scan every profile's hosts
      --force[=true|false]   re-scan already-trusted hosts and add any new keys
  -y, --yes[=true|false]     pin without confirming (implied when there is no terminal)
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
      --all[=true|false]   pin every host in the manifest
  -p, --port int           port for an unmanaged host (default 22)
  -y, --yes[=true|false]   trust scanned keys without prompting
```

## authama ssh list

Filterable tree across profiles.

```
authama ssh list [flags]
```

Aliases: `ls`

Examples:

```sh
  authama ssh list
  authama ssh list --profile work --tag prod
  authama ssh list --type vcs
```

Flags:

```
      --csv[=true|false]   print the hosts as CSV, one row each, header first
      --profile string     filter to one profile
      --provider string    filter by provider
      --tag string         filter by host tag
      --type string        provider category, e.g. vcs
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

## authama ssh naming

The naming rules live in the manifest's defaults.naming, a profile's own naming
over them, and each profile's code name in profiles.<name>.code:

  alias          the alias template, {code}-{env}-{role}-{nn} by default;
                 {env?} is optional, {nnn} is three digits
  key_name       the key-name template, {profile}_{alias}-{algo} by default
  envs, roles    the values {env} and {role} may take (empty: any)
  enforce        off, warn or error, when a host is added
  start          the first number, 0 (00) or 1 (01)
  code_fallback  a profile without a code: none, or profile (its name)

`host add <profile> --env prd --role web` names a server by them. No change to
them, or to a code, renames a key already minted.

```
authama ssh naming
```

Subcommands:

- [`check`](#authama-ssh-naming-check) - List the hosts whose alias does not follow the naming rules
- [`key-template`](#authama-ssh-naming-key-template) - Set how new keys are named, keeping every existing key's name
- [`set`](#authama-ssh-naming-set) - Set the naming rules, for every profile or for one
- [`show`](#authama-ssh-naming-show) - Show the naming rules in effect, and each profile's code and own rules

## authama ssh naming check

List every server whose alias does not follow the naming rules. Hosts named before
the rules keep their names, so this is a report, not an error, unless --strict.
VM hosts are devboxes, not servers, and are not checked.

```
authama ssh naming check [flags]
```

Examples:

```sh
  authama ssh naming check
  authama ssh naming check --strict
```

Flags:

```
      --strict[=true|false]   exit non-zero when any host breaks the rules
```

## authama ssh naming key-template

Set defaults.naming.key_name: how the key of a new host is named, from
{profile}, {alias}, {code}, {env}, {role}, {nn} and {algo}. It starts with
{profile}_ and ends with -{algo}, which is how a key's profile and type are read
back. Every host the new template would rename keeps the key name it has now,
written into the manifest as its key_name, so no key is renamed or left behind;
only hosts added from now on follow the template.

```
authama ssh naming key-template <template> [flags]
```

Examples:

```sh
  authama ssh naming key-template '{profile}_{code}_{env}_{role}_{nn}-{algo}'
  authama ssh naming key-template '{profile}_{alias}-{algo}' --yes
```

Flags:

```
      --profile string     set this profile's own key template instead of the manifest's
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama ssh naming set

Set the naming rules: the manifest's (defaults.naming), or with --profile that
profile's own, which override the manifest's field by field. Only the flags given
change; an empty value (--alias '') goes back to inheriting. Every existing host
keeps its key name, written into the manifest when a change would alter it.

```
authama ssh naming set [flags]
```

Examples:

```sh
  authama ssh naming set --envs prd,stg,dev --enforce warn
  authama ssh naming set --profile adelsaiq --alias '{code}-{role}-{nn}' --start 1
  authama ssh naming set --code-fallback profile
```

Flags:

```
      --alias string           the alias template, from {code}, {env}, {role} and a number such as {nn}; {env?} is optional
      --code-fallback string   the {code} of a profile with none of its own: none or profile (its name)
      --enforce string         off, warn or error, when a host is added
      --envs strings           the environments {env} may be, comma-separated (empty: any)
      --profile string         set this profile's own rules instead of the manifest's
      --roles strings          the roles {role} may be, comma-separated (empty: any)
      --start int              the first number: 0 (00) or 1 (01)
```

## authama ssh naming show

Show the naming rules in effect, and each profile's code and own rules.

```
authama ssh naming show
```

Examples:

```sh
  authama ssh naming show
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

Manage the scheduled expiry notifier and desktop notifications.

```
authama ssh notify
```

Subcommands:

- [`doctor`](#authama-ssh-notify-doctor) - Say whether desktop notifications work here, and why not
- [`install`](#authama-ssh-notify-install) - Install the scheduled expiry notifier
- [`ping`](#authama-ssh-notify-ping) - Hand a notification to the running web UI (authama serve), which shows it
- [`send`](#authama-ssh-notify-send) - Show a desktop notification from this process
- [`test`](#authama-ssh-notify-test) - Fire a test notification

## authama ssh notify doctor

Say whether desktop notifications work here, and why not.

```
authama ssh notify doctor [-title T] [-subtitle S] [-body B] [-group G] [-level info|warn|alert] [-sound] [-json]
```

Examples:

```sh
  authama ssh notify doctor
```

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
  -y, --yes[=true|false]   install without confirming (implied when there is no terminal)
```

## authama ssh notify ping

Hand a notification to the running web UI (authama serve), which shows it.

```
authama ssh notify ping [-title T] [-subtitle S] [-body B] [-group G] [-level info|warn|alert] [-sound] [-json]
```

Examples:

```sh
  authama ssh notify ping -title "Deploy finished" -body "billing-api is on v1.4.0" -group deploy
```

## authama ssh notify send

Show a desktop notification from this process.

```
authama ssh notify send [-title T] [-subtitle S] [-body B] [-group G] [-level info|warn|alert] [-sound] [-json]
```

Examples:

```sh
  authama ssh notify send -title Backup -body "42 files"
  authama ssh notify send -json < notification.json
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
      --category string       file the profile under profiles/<category>/ and render its hosts into profiles/<category>/config
      --code string           the organisation's code name, the first part of its servers' aliases (simbi for simtabi)
      --key-comment string    comment template for keys minted from now on: {profile} {alias} {key} {date} {device} (default {profile}/{alias} {date})
      --key-name string       profile key name (shared scope)
      --key-scope string      per_service | shared (shared: one key, --key-name, for every host) (default "per_service")
      --shared[=true|false]   shorthand for --key-scope shared (--shared=false: per_service)
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

Aliases: `rm`

Examples:

```sh
  authama ssh profile delete old
  authama ssh profile delete old --purge --revoke --yes
```

Flags:

```
      --no-key-backup[=true|false]   purge without writing an encrypted backup first (the keys are then unrecoverable)
      --purge[=true|false]           also delete the profile's key files and directory
      --revoke[=true|false]          also revoke deployed keys from targets (with --yes)
  -y, --yes[=true|false]             skip confirmation prompts
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
      --category string       move the profile under profiles/<category>/ (its key directory and inventory records follow); "" returns it to the flat layout
      --code string           the organisation's code name, the first part of its servers' aliases; "" clears it
      --key-comment string    comment template for keys minted from now on: {profile} {alias} {key} {date} {device} (default {profile}/{alias} {date}); "" inherits defaults.key_comment
      --key-name string       the shared key every host in this profile uses (key_scope: shared)
      --key-scope string      per_service | shared
      --shared[=true|false]   shorthand for --key-scope shared (--shared=false: per_service)
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
      --export[=true|false]   write the default catalog to <home>/providers.json to customize
      --force[=true|false]    overwrite an existing file (with --export)
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
      --dry-run[=true|false]      preview without writing
      --no-pin[=true|false]       don't auto-pin reachable hosts' known_hosts
      --passphrase[=true|false]   protect newly minted keys (prompts without echo)
  -y, --yes[=true|false]          apply without confirming (implied when there is no terminal)
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
  -i, --identity string    age identity file (else $AUTHAMA_AGE_IDENTITY_FILE)
  -y, --yes[=true|false]   skip the confirmation prompt
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
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama ssh rotate

Zero-downtime staged key rotation.

The key being replaced is archived to the profile's old/ dir, but only one
predecessor is kept. Rotating again stops while that predecessor is still
authorised on a host it was never removed from; --evict-old replaces it anyway.
An encrypted bundle is written first unless --no-key-backup is given.

Name one key, several, whole profiles or host aliases, every key in a
category, or --all. Each key rotates on its own: one that fails is left
untouched and the rest still rotate. A key no host uses is skipped, since
there is nothing to deploy it to.

```
authama ssh rotate [<[profile/]key>|profile|alias ...] [--category <name>] [--all] [flags]
```

Aliases: `recycle`

Examples:

```sh
  authama ssh rotate work/deploy
  authama ssh rotate --all --dry-run
  authama ssh rotate --all --check   # test every key first, change nothing
  authama ssh rotate --all --no-key-backup --yes   # a clean rotation, no backup bundle
  authama ssh rotate --all --force --yes   # replace locally what cannot rotate in place
  authama ssh rotate --failed --yes        # run again the keys the last run left
  authama ssh rotate --category clients --yes
```

Flags:

```
      --all[=true|false]                rotate every key a host uses
      --allow-unverified[=true|false]   commit even if a target can't auto-verify
      --category string                 rotate every key in profiles of this category
      --check[=true|false]              test each key's rotation and change nothing: every server answers and the current key logs in, or the target takes it by hand
      --dry-run[=true|false]            list the keys that would rotate, and change nothing
      --evict-old[=true|false]          replace the previous key in old/ even while a host it was never removed from still accepts it
      --failed[=true|false]             run again the keys the last rotation or dry run left (not rotated, or would fail)
      --force[=true|false]              replace on this computer (as ssh keygen --force) each key that cannot be rotated in place - never deployed, or its host unreachable - to deploy afterwards
      --no-key-backup[=true|false]      rotate without writing an encrypted backup first (the earlier predecessor is lost)
      --passphrase[=true|false]         protect the rotated-in key (prompts without echo)
  -y, --yes[=true|false]                skip the confirmation prompt
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
authama ssh snapshots list [flags]
```

Aliases: `ls`

Examples:

```sh
  authama ssh snapshots list
```

Flags:

```
      --csv[=true|false]   print the snapshots as CSV, one row each, header first
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
  -y, --yes[=true|false]   skip the confirmation prompt
```

## authama ssh termius

Termius keeps its hosts in its own encrypted vault, offers no command line or API
to add them, and its SSH-config importer does not follow Include. Write its CSV
import with `authama export hosts --format termius`; the CSV carries no keys, and
`termius keys` lists and copies the key to attach to each host.

```
authama ssh termius
```

Subcommands:

- [`keys`](#authama-ssh-termius-keys) - List the Termius key label for each host, or copy one host's private key to the clipboard

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
      --clear[=true|false]         empty the clipboard and exit
      --clear-after duration       after copying, wait this long (e.g. 60s) and empty the clipboard
      --include-git[=true|false]   also list git hosts, as 'export hosts --format termius --include-git' exports them (an ALIAS finds a git host without it)
      --print[=true|false]         print the private key to stdout instead of copying it (warns on stderr)
      --skip stringArray           leave out this alias (repeatable)
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
each connection asks the hypervisor for the VM's current address. Built in:
parallels, vmpal, vmware, utm, lima, orbstack, podman. OrbStack and Podman run their own SSH and keys;
they are listed, but authama does not take their keys over or make them hosts (each has
its own Docker: OrbStack's context is orbstack). A VM host whose guest runs Docker gets a
Docker context ssh://<alias> from `vm sync`, shared-connection SSH (vm.multiplex), and an
engine check in doctor.

Which hypervisors you use is vm.hypervisors (AUTHAMA_VM_HYPERVISORS); any other tool can be
added as a command driver in authama.json's "hypervisors" section, and the VMs you work
in declared in its "devboxes" section (docs/tools/vm.md). `vm sync` keeps the hosts in
step with the VMs; `vm adopt-key` turns a key you already use for a VM into a VM host.

```
authama ssh vm
```

Subcommands:

- [`add`](#authama-ssh-vm-add) - Add a VM as a host: its address comes from the hypervisor at connect time
- [`adopt-key`](#authama-ssh-vm-adopt-key) - Bring a loose ~/.ssh key under management as a devbox VM host's key
- [`docker-context`](#authama-ssh-vm-docker-context) - Create or update a Docker context ssh://<alias> for a VM host
- [`list`](#authama-ssh-vm-list) - List every hypervisor's VMs, and which manifest host uses each
- [`sync`](#authama-ssh-vm-sync) - Keep VM hosts in step with their hypervisors: pins, guest keys, Docker contexts, gone VMs

## authama ssh vm add

Add a VM as a host: its address comes from the hypervisor at connect time.

```
authama ssh vm add <profile> <alias> [flags]
```

Examples:

```sh
  authama ssh vm add lab dev --hypervisor parallels --vm Ubuntu --user me
```

Flags:

```
      --hypervisor string   parallels, vmpal, vmware, utm, lima, orbstack, podman, or a custom one from authama.json
      --key-name string     use this key instead of the derived <profile>_<alias>-<algo> name
  -u, --user string         guest user
      --vm string           the VM's name as the hypervisor lists it (see 'authama ssh vm list')
  -y, --yes[=true|false]    add without confirming (implied when there is no terminal)
```

## authama ssh vm adopt-key

adopt-key takes a key you already use for a VM (say ~/.ssh/docker-tunnel), copies it into
a profile as the key of a new VM host, records it, and re-renders ~/.ssh/config. The
original stays where it is unless --move; the private key is copied, never read into
memory for anything else. Run `authama ssh vm sync` afterwards to pin the guest's host
key, install the public key in the guest if it lacks it, and create the Docker context.

At a terminal, whatever is missing is asked for: the key from the loose keys in ~/.ssh,
the VM from the enabled hypervisors' VMs (authama.json vm.hypervisors).

```
authama ssh vm adopt-key [<key> [<profile> [<alias>]]] [flags]
```

Examples:

```sh
  authama ssh vm adopt-key
  authama ssh vm adopt-key devbox-key devbox devbox --hypervisor parallels --vm Ubuntu --user me --dry-run
```

Flags:

```
      --docker-context[=true|false]   also create Docker context ssh://<alias>
  -n, --dry-run[=true|false]          show what would happen and change nothing
      --hypervisor string             the VM's hypervisor: parallels, vmpal, vmware, utm, lima, orbstack, podman
      --key-name string               the adopted key's name in the profile (default: the file's name)
      --move[=true|false]             remove the originals from ~/.ssh once the copy is verified identical
  -u, --user string                   guest user
      --vm string                     the VM's name as the hypervisor lists it (see 'authama ssh vm list')
  -y, --yes[=true|false]              adopt without confirming or asking (implied when there is no terminal)
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

Aliases: `ls`

Examples:

```sh
  authama ssh vm list
```

## authama ssh vm sync

For every VM host, ask its hypervisor what became of the VM:
  running    pin its host key when nothing is pinned, re-install the host's public key in
             the guest when the guest refuses it, create or refresh Docker context ssh://<alias>
  gone       back up the manifest, remove the host entry (never its key files) and the
             Docker context authama made for it (exported first)
  suspended  reported and left alone - sync never starts, stops or deletes a VM
It also adds the devboxes authama.json declares, and removes Docker contexts authama made
whose host is gone. Running it twice changes nothing the second time.

--install-schedule runs `sync --quiet --yes` daily with launchd, systemd/cron or Task Scheduler.

```
authama ssh vm sync [flags]
```

Examples:

```sh
  authama ssh vm sync --dry-run
  authama ssh vm sync --yes
  authama ssh vm sync --install-schedule
```

Flags:

```
  -n, --dry-run[=true|false]            show the plan and change nothing
      --install-schedule[=true|false]   run a quiet sync daily (sync --quiet --yes) with this system's scheduler
      --no-probe[=true|false]           ask the hypervisors only; do not connect to running guests
  -q, --quiet[=true|false]              print only what changed and errors (for a scheduled run)
      --remove-schedule[=true|false]    remove the scheduled sync
      --repin[=true|false]              replace a VM host's pin when the guest's host key changed (vm.repin_changed)
  -y, --yes[=true|false]                apply without confirming (implied when there is no terminal)
```

## authama token

Builds the credential files Composer, npm, Docker, git and curl read, from a
config that names environment variables (never secrets), and lists, copies or
removes what is installed. Files are written atomically and owner-only.

Config: --config, $AUTHAMA_TOKENS_CONFIG, ./authama-tokens.json, or tokens.json in the config home.

```
authama token
```

Aliases: `auth`

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
      --config string              the token config file (default: discovered)
      --creds-store-mode string    when Docker keeps a registry in a credential store: login (docker login --password-stdin) or refuse
      --dry-run[=true|false]       print the resolved credentials as JSON, secrets masked; write nothing
      --exports[=true|false]       print COMPOSER_AUTH as a shell export instead of writing files
      --no-env-hint[=true|false]   do not print the CI hint after writing Composer's file
      --project string             write the project's auth.json and .npmrc instead of the user ones
      --source string              load VAR=value lines from this env file first (set variables win)
      --target strings             only these tools (repeatable: composer, npm, docker, git, netrc, pypi, cargo, gem, gradle, maven, yarn, helm, helm-oci, gh)
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
      --target strings   only these tools (repeatable: composer, npm, docker, git, netrc, pypi, cargo, gem, gradle, maven, yarn, helm, helm-oci, gh)
```

## authama token copy

Puts HOST's secret from TARGET's credential file on the clipboard, through
pbcopy, wl-copy, xclip, xsel or clip, and prints only what it copied. The
secret is written nowhere else; the history records that it was copied, and
from which file, never the value.

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

Aliases: `rm`

Examples:

```sh
  authama token delete npm registry.npmjs.org
  authama token delete composer github.com --project . --yes
```

Flags:

```
      --project string     edit the project's auth.json or .npmrc instead of the user one
  -y, --yes[=true|false]   skip the confirmation prompt
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
      --json[=true|false]   print the placeholders as JSON, masked, instead of as files
      --project string      write the placeholder auth.json and .npmrc into this directory
      --target strings      only these tools (repeatable: composer, npm, docker, git, netrc, pypi, cargo, gem, gradle, maven, yarn, helm, helm-oci, gh)
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
      --dry-run[=true|false]   check the file parses and say where it would go; write nothing
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
      --target strings   start from these tools' templates (repeatable: composer, npm, docker, git, netrc, pypi, cargo, gem, gradle, maven, yarn, helm, helm-oci, gh)
```

## authama token list

List the tokens installed in the credential files (secrets masked).

```
authama token list [flags]
```

Aliases: `ls`

Examples:

```sh
  authama token list
  authama token list --target npm --target git
  authama token list --project . --json
```

Flags:

```
      --csv[=true|false]      print the list as CSV, header first; never the secrets
      --json[=true|false]     print the list as JSON
      --project string        read the project's auth.json and .npmrc instead of the user ones
      --reveal[=true|false]   show the secret values (asks first; --yes when not at a terminal)
      --target strings        only these tools (repeatable: composer, npm, docker, git, netrc, pypi, cargo, gem, gradle, maven, yarn, helm, helm-oci, gh)
  -y, --yes[=true|false]      reveal without asking
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
      --target string             the tool: npm, docker, git, netrc, pypi, cargo, gem, gradle, maven, yarn, helm, helm-oci or gh (Composer uses --service)
      --username string           the user name, for http-basic, docker, git and netrc
```

## authama token shell-aliases

The aliases auth-token-manager installed, pointed at this tool: atm runs
`authama token`, atm-ui the web UI, atm-doctor `token audit`,
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
      --file string              the startup file (default: your shell's rc file)
      --install[=true|false]     add or update the alias block
      --login[=true|false]       use the login profile (.zprofile, .bash_profile) instead of the rc file
      --no-backup[=true|false]   do not copy the file to <file>.authama.bak first
      --print[=true|false]       print the alias lines, for eval
      --quiet[=true|false]       print nothing on success
      --uninstall[=true|false]   remove the alias block
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
      --json[=true|false]     print the entry as JSON (the same fields as token list --json)
      --project string        read the project's auth.json or .npmrc instead of the user one
      --reveal[=true|false]   show the secret value (asks first; --yes when not at a terminal)
  -y, --yes[=true|false]      reveal without asking
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
      --config string       the token config file (default: discovered)
      --json[=true|false]   machine-readable output (scripting)
      --project string      report the project's auth.json and .npmrc instead of the user ones
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

## authama webhook

A webhook sends each new event (a change finished, a watched file changed) to an endpoint you add,
signed as Standard Webhooks do. `authama serve` delivers them when webhooks.enabled is on (it is off by
default); `authama webhook deliver` sends what is due by hand. Endpoints must be https to a public
address unless webhooks.allow_private is on.

```
authama webhook
```

Subcommands:

- [`add`](#authama-webhook-add) - Add an endpoint; its signing secret is shown once
- [`deliver`](#authama-webhook-deliver) - Send every endpoint the events due to it now
- [`disable`](#authama-webhook-disable) - Stop sending to an endpoint; it keeps its place
- [`enable`](#authama-webhook-enable) - Send to an endpoint again, from where it stopped
- [`list`](#authama-webhook-list) - List endpoints and how their delivery stands (never their secrets)
- [`remove`](#authama-webhook-remove) - Remove an endpoint and its secret
- [`test`](#authama-webhook-test) - Send one webhook.test event now

## authama webhook add

Add an endpoint; its signing secret is shown once.

```
authama webhook add <name> <url> [flags]
```

Examples:

```sh
  authama webhook add ops https://hooks.example.com/authama   # prints its signing secret once
  authama webhook add audit https://hooks.example.com/a --event operation.finished
```

Flags:

```
      --event strings   send only events whose type starts with this, whole or without com.simtabi.authama. (repeatable): operation.finished, file.
```

## authama webhook deliver

Send every endpoint the events due to it now.

```
authama webhook deliver
```

Examples:

```sh
  authama webhook deliver   # send what is due now
```

## authama webhook disable

Stop sending to an endpoint; it keeps its place.

```
authama webhook disable <name>
```

Examples:

```sh
  authama webhook disable ops
```

## authama webhook enable

Send to an endpoint again, from where it stopped.

```
authama webhook enable <name>
```

Examples:

```sh
  authama webhook enable ops   # after it was turned off
```

## authama webhook list

List endpoints and how their delivery stands (never their secrets).

```
authama webhook list [flags]
```

Aliases: `ls`

Examples:

```sh
  authama webhook list
  authama webhook list --json
```

Flags:

```
      --json[=true|false]   a JSON array
```

## authama webhook remove

Remove an endpoint and its secret.

```
authama webhook remove <name>
```

Examples:

```sh
  authama webhook remove ops
```

## authama webhook test

Send one webhook.test event now.

```
authama webhook test <name>
```

Examples:

```sh
  authama webhook test ops
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

---

[← Docs index](../../README.md#documentation)
