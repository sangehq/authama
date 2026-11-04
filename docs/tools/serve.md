# `serve`

`authama serve` opens a web UI for one authama home in a browser on this machine. It shows everything,
and makes the everyday manifest and auth-token changes, each previewed before it is made.

## Starting it

```sh
authama                       # on a terminal: the same as `authama serve`
authama serve                 # 127.0.0.1:7822, opens a browser
authama serve --no-open       # print the address only
authama --dev-root ./scratch serve
```

A bare `authama` on a terminal runs `serve` with every flag at its default; the
global `--dev-root` and `--no-dotenv` still apply. With no terminal (a pipe, a
script, CI) a bare run prints help instead, since `serve` runs until Ctrl-C. The
menu is `authama tui`.

It prints an address like this:

```text
Authentication Access Manager web UI: http://127.0.0.1:7822/?token=…
home /Users/you/.config/authama (changes are previewed before they are made)
press Ctrl-C to stop
```

The token in the address signs the browser in, once. The browser swaps it for a session cookie and
drops it from the address bar. Without the token, every page answers 401.

If port 7822 is taken, `serve` uses the next free port and says which. `--port` pins a port and fails
when that port is taken.

## Pages

The layout is Tabler's. The header holds the name, a theme toggle (light or dark), a bell listing the
newest [file events](events.md), and a menu with Settings, Events, Doctor, *Follow the system theme*,
About and the licences; it also says when the server is in dev mode or read-only. Under it, a menu
holds the Overview and three dropdowns: **SSH** (Profiles, Hosts, SSH config, Keys, VMs, Snapshots),
**Auth** (Auth tokens) and **System** (History, Events, Doctor, Audit, Settings, About). On a narrow
screen it folds behind a button.

- **Page headers** name the section, summarise the page, and hold its main actions.
- **Adding** a profile, a host or a token opens its form in a panel that slides in from the right.
- **Every destructive action** (deleting a profile, host, key or token, pruning snapshots, restoring
  one, cleaning stale pins) asks in a dialog first, then previews, then asks again on the preview.
- **Tables** sort by any column, and a list longer than ten rows gains a search box, a page size and
  pages.

| Page | Shows |
|---|---|
| Overview | the rows the `authama tui` menu prints at the top, each problem with the command that fixes it; counts; key health, auth tokens and snapshots; quick actions; hosts by profile; the newest file events; keys due for rotation; recent changes |
| Profiles | each profile's category, key scope, key store, and host and key counts |
| Hosts | every host by profile. A host's page shows its manifest entry, its deployments, the block rendered into `~/.ssh/config`, and the host keys pinned for it |
| Keys | every key with its fingerprint and expiry, and the reserved keys (loose keys kept on purpose). A key's page shows where the key is deployed and every operation that touched it |
| Snapshots | every snapshot. A snapshot's page diffs it against `~/.ssh` now or against another snapshot |
| History | the [`history`](history.md), filtered the same way. An operation's page shows its events and the manifest before and after |
| VMs | the VM hosts and their VMs' state, the pending `ssh vm sync` plan, the enabled hypervisors and their VMs, the declared devboxes (planned ones without a VM yet), the reserved keys, the loose keys that could be adopted, and whether the daily sync is scheduled |
| Auth tokens | the installed auth tokens (tool, host, user, whether a secret is set, file - never the secret), each tool's file with its mode and how many entries are installed, configured and resolvable, what `token apply` would skip, and the token audit, as `authama token list`, `token status` and `token audit` print |
| SSH config | every file the manifest renders into `~/.ssh` and whether the files on disk match it, as `authama ssh config show` and `ssh config check` print |
| Doctor, Audit | the reports `authama doctor` and `authama ssh audit` print |
| Events | every change seen to a tracked file, with the authama operation that made it, as [`authama events`](events.md) lists them; filtered by area |
| About | this build (version, commit, Go, platform), where it keeps things, and links to the documentation, the source and the licences |
| Settings | where the config home, state home, `.env` and `authama.json` are, every setting's value in effect and where it came from, and what `authama settings validate` reports, as `settings path`, `settings show --effective` and `settings validate` print them. The values are those the server started with |
| Third-party licences | the licences of the web UI's bundled front-end assets (Tabler, Tabler Icons, Bootstrap, Popper), at `/about/licenses` |

