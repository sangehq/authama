# CLI and web parity

Every `authama` command and flag, and where the web UI (`authama serve`) offers it. The command list comes from the cobra tree; regenerate this page when a command or flag changes. Hidden commands and help-only parents (`ssh`, `ssh key`, `token` and so on) are left out.

The web UI runs every change through the same change engine as the CLI: a preview first, the owner PIN when one is set, and history and audit records. It never renders a private key or a token secret.

## Status

| Status | Meaning |
| --- | --- |
| web | A page or control does the same thing. |
| partial | The page shows or builds it, and part stays in the terminal. |
| CLI-only by design | It reveals a secret, writes to the clipboard or the agent, installs a background service, or runs the server itself. |
| gap | No web equivalent yet, listed under [Open gaps](#open-gaps). |

In the flag column, a plain flag is covered by the page, `(CLI)` is CLI-only by design, and `(gap)` has no web control yet. `--yes`, `--dry-run` and `--confirm` map to the preview and its Confirm button; `--json` and `--csv` map to the page itself, its download, or the JSON API.

## Counts

| | Count |
| --- | --- |
| Commands (runnable, not hidden, not help-only parents) | 184 |
| Help-only parent commands left out | 30 |
| Web | 162 |
| Partial | 3 |
| CLI-only by design | 19 |
| Gap | 0 |
| Flags on those commands | 495 |
| Flags covered | 415 |
| Flags CLI-only by design | 80 |
| Flags with no web control | 0 |


## Matrix

| Command | Status | Web | Flags | Notes |
| --- | --- | --- | --- | --- |
| `(root)` | CLI-only by design | - | `--dev-root` (CLI), `--no-dotenv` (CLI), `--tz` (CLI) | Global flags; `serve --dev-root` runs the web UI in a sandbox. `--tz` is one run; the web uses display.timezone. |
| `api token create` | web | /api-tokens: Create (PIN; the token is shown once) | `--expires` | Shown once, never again; copy button. |
| `api token list` | web | /api-tokens | `--json` | Names, scope and dates; never the token. |
| `api token revoke` | web | /api-tokens: Revoke (PIN) | - |  |
| `checklist` | web | /checklists | `--json`, `--open` (CLI) |  |
| `checklist revoking` | web | /checklist: Revoking | `--stop` |  |
| `checklist show` | web | /checklist?key= | `--json` |  |
| `checklist tick` | web | /checklist: tick box | - |  |
| `checklist untick` | web | /checklist: tick box | - |  |
| `companion info` | CLI-only by design | - | `--json` (CLI) | Desktop companion app. |
| `doctor` | web | /doctor, Fix | `--dry-run`, `--fix`, `--json`, `--strict` (CLI), `--yes` |  |
| `events` | web | /events (filters, CSV) | `--area`, `--csv`, `--json`, `--kind`, `--limit`, `--no-scan` (CLI), `--since`, `--verify` |  |
| `export` | web | /export builder, Copy command, download | `--aggregate`, `--area`, `--columns`, `--command`, `--compact`, `--dedupe`, `--env`, `--exclude`, `--fields`, `--format`, `--group-by`, `--include`, `--include-git`, `--key`, `--kind`, `--limit`, `--narrow`, `--no-header`, `--out`, `--pivot`, `--profile`, `--role`, `--selection`, `--shape`, `--since`, `--sort`, `--source`, `--status`, `--tag`, `--template`, `--termius-groups`, `--tool`, `--totals`, `--transform`, `--until` |  |
| `export pack` | web | /transfer: Export pack | `--clients`, `--dry-run`, `--include`, `--include-git`, `--out`, `--termius-groups` |  |
| `export schedule add` | web | /export: Scheduled exports (PIN) | `--format`, `--to` | Daily export of a saved selection. |
| `export schedule install` | CLI-only by design | - | - | Registers a launchd/systemd/schtasks job. |
| `export schedule list` | web | /export: Scheduled exports | - |  |
| `export schedule remove` | web | /export: Scheduled exports (PIN) | `--format` |  |
| `export schedule run` | web | /export: Run now (PIN) | `--quiet` (CLI) |  |
| `export schedule uninstall` | CLI-only by design | - | - | Background service. |
| `export selections delete` | web | /export: saved selections | `--from`, `--yes` |  |
| `export selections list` | web | /export: saved selections | `--json` |  |
| `export selections save` | web | /export: Save selection | `--from`, `--to`, `--yes` |  |
| `export selections show` | web | /export?selection= | - |  |
| `history` | web | /history (filters, CSV) | `--command`, `--csv`, `--host`, `--json`, `--key`, `--limit`, `--profile`, `--since`, `--source`, `--token`, `--until` |  |
| `history reset` | web | /reset | `--confirm` |  |
| `history show` | web | /history/{op} | - |  |
| `init` | web | /reset: re-initialise home | `--force`, `--no-backup` (CLI), `--yes` | The web always keeps a backup. |
| `logs` | web | /logs (level, since, search, Follow, Download) | `--follow`, `--grep`, `--json`, `--level`, `--limit`, `--since` | The page shows the newest 500; Download has them all. |
| `logs export` | web | /logs: Download | `--grep`, `--level`, `--limit`, `--out`, `--since` |  |
| `logs level` | web | /settings: log.level | - |  |
| `logs path` | web | /logs (footer) | - |  |
| `logs retention` | web | /settings: log.retention_days | - |  |
| `man` | CLI-only by design | - | - | Man pages. |
| `mcp` | CLI-only by design | - | - | MCP server over stdio. |
| `migrate` | CLI-only by design | - | `--force` (CLI), `--yes` (CLI) | One-off v1 migration. |
| `owner fingerprint` | web | /security | - |  |
| `owner forget` | web | /security: Unbind | `--confirm`, `--yes` |  |
| `owner init` | web | /security: Bind | `--yes` |  |
| `owner pin remove` | web | /security: Remove the PIN | - |  |
| `owner pin reset` | CLI-only by design | - | - | Recovery for a forgotten PIN; needs the terminal by design. |
| `owner pin set` | web | /security: Set/Change the PIN | - |  |
| `owner reseal` | web | /security: Reseal | `--yes` |  |
| `owner status` | web | /security | - |  |
| `owner verify` | web | /security, header badge | - |  |
| `repair` | web | /repair (area checkboxes) | `--area`, `--check`, `--yes` |  |
| `reset` | web | /reset: factory reset | `--confirm` |  |
| `serve` | CLI-only by design | - | `--addr` (CLI), `--allow-host` (CLI), `--allow-remote` (CLI), `--new` (CLI), `--no-open` (CLI), `--port` (CLI), `--read-only` (CLI), `--restart` (CLI), `--status` (CLI), `--stop` (CLI), `--stop-all` (CLI), `--token-file` (CLI) | Starts the web UI itself. |
| `serve browsers` | web | /settings: Remembered browsers | `--json` |  |
| `serve browsers forget` | web | /settings: Forget, Forget all | `--all` |  |
| `settings backup-key` | web | /settings: Backup key | - |  |
| `settings init` | web | /settings: Reset to defaults | `--force` |  |
| `settings path` | web | /settings, /about | `--json` |  |
| `settings set` | web | /settings | - |  |
| `settings show` | web | /settings | `--effective`, `--json` |  |
| `settings timezone list` | web | /settings: display.timezone (searchable list) | - |  |
| `settings timezone set` | web | /settings: display.timezone | - |  |
| `settings timezone show` | web | /settings: display.timezone (time there now) | - |  |
| `settings unset` | web | /settings | - |  |
| `settings validate` | web | /settings (checked as you type) | - |  |
| `setup export` | web | /setup: export (PIN-gated encrypt) | `--encrypt`, `--out`, `--passphrase-file` (CLI), `--sign-key` (CLI) | --sign-key stays in the terminal: ssh-keygen may ask for the key's passphrase. |
| `setup import` | web | /setup: import (preview first) | `--allow-risky-options`, `--dry-run`, `--on-conflict`, `--passphrase-file` (CLI), `--require-signature` (CLI), `--signers` (CLI), `--yes` | An upload arrives without its .sig, so signatures are checked in the terminal. |
| `ssh audit` | web | /audit (CSV, Notify) | `--csv`, `--notify` |  |
| `ssh bundle` | web | /snapshots: Bundle | `--keep`, `--output` (CLI), `--recipient`, `--sign-key` (CLI) | Written to the state home dist/. --sign-key stays in the terminal: ssh-keygen may ask for the key's passphrase. |
| `ssh clean` | web | /doctor: Clean | `--adopt`, `--dry-run`, `--yes` |  |
| `ssh config adopt` | web | /ssh-config: Adopt duplicate Host blocks | `--dry-run`, `--yes` | New in this branch. |
| `ssh config check` | web | /ssh-config | - | HAND-EDIT lines show as the Hand edits card, with Keep and Discard. |
| `ssh config export` | web | /ssh-config: download ssh_config | `--flat`, `--out` |  |
| `ssh config keep-edits` | web | /ssh-config: Keep hand edits; offered on a render refused over hand edits | `--dry-run`, `--yes` | The preview is the dry run: it lists each block adopted and the manifest diff. |
| `ssh config render` | web | /ssh-config, overview: Re-render; /ssh-config: Discard hand edits and render | `--dry-run`, `--force`, `--yes` | A render refused over hand edits lists them and offers Keep or Discard. --force is a destructive change (confirm dialog, owner PIN when set); its result names the backup. |
| `ssh config show` | web | /ssh-config | - |  |
| `ssh deploy` | web | key page: Deploy (job) | `--direct`, `--yes` |  |
| `ssh diff` | web | Reconcile preview | - |  |
| `ssh expiry` | web | /keys, /audit | - |  |
| `ssh git clone` | partial | /git-sign: clone helper | `--dry-run` (CLI), `--mode` (CLI), `--profile` (CLI) | The page builds the command; cloning runs in a terminal. |
| `ssh git resolve` | web | /git-sign: clone helper | `--json`, `--mode`, `--profile` |  |
| `ssh git sign apply` | web | /git-sign: Apply | `--dry-run`, `--yes` |  |
| `ssh git sign set` | web | /git-sign: profile form | `--clear`, `--dir`, `--email`, `--key`, `--name`, `--no-apply` (CLI), `--no-sign`, `--yes` | The web always applies. |
| `ssh git sign status` | web | /git-sign | `--json` |  |
| `ssh git sign use` | web | /git-sign: Use for checkouts | `--dry-run`, `--yes` |  |
| `ssh git sign verify` | web | /git-sign?check= | `--json`, `--number`, `--strict` (CLI) |  |
| `ssh git status` | web | /git-sign: Remote mode (?remotes=) | `--json` | Mode and owner per checkout. |
| `ssh git use` | web | /git-sign: Remote mode, Switch (preview) | `--dry-run`, `--json`, `--mode`, `--profile`, `--yes` | Checks with ls-remote on apply. |
| `ssh host add` | web | /hosts: Add a host | `--alias`, `--comment`, `--connect-timeout`, `--env`, `--hostname`, `--key-name`, `--option`, `--port`, `--provider`, `--proxy`, `--proxy-command`, `--proxy-jump`, `--role`, `--section`, `--service`, `--tag`, `--token-env`, `--update`, `--user` | Alias empty: generated from env and role. Routes under Routes. --update: the Update it if it exists box, and a preview of an alias the profile has offers the update. |
| `ssh host apply` | web | /hosts: Add or update in bulk | `--category`, `--create-profile`, `--dry-run`, `--file` (CLI), `--host`, `--no-pin`, `--reconcile`, `--tag`, `--yes` | The web reconcile is always --no-pin; pin from /hosts. |
| `ssh host delete` | web | host page: Delete; --revoke/--purge as a job | `--no-key-backup` (CLI), `--purge`, `--revoke`, `--yes` |  |
| `ssh host edit` | web | host page: Edit | `--alias`, `--comment`, `--connect-timeout`, `--hostname`, `--key-name`, `--option`, `--port`, `--provider`, `--proxy`, `--proxy-command`, `--proxy-jump`, `--remove-alias`, `--remove-service`, `--section`, `--service`, `--tag`, `--token-env`, `--unset-option`, `--untag`, `--user` | Routes section filled in with the current values: a name or option taken out is --remove-alias/--unset-option. |
| `ssh host variant remove` | web | host page: Route variants, remove | - | New in this branch. |
| `ssh host variant set` | web | host page: Route variants, Add or change one | `--comment`, `--connect-timeout`, `--hostname`, `--option`, `--port`, `--proxy`, `--proxy-command`, `--proxy-jump`, `--remove-service`, `--section`, `--service`, `--unset-option` | Empty fields leave a variant's settings; unset one from the CLI or remove and set again. |
| `ssh import` | web | /import | `--dry-run`, `--force`, `--yes` | Takes in extra Host names as aliases, ProxyCommand (cloudflared, none, a command), options, section and comment lines. |
| `ssh key ack` | web | key page: Acknowledge; /keys: Acknowledge new keys | `--all`, `--category` |  |
| `ssh key add` | web | /keys: Add a key | `--host`, `--passphrase` (CLI), `--rotate-after-days`, `--type` | A passphrase stays in the terminal. |
| `ssh key adopt` | web | /structure: keep in place | `--host`, `--yes` |  |
| `ssh key delete` | web | /keys: delete, bulk Delete (job) | `--all`, `--category`, `--no-key-backup` (CLI), `--purge`, `--revoke`, `--yes` |  |
| `ssh key lifetime` | web | key page: Set lifetime | `--clear`, `--default`, `--yes` |  |
| `ssh key list` | web | /keys | `--csv`, `--json`, `--new`, `--since` |  |
| `ssh key regen-pub` | web | /keys, key page | `--all`, `--category`, `--dry-run`, `--yes` |  |
| `ssh key reserve` | web | /keys: Reserve | `--dry-run`, `--for`, `--note`, `--yes` |  |
| `ssh key revoke` | web | key page, bulk Revoke (job) | `--all`, `--category`, `--direct`, `--reason`, `--yes` |  |
| `ssh key root` | web | /keys, /structure | `--yes` |  |
| `ssh key unreserve` | web | /keys | `--yes` |  |
| `ssh keygen` | web | /keys: Mint | `--force` (CLI), `--no-key-backup` (CLI), `--no-pin`, `--passphrase` (CLI), `--yes` | Runs with --no-pin; overwrite and passphrase stay in the terminal. |
| `ssh knownhosts init` | web | /hosts: Pin host keys (job) | `--all`, `--force`, `--yes` |  |
| `ssh knownhosts pin` | web | host page: Pin its keys (job); /hosts: Pin host keys for --all | `--all`, `--port` (CLI), `--yes` | A form never sends a free hostname to ssh-keyscan: --port (unmanaged hosts) is CLI-only. |
| `ssh list` | web | /hosts (filters, CSV) | `--csv`, `--profile`, `--provider`, `--tag`, `--type` |  |
| `ssh load` | CLI-only by design | - | - | Adds keys to the running ssh-agent. |
| `ssh naming check` | web | /naming | `--strict` (CLI) |  |
| `ssh naming key-template` | web | /naming | `--profile`, `--yes` |  |
| `ssh naming set` | web | /naming | `--alias`, `--code-fallback`, `--enforce`, `--envs`, `--profile`, `--roles`, `--start` |  |
| `ssh naming show` | web | /naming | - |  |
| `ssh net` | web | /net | - |  |
| `ssh notify doctor` | CLI-only by design | - | - | Desktop helper diagnosis. |
| `ssh notify install` | CLI-only by design | - | `--yes` (CLI) | Background service. |
| `ssh notify ping` | CLI-only by design | - | - | Helper verb. |
| `ssh notify send` | CLI-only by design | - | - | Helper verb. |
| `ssh notify test` | web | /settings: Test notification | - |  |
| `ssh profile add` | web | /profiles: Add | `--category`, `--code`, `--key-comment`, `--key-name`, `--key-scope`, `--shared` | --shared is the Key scope select. |
| `ssh profile delete` | web | /profiles: Delete; --revoke/--purge as a job | `--no-key-backup` (CLI), `--purge`, `--revoke`, `--yes` |  |
| `ssh profile edit` | web | /profiles: Edit, Category, Code, Comment | `--category`, `--code`, `--key-comment`, `--key-name`, `--key-scope`, `--shared` | --shared is Change key scope. |
| `ssh providers` | partial | /settings: Export providers; host form provider list | `--export`, `--force` | No page lists credential state. |
| `ssh reconcile` | web | overview/keys: Reconcile | `--dry-run`, `--no-pin`, `--passphrase` (CLI), `--yes` | Runs with --no-pin; host keys are pinned from /hosts. |
| `ssh recover` | web | key page: Break-glass | - | New in this branch. The no-argument fixkeys tool stays CLI. |
| `ssh restore` | web | /transfer, /snapshots: Restore bundle (job; checks bundle_signers, Require a trusted signature) | `--allow-risky-options`, `--identity` (CLI), `--require-signature`, `--signers` (CLI), `--yes` | --signers: the web trusts bundle_signers only. |
| `ssh rollback` | web | key page: Roll back (job) | `--direct`, `--yes` |  |
| `ssh rotate` | web | key page / bulk Rotate (job) | `--all`, `--allow-unverified`, `--category`, `--check`, `--direct`, `--dry-run`, `--evict-old`, `--failed`, `--force`, `--keep-old`, `--no-key-backup`, `--passphrase` (CLI), `--yes` | --failed: the review link; --keep-old: Take the previous key off servers unticked. |
| `ssh show` | web | profile/host/key pages | - |  |
| `ssh snapshots list` | web | /snapshots | `--csv` |  |
| `ssh snapshots prune` | web | /snapshots: Prune | `--keep` |  |
| `ssh snapshots restore` | web | /snapshots/{id}: Restore | `--yes` |  |
| `ssh service add` | web | /services: Add a catalog service | `--binary`, `--kind`, `--label` |  |
| `ssh service edit` | web | /services: Edit a catalog service | `--binary`, `--kind`, `--label` | Dedicated edit form; the add form also updates an existing name. |
| `ssh service health` | web | /services: Health | `--json` | Read-only; never starts a tunnel or changes WARP. |
| `ssh service list` | web | /services | `--json` |  |
| `ssh service remove` | web | /services: Remove | `--yes` |  |
| `ssh service show` | web | /services: Catalog | - |  |
| `ssh service status` | web | /services: Health | `--json` | Alias of health. |
| `ssh structure approve` | web | /structure: drag-and-drop, Approve | `--dry-run`, `--name`, `--plan`, `--profile`, `--yes` |  |
| `ssh structure check` | web | /structure (live watcher) | `--notify` (CLI), `--quiet` (CLI) | Auto-applies rules you enabled with --auto. |
| `ssh structure classify` | web | /structure (suggestions, evidence) | `--dry-run`, `--json` |  |
| `ssh structure dismiss` | web | /structure: Dismiss/Undo | `--undo` |  |
| `ssh structure install` | CLI-only by design | - | `--yes` (CLI) | Background service; the page shows the command. |
| `ssh structure list` | web | /structure | `--all`, `--json` |  |
| `ssh structure rules add` | web | /structure: Purpose rules | `--auto`, `--comment`, `--fingerprint`, `--kind`, `--name`, `--profile`, `--tool`, `--yes` |  |
| `ssh structure rules list` | web | /structure: Purpose rules | `--json` |  |
| `ssh structure rules remove` | web | /structure: Purpose rules | `--yes` |  |
| `ssh structure rules set` | web | /structure: Enable/disable auto | `--auto`, `--yes` |  |
| `ssh structure uninstall` | CLI-only by design | - | - | Background service. |
| `ssh tool list` | web | /tools: list | `--json` |  |
| `ssh tool show` | web | /tools?show=profile/key | - | Open a row, or pass `?show=`. |
| `ssh tool status` | web | /tools: Status | `--json` | Declared / present / missing counts and the missing list. |
| `ssh termius keys` | web | /termius: keys | `--clear` (CLI), `--clear-after` (CLI), `--include-git`, `--print` (CLI), `--skip` |  |
| `ssh validate` | web | /validate | - |  |
| `ssh view` | web | host page | - |  |
| `ssh vm add` | web | /hosts: Add a VM host | `--hypervisor`, `--key-name`, `--user`, `--vm`, `--yes` |  |
| `ssh vm adopt-key` | web | /vms: Adopt key | `--docker-context`, `--dry-run`, `--hypervisor`, `--key-name`, `--move`, `--user`, `--vm`, `--yes` |  |
| `ssh vm docker-context` | web | host page: Docker context | - |  |
| `ssh vm list` | web | /vms | - |  |
| `ssh vm sync` | web | /vms: Sync, schedule | `--dry-run`, `--install-schedule`, `--no-probe`, `--quiet` (CLI), `--remove-schedule`, `--repin`, `--yes` |  |
| `token apply` | web | /tokens: Apply (scope bar for --project/--config) | `--config`, `--creds-store-mode`, `--dry-run`, `--exports` (CLI), `--no-env-hint` (CLI), `--project`, `--source`, `--target` | --creds-store-mode: the config's _settings.docker_creds_store, which the web apply reads. |
| `token audit` | web | /tokens, overview (scope bar) | `--config`, `--project`, `--target` | --target: the page lists every tool. |
| `token copy` | CLI-only by design | - | `--project` (CLI) | Clipboard, never printed. |
| `token delete` | web | /tokens: Delete (scope bar) | `--project`, `--yes` |  |
| `token examples` | web | /tokens: examples | `--json`, `--project` (CLI), `--target` | Placeholders are not credentials: writing them into a project stays in the terminal. |
| `token import` | web | /tokens: Import | `--dry-run` |  |
| `token init` | web | /tokens: Init | `--target` |  |
| `token list` | web | /tokens (scope bar) | `--csv`, `--json`, `--project`, `--reveal` (CLI), `--target`, `--yes` | Secrets never render. |
| `token providers` | partial | /tokens: Set form service list | - | No list of git hosts/registries. |
| `token set` | web | /tokens: Add (secret stashed, never echoed; scope bar) | `--creds-store-mode`, `--project`, `--scope`, `--server`, `--service`, `--target`, `--username` |  |
| `token shell-aliases` | CLI-only by design | - | `--file` (CLI), `--install` (CLI), `--login` (CLI), `--no-backup` (CLI), `--print` (CLI), `--quiet` (CLI), `--uninstall` (CLI) | Edits shell rc files. |
| `token show` | CLI-only by design | - | `--json` (CLI), `--project` (CLI), `--reveal` (CLI), `--yes` (CLI) | Reveals a secret. |
| `token status` | web | /tokens (scope bar) | `--config`, `--json`, `--project` |  |
| `version` | web | /about | - |  |
| `vm add` | web | (same as ssh vm add) /hosts: Add a VM host | `--hypervisor`, `--key-name`, `--user`, `--vm`, `--yes` |  |
| `vm adopt-key` | web | (same as ssh vm adopt-key) /vms: Adopt key | `--docker-context`, `--dry-run`, `--hypervisor`, `--key-name`, `--move`, `--user`, `--vm`, `--yes` |  |
| `vm docker-context` | web | (same as ssh vm docker-context) host page: Docker context | - |  |
| `vm list` | web | (same as ssh vm list) /vms | - |  |
| `vm sync` | web | (same as ssh vm sync) /vms: Sync, schedule | `--dry-run`, `--install-schedule`, `--no-probe`, `--quiet` (CLI), `--remove-schedule`, `--repin`, `--yes` |  |
| `webhook add` | web | /webhooks: Add (PIN; secret shown once) | `--event` |  |
| `webhook deliver` | web | /webhooks: Deliver now | - |  |
| `webhook disable` | web | /webhooks: Disable | - |  |
| `webhook enable` | web | /webhooks: Enable | - |  |
| `webhook list` | web | /webhooks | `--json` |  |
| `webhook remove` | web | /webhooks: Remove (PIN) | - |  |
| `webhook test` | web | /webhooks: Test | - |  |

## Open gaps

None. Every command and flag is on the web, or CLI-only by design for the reason in its Notes. The web forms map some flags to a control of another name:

- `profile add/edit --shared`: the Key scope select.
- `rotate --keep-old`: Take the previous key off servers, unticked.
- `rotate --failed`: Choose the keys the last rotation left, on the rotate review.
- `host apply --no-pin`: the web reconcile always runs with `--no-pin`; host keys are pinned from /hosts.
- `host add --update`: Update it if the profile has it already; a preview of an alias the profile has also offers the update.
- `host edit --remove-alias`, `--unset-option`, `host variant set --unset-option`: take the name or option out of its field; the preview shows the flag.
- `ssh config keep-edits --dry-run`: the preview, which lists each block adopted; Make this change is `--yes`.
- `ssh config render --force`: Discard hand edits and render, on /ssh-config or on a refused render; the backup path is in the result.
- `host edit --remove-alias/--unset-option`: take the name or option out of the Routes fields on the Edit panel.
- `host add|edit` and `host variant set --service` / `--remove-service`: Services checkboxes (and Other services) on the host add, edit and variant panels; unchecking one is `--remove-service`.
- `ssh service edit`: the Edit a catalog service form on /services.
- `ssh tool show` / `ssh tool status`: a row on /tools (`?show=profile/key`) and the Status card.
- `knownhosts pin --all`: Pin host keys on /hosts (`knownhosts init`), which pins every host.
- `host delete`, `profile delete --revoke/--purge`: a job, linked from the plain delete.

## Web options the CLI lacks

None found. Bulk selection on /keys (keys, profiles and categories at once) is `--all` or `--category` in the CLI. The drag-and-drop tree on /structure writes the same plan lines as `ssh structure approve --plan`.
