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
| Commands (runnable, not hidden, not help-only parents) | 156 |
| Help-only parent commands left out | 25 |
| Web | 134 |
| Partial | 3 |
| CLI-only by design | 19 |
| Gap | 0 |
| Flags on those commands | 406 |
| Flags covered | 338 |
| Flags CLI-only by design | 68 |
| Flags with no web control | 0 |


## Matrix

| Command | Status | Web | Flags | Notes |
| --- | --- | --- | --- | --- |
| `(root)` | CLI-only by design | - | `--dev-root` (CLI), `--no-dotenv` (CLI) | Global flags; `serve --dev-root` runs the web UI in a sandbox. |
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
| `export pack` | web | /transfer: Export pack | `--dry-run`, `--include`, `--include-git`, `--out`, `--termius-groups` |  |
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
| `settings backup-key` | web | /settings: Backup key | - |  |
| `settings init` | web | /settings: Reset to defaults | `--force` |  |
| `settings path` | web | /settings, /about | `--json` |  |
| `settings set` | web | /settings | - |  |
| `settings show` | web | /settings | `--effective`, `--json` |  |
| `settings unset` | web | /settings | - |  |
| `settings validate` | web | /settings (checked as you type) | - |  |
| `setup export` | web | /setup: export (PIN-gated encrypt) | `--encrypt`, `--out`, `--passphrase-file` (CLI) |  |
| `setup import` | web | /setup: import (preview first) | `--allow-risky-options`, `--dry-run`, `--on-conflict`, `--passphrase-file` (CLI), `--yes` |  |
| `ssh audit` | web | /audit (CSV, Notify) | `--csv`, `--notify` |  |
| `ssh bundle` | web | /snapshots: Bundle | `--keep`, `--output` (CLI), `--recipient` | Written to the state home dist/. |
| `ssh clean` | web | /doctor: Clean | `--adopt`, `--dry-run`, `--yes` |  |
| `ssh config adopt` | web | /ssh-config: Adopt duplicate Host blocks | `--dry-run`, `--yes` | New in this branch. |
| `ssh config check` | web | /ssh-config | - |  |
| `ssh config export` | web | /ssh-config: download ssh_config | `--flat`, `--out` |  |
| `ssh config render` | web | /ssh-config, overview: Re-render | `--dry-run`, `--yes` |  |
| `ssh config show` | web | /ssh-config | - |  |
| `ssh deploy` | web | key page: Deploy (job) | `--yes` |  |
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
| `ssh host add` | web | /hosts: Add a host | `--env`, `--hostname`, `--key-name`, `--port`, `--provider`, `--proxy`, `--role`, `--tag`, `--token-env`, `--user` | Alias empty: generated from env and role. |
| `ssh host apply` | web | /hosts: Add or update in bulk | `--category`, `--create-profile`, `--dry-run`, `--file` (CLI), `--host`, `--no-pin`, `--reconcile`, `--tag`, `--yes` | The web reconcile is always --no-pin; pin from /hosts. |
| `ssh host delete` | web | host page: Delete; --revoke/--purge as a job | `--no-key-backup` (CLI), `--purge`, `--revoke`, `--yes` |  |
| `ssh host edit` | web | host page: Edit | `--hostname`, `--key-name`, `--port`, `--provider`, `--proxy`, `--tag`, `--token-env`, `--untag`, `--user` | --key-name/--provider/--token-env/--proxy added in this branch. |
| `ssh import` | web | /import | `--dry-run`, `--force`, `--yes` |  |
| `ssh key ack` | web | key page: Acknowledge; /keys: Acknowledge new keys | `--all`, `--category` |  |
| `ssh key add` | web | /keys: Add a key | `--host`, `--passphrase` (CLI), `--rotate-after-days`, `--type` | A passphrase stays in the terminal. |
| `ssh key adopt` | web | /structure: keep in place | `--host`, `--yes` |  |
| `ssh key delete` | web | /keys: delete, bulk Delete (job) | `--all`, `--category`, `--no-key-backup` (CLI), `--purge`, `--revoke`, `--yes` |  |
| `ssh key lifetime` | web | key page: Set lifetime | `--clear`, `--default`, `--yes` |  |
| `ssh key list` | web | /keys | `--csv`, `--json`, `--new`, `--since` |  |
| `ssh key regen-pub` | web | /keys, key page | `--all`, `--category`, `--dry-run`, `--yes` |  |
| `ssh key reserve` | web | /keys: Reserve | `--dry-run`, `--for`, `--note`, `--yes` |  |
| `ssh key revoke` | web | key page, bulk Revoke (job) | `--all`, `--category`, `--reason`, `--yes` |  |
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
| `ssh restore` | web | /transfer, /snapshots: Restore bundle (job) | `--allow-risky-options`, `--identity` (CLI), `--yes` |  |
| `ssh rollback` | web | key page: Roll back (job) | `--yes` |  |
| `ssh rotate` | web | key page / bulk Rotate (job) | `--all`, `--allow-unverified`, `--category`, `--check`, `--dry-run`, `--evict-old`, `--failed`, `--force`, `--keep-old`, `--no-key-backup`, `--passphrase` (CLI), `--yes` | --failed: the review link; --keep-old: Take the previous key off servers unticked. |
| `ssh show` | web | profile/host/key pages | - |  |
| `ssh snapshots list` | web | /snapshots | `--csv` |  |
| `ssh snapshots prune` | web | /snapshots: Prune | `--keep` |  |
| `ssh snapshots restore` | web | /snapshots/{id}: Restore | `--yes` |  |
| `ssh structure approve` | web | /structure: drag-and-drop, Approve | `--dry-run`, `--name`, `--plan`, `--profile`, `--yes` |  |
| `ssh structure check` | web | /structure (live watcher) | `--notify` (CLI), `--quiet` (CLI) |  |
| `ssh structure dismiss` | web | /structure: Dismiss/Undo | `--undo` |  |
| `ssh structure install` | CLI-only by design | - | `--yes` (CLI) | Background service; the page shows the command. |
| `ssh structure list` | web | /structure | `--all`, `--json` |  |
| `ssh structure uninstall` | CLI-only by design | - | - | Background service. |
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
- `knownhosts pin --all`: Pin host keys on /hosts (`knownhosts init`), which pins every host.
- `host delete`, `profile delete --revoke/--purge`: a job, linked from the plain delete.

## Web options the CLI lacks

None found. Bulk selection on /keys (keys, profiles and categories at once) is `--all` or `--category` in the CLI. The drag-and-drop tree on /structure writes the same plan lines as `ssh structure approve --plan`.