## Changes

These changes can be made from the pages:

| Change | Where | The same as |
|---|---|---|
| Add a profile, optionally in a category and with a key comment | Profiles | `authama ssh profile add [--category] [--key-comment]` |
| Change a profile's key comment | Profiles | `authama ssh profile edit --key-comment` |
| Change a profile's key scope | Profiles | `authama ssh profile edit --key-scope --key-name` |
| Delete a profile | Profiles | `authama ssh profile delete --yes` |
| Delete a profile and its key files | Profiles, ticking *and its key files* | `authama ssh profile delete --purge --yes` |
| Move a profile to a category | Profiles | `authama ssh profile edit --category` |
| Add a host, or a VM host | Hosts | `authama ssh host add`, `authama ssh vm add` |
| Add or update hosts from specs | Hosts | `authama ssh host apply --host … --yes` |
| Create a VM host's Docker context | the host's page | `authama ssh vm docker-context` |
| Sync the VM hosts with their VMs (optionally replacing changed pins) | VMs | `authama ssh vm sync --yes` (`--repin`) |
| Adopt a loose key as a VM host's key | VMs | `authama ssh vm adopt-key <key> <profile> <alias> --hypervisor … --vm … --user … --yes` |
| Install or remove the daily VM sync | VMs | `authama ssh vm sync --install-schedule` / `--remove-schedule` |
| Edit a host's hostname, user, port or tags | the host's page | `authama ssh host edit` |
| Delete a host and its pins | the host's page | `authama ssh host delete --yes` |
| Delete a declared key that no host uses | Keys | `authama ssh key delete --yes` |
| Reserve a loose key (keep it on purpose) | Keys | `authama ssh key reserve <key> --for … --note … --yes` |
| Drop a key's reservation | Keys | `authama ssh key unreserve <key> --yes` |
| Re-render `~/.ssh/config` | Overview, SSH config | `authama ssh config render --yes` |
| Reconcile: mint missing keys, record keys on disk, render | Overview | `authama ssh reconcile --yes --no-pin` |
| Mint a profile's or host's missing keys | Keys | `authama ssh keygen <target> --no-pin --yes` |
| Add a key, optionally wired to a host | Keys | `authama ssh key add` |
| Prune old snapshots | Snapshots | `authama ssh snapshots prune --keep` |
| Restore a snapshot | the snapshot's page | `authama ssh snapshots restore <id> --yes` |
| Fix file modes | Doctor | `authama doctor --fix` |
| Write an editable `providers.json` | Doctor | `authama ssh providers --export` |
| Create a starter `authama.json` | Settings, when there is none | `authama settings init` |
| Remove stale pins and records, optionally adopting pins first | Doctor | `authama ssh clean --yes [--adopt]` |
| Write an encrypted bundle | Snapshots | `authama ssh bundle [--keep]` |
| Send the desktop alert, if one is due | Audit | `authama ssh audit --notify` |
| Send a test notification | Audit | `authama ssh notify test` |
| Remove an installed token | Tokens | `authama token delete <target> <host>` |
| Write the token config's credential files | Auth tokens | `authama token apply` (a Docker registry in a credential store follows `_settings.docker_creds_store`, and is refused when it is unset, since a page cannot prompt) |
| Add or replace one token | Auth tokens | `authama token set --service\|--target --server [--username] [--scope] [--creds-store-mode]` |
| Write a starter `tokens.json` | Auth tokens, when there is none | `authama token init [--target …]` |
| Import an auth-token-manager config | Auth tokens, when the config in use is at its location | `authama token import` |

The token apply resolves secrets from the environment `authama serve` was started
in, so a variable set in another terminal is not seen.

**Adding a token is the one form that takes a secret.** The secret goes to the server with the
preview, and straight into memory: the preview page and its confirmation carry a random reference
in its place, never the secret. The reference works once, for the browser session that made it, for
15 minutes, and is forgotten when the change is made; nothing writes it to disk but the tool's own
credential file. The preview names the file and host it writes, and says the secret is not shown. A
secret posted with any other form is refused. Revealing a stored token and copying one stay in the
terminal (`token list --reveal`, `token copy`): a page never shows a secret.

Each change takes two steps:

1. **Preview.** The change runs for real against a private, throwaway copy of the home. The copy
   holds the config files (`manifest.json`, `inventory.json`, `providers.json` and `authama.json`; never
   `.env` or the age identity), plus the files from `~/.ssh` that a snapshot may hold: config,
   `known_hosts` and public keys. No private key is copied. The page shows the command line it
   stands for, what authama would say, and a diff of every file the change would write.
2. **Confirm.** authama takes the lock, then previews the change again. If the new preview differs
   from the one you saw, the change isn't made: someone else may have changed the home since
   (a CLI command, the menu, another tab). The page shows the new preview to confirm instead.
   Otherwise authama ssh snapshots `~/.ssh`, makes the change, releases the lock, and links to the
   change in the [`history`](history.md), where it shows with source `web`.

The lock is held only while a change is checked and made, never while you read a preview.

**Where the copy has no file, a placeholder stands in.** A private key's place in the copy holds an
empty placeholder file. It tells a command the key exists, so the key is never minted again, but it
holds nothing of the key. Copied files keep their modification times.

**Changes that mint a key.** A preview of `reconcile`, `keygen` or `key add` mints throwaway keys in
the copy, and the page says so. Confirming mints new keys, so their fingerprints differ from the
preview's; everything else comes out as shown. These changes never overwrite a key that exists,
never take a passphrase, and never pin hosts, which needs the network. They're refused for a profile
kept in 1Password: a preview mints for real, and in a vault that would leave an item behind.

**Files the copy cannot show.** A purge deletes private keys, and a category move carries them to a
new directory. The copy holds them only as placeholders, so the preview lists each one the change
would remove or create, by path, without its contents. That list is bound to the confirmation too.

**Deleting a profile's key files** always writes an encrypted backup of `~/.ssh` and the config
first, as the CLI does, so it needs `AUTHAMA_AGE_RECIPIENT` set where `authama serve` runs, and
`age` installed. There is no `--no-key-backup` on the web.

**Changes with effects outside the copy.** Prune and fix list what they would do outside the copied
files: the snapshots they would remove, the paths whose modes they would fix. A bundle, a Docker
context and the notifications act outside the home altogether, so a preview never runs that part;
it lists what confirming will do, and only confirming does it. The confirmation is bound to those
lists too. Prune, bundle, the Docker context and the notifications take no snapshot first, as their
commands take none.

**Restoring a snapshot** previews exactly what it puts back: the copy is restored from the
snapshot, and the page shows the diff against `~/.ssh` now. The confirmation names the snapshot
file. Confirming snapshots `~/.ssh` first, then restores. A snapshot that still holds private keys
is refused here, as its page refuses to open it.

**Exports download.** The hosts page offers the flat `ssh_config` (`authama ssh config export --flat`)
and the Termius CSV (`authama ssh termius export`, with or without git hosts) as downloads. Nothing is
written to the home, so `--read-only` keeps them.

## Jobs

Deploying, rotating and rolling back reach the hosts that use a key, so no copy can preview them.
A key's page offers each as a **job**:

1. **Review.** The page says what the job will do, and the command it is the same as.
2. **Run.** The job starts, and its page shows the output as it comes. It holds the lock until it
   ends, and it is recorded in the history like any other change.

| Job | Where | The same as |
|---|---|---|
| Deploy a key to every host that uses it | the key's page | `authama ssh deploy <profile/key> --yes` |
| Rotate a key | the key's page | `authama ssh rotate <profile/key> --yes` |
| Roll a key back to its predecessor | the key's page | `authama ssh rollback <profile/key> --yes` |
| Rotate a profile's keys, or every key | Profiles | `authama ssh rotate <profile> --yes`, `authama ssh rotate --all --yes` |
| Restore an encrypted bundle from `dist/` | Snapshots | `authama ssh restore <bundle> --yes` |

- **Not in dev mode.** Deploy, rotate and rollback reach real servers, so under `--dev-root` the key
  and profile pages say so instead of offering them, as the CLI refuses them. A bundle restore is
  local, and is offered.
- **One job at a time.** Starting a second while one runs takes you to the one running. No change
  is previewed or made while a job runs.
- **Nothing a job runs can prompt.** `ssh` and `ssh-copy-id` run with stdin closed and
  `BatchMode=yes`, so a target that needs a password fails at once, with a note to deploy it from a
  terminal. A terminal is where the CLI still asks.
- **Rotate asks you to paste a key.** A target that takes the key by hand, such as a panel, pauses
  the job with the new public key shown for copying. Confirm once it is pasted; authama then logs in
  with it to prove it. Rotate writes an encrypted backup first, so it needs
  `AUTHAMA_AGE_RECIPIENT`.
- **Rotating many keys** reviews the keys it will rotate, then rotates each a host uses in turn. A
  key that fails is left as it was, and the rest go on. Cancel stops before the next key.
- **Restoring a bundle** decrypts with the identity file in `AUTHAMA_AGE_IDENTITY_FILE`, as set
  where `authama serve` runs; the web takes no identity and no passphrase. Only a bundle in the state
  home's `dist/`, named as `authama ssh bundle` names them, can be restored here.
- **Cancel** stops a job at its next step, never inside one. A rotate waiting on you is answered no.

Some changes are terminal-only:

- rotating with any option but `--allow-unverified`, overwriting a key (`keygen --force`), and
  passphrases;
- revoking a key from its targets (`--revoke`), and restoring a bundle from anywhere but `dist/`, or
  with an identity that has a passphrase;
- pinning host keys (`knownhosts init`, `knownhosts pin`): a scan reaches every host, and trusting
  a key is a decision made per key;
- `key regen-pub`: it needs the private keys, which a preview never copies, and may ask for a
  passphrase;
- `--no-key-backup`.

`--read-only` serves none of the changes in the table above, and the pages show no forms.

## What it will not do

- **Hold the lock while you read.** It holds the lock only while it checks and makes a change you
  confirmed. The `authama tui` menu now does the same.
- **Open a snapshot that holds private keys.** A snapshot written before private keys were left out
  is listed but never opened. `authama ssh snapshots prune --keep 0` removes it.
- **Show private keys.** Pages read public keys, `known_hosts` and config only.
- **Create the history from a page view.** A page reads `log/history.db` if it exists. Only a change
  creates the database or imports into it, and only while it holds the lock.

## Security

- **Loopback by default.** A non-loopback `--addr` is refused unless you pass `--allow-remote`, and
  even then it serves plain HTTP. An SSH tunnel is better:
  `ssh -L 7822:127.0.0.1:7822 <host>`.
- **Host check.** The server answers only to `127.0.0.1`, `localhost` and `::1` at its port, plus
  any `--allow-host`. That stops a page on another site from rebinding its name to this machine.
- **Session cookie.** `HttpOnly` and `SameSite=Strict`. Every change is also held to the page's own
  origin and a per-session form token.
- **Forms take names, never paths.** Profile, host and key names must be letters, digits, `.`, `_`
  and `-`. A hostname or user with a control character in it is refused, so a form cannot smuggle a
  line into `~/.ssh/config`.
- **Headers.** Every response sends a strict Content-Security-Policy with no inline script or style,
  plus `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin` (no referrer leaves for another
  site) and same-origin COOP and CORP.
- **No network.** The page loads nothing from the network: Tabler, its icons and the page's own script
  are embedded in the binary.

`--token-file <path>` also writes the sign-in address to a file with mode 0600, for scripts.

## Flags

| Flag | Meaning |
|---|---|
| `--addr` | host, or host:port, to listen on (a port here pins it) |
| `--port` | port; pinned when given |
| `--allow-remote` | permit a non-loopback `--addr` |
| `--allow-host` | an extra Host header to accept, such as a proxy's name (repeatable) |
| `--read-only` | serve no route that changes anything, and show no form |
| `--no-open` | do not open a browser |
| `--token-file` | also write the sign-in address to this file |

## Updating Tabler

The embedded Tabler version is pinned in `src/internal/web/assets/vendor/TABLER_VERSION`. Three
targets manage it:

- `make tabler-check` reports a newer release.
- `make tabler-update` moves to the newest release.
- `make tabler-vendor` re-vendors the pinned version.

Each target checks every download against npm's integrity hash, then rewrites the files, the
licences, `MANIFEST.sha256` and `third-party-notices.md` together.

---

[← Docs index](../../README.md#documentation)
