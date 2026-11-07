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
web UI replaced the terminal menu, which was removed ([`tui`](tui.md)).

It prints an address like this:

```text
Authentication Access Manager web UI: http://127.0.0.1:7822/?token=…
home /Users/you/.config/authama (changes are previewed before they are made)
press Ctrl-C to stop
```

The token in the address signs the browser in, once. The browser swaps it for a session cookie and
drops it from the address bar. Without the token, every page answers 401.

If port 7822 is taken, `serve` uses the next free port and says which. If all twenty from 7822 up are
taken, it uses any free port the system offers. `--port` pins a port and fails when that port is
taken.

### One server per home

Launching `authama serve` again - or the app, or a bare `authama` - while a server is already
running for the same home opens that server in the browser instead of starting another, so servers
never pile up and fill the ports:

```sh
authama serve --status   # is one running for this home? where, since when, which pid
authama serve --stop     # stop it
authama serve --new      # start another beside it anyway
```

The running server leaves an owner-only `serve.json` in the state home's `.state/`, with its pid,
address and sign-in address. A launch trusts it only when the server at that address answers
`/healthz` with the same random instance id, so a stale record (a server that crashed) or a pid or
port reused by something else is ignored. `/healthz` is the one page that answers without signing
in, and it says nothing but `{"app":"authama","instance":"…"}`. A pinned `--port`, `--read-only`
when the running server is not, or `--allow-remote` starts a new server rather than reusing one.

**Why not a named address like `http://127.0.0.1/authama`?** A path does not free a port: two
programs still cannot listen on the same one, so a path only helps behind a reverse proxy that
owns the port, which a single-user tool should not need. A name like `http://authama.localhost/`
resolves in Chrome and Firefox but not through the macOS system resolver (Safari, `curl`, scripts),
so it is not used as the default.

### <a name="address"></a>Choosing the address

The default address is `http://127.0.0.1:7822/`. Three settings in `authama.json` change it, each
also an environment variable; `--addr` and `--port` on the command line win over them:

```json
{ "web": { "addr": "127.0.0.1", "port": 7900, "base_path": "/authama" } }
```

| Setting | Variable | What it does |
|---|---|---|
| `web.addr` | `AUTHAMA_WEB_ADDR` | The address to listen on. A non-loopback one still needs `--allow-remote`. |
| `web.port` | `AUTHAMA_WEB_PORT` | The port to try first. When it is taken the next free one is used, as with the default; only `--port` turns a taken port into an error. |
| `web.base_path` | `AUTHAMA_WEB_BASE_PATH` | A path to serve the UI under, such as `/authama`, so it can sit behind a reverse proxy beside other sites: `http://127.0.0.1:7900/authama/`. |

With a base path every link, form, asset, redirect and the session cookie carry it; `/` sends you to
it, and any other path outside it is not found. Letters, digits and `. _ ~ -` between slashes are
accepted. A running server is reused only when its base path is the one asked for.

## Pages

The layout is Tabler's, in two bars. The top bar holds the name (with a *read-only* badge when the
server only reads), a search box that opens the command palette, a theme toggle (light or dark), a
bell listing the newest [file events](events.md), and an account menu named for the system user the
server runs as. The account menu opens with *Running as <user> on <host>* and holds Settings, Events,
Doctor, the tour and About. Under it, a menu holds the Overview and five dropdowns: **SSH**
(Profiles, Naming, Hosts, Keys, VMs), **Config & backups** (SSH config, Snapshots), **Security** (Auth tokens,
Audit, Owner & PIN), **Activity** (History, Events), Export, and **System** (Doctor, Validate, Repair,
Settings, About). On a narrow screen it folds behind a button. About shows the build (and whether it is
current), this computer, and the third-party licences; following the system's light or dark theme is
chosen on the Settings page. The footer carries the copyright, the version, and links to About, the
documentation, the source and the licences.

- **Every control explains itself.** Hovering or focusing any button, menu entry or link styled as a
  button shows what it does and what to expect, for instance that a form only previews and nothing
  is written until you confirm. The text is also the control's accessible description, so a screen
  reader reads it. Menu entries describe their page, and the palette describes the highlighted entry.

- **Page headers** name the section, summarise the page, and hold its actions: the first two as
  buttons, any more (other forms, downloads) in one **More** menu.
- **Every form opens in a panel** that slides in from the right, from the page header or from a
  row's menu, which fills it in for that row: adding a profile, host, VM host, key or token, editing
  a profile's key scope, key comment or category, editing a host, adding hosts in bulk, minting
  keys, adopting a loose key, making a bundle and pruning snapshots. The token form starts with the
  tool and shows only the fields it uses.
- **Changes happen in a side panel.** Previewing a change, confirming it, a job's review and its
  live output, an operation in the history and a snapshot's diff all open in one panel that slides
  in over the page, so the page you came from stays where it was. Pages stack in the panel: **Back**
  returns to the previous one, and *Open as a page* opens the current one on its own. When a change
  was made in the panel, the page under it reloads as the panel closes. Without JavaScript each of
  these is its own page, as before.
- **A change says what it runs.** The review starts with the command it is the same as,
  highlighted, with a copy button and **Run**, which confirms the change (the same as *Make this
  change* at the foot of the review).
- **Diffs are numbered and highlighted.** Every file a change would write is a table of numbered
  old and new lines, coloured in the file's own syntax (JSON, INI, TOML, YAML, properties,
  ssh_config, netrc, shell), with its added and removed counts, a copy button for its path and, for
  JSON, whether the new content is valid; if it is not, the line and column of the problem.
- **Each row's actions are one menu** (the ⋮ button at the row's end): open it, copy what it names,
  see its history, and the row's destructive action last, set apart. A token's menu also opens the
  add-a-token panel filled in for it, which is how a token is replaced or rotated.
- **Connection status is checked on request.** *Check connections* on the Overview and Hosts pages
  (and *Check connection* on a host's page) runs `ssh net` and shows the result in the side panel.
  No page probes the network as it loads. See [network](network.md#in-the-web-ui).
- **Copying** puts public values on the clipboard: a public key, a fingerprint, a path, a hostname,
  `ssh <alias>`. A toast says it worked. Nothing secret can be copied from a page: a private key
  is copied only from the terminal (`authama ssh termius keys`), and a token's secret only with
  `authama token copy`.
- **`tokens.json` is editable** in a dialog on the Auth tokens page, previewed as a diff like any
  change. The editor checks the JSON as you type, names the line and column of a mistake, will not
  send the form while it is invalid, and re-indents valid JSON with *Format JSON*. The editor opens only a file that holds variable names, and refuses an edit that adds a
  value: every `env`, `user_env`, `pass_env`, `key_env` and `secret_env` must be an upper-case
  variable name of at most 48 characters that does not start like a known token (`ghp_`, `glpat-`,
  `npm_`, …), and a header that carries a credential must take it from `${VAR}`.
- **Every action says what it does** before you use it: hover or focus a page's actions, a form's
  Preview, a quick action or a destructive menu item, and a tip explains it, and that nothing is
  written before the preview is confirmed. Screen readers announce the same text.
- **Help** sits where it is needed: a hint under each form field, a ? beside a term that needs
  explaining (it opens a popover), and a tooltip on every icon button. **Take the tour**, in the
  account menu, walks the header and the overview one step at a time; it offers itself once, on a
  browser's first visit to the overview, and Escape ends it.
- **Every destructive action** (deleting a profile, host, key or token, pruning snapshots, restoring
  one, cleaning stale pins) is asked for first, in a dialog or in the panel it sits in, then
  previewed, then confirmed again on the preview.
- **Tables** sort by any column, and a list longer than ten rows gains a search box, a page size and
  pages. Hosts, keys, profiles, auth tokens, snapshots, key deployments (on the Audit page), history
  and events each download as CSV, with the same columns as `--csv` on their command. The History and
  Events downloads keep the page's filters, so the file holds what the page shows.
- **A page says whether its build is current.** About (System > About, or the account menu) shows
  the running build's version, marked *current* or *older build*, with when it was built and when
  the server started. The account menu also says which user and host the server runs as.
  When the `authama` binary is rebuilt, reinstalled or removed after `authama serve` started, every page shows a yellow banner naming the file, with **Restart
  now**: the server relaunches from the binary now installed, on the same address and with the
  same sign-in token, stops, and the page signs back in and reloads, then says in a green banner
  that it runs the current build. With `web.auto_restart` (the default) this happens by itself
  when the binary is rebuilt: open pages reload into the new build, or offer **Reload** when
  they hold unsaved input. `serve --restart` and `serve --stop-all` do the same from a
  terminal for every authama web UI on the machine
  ([Restart the web UI after updating](../installation.md#updating)).
- **Any list in any format.** `/exports/run` writes a selection of a home's records as a file.
  A built-in preset is a list as its page shows it (`?preset=hosts`, `keys`, `profiles`, `tokens`,
  `history`, `events`, `snapshots`, `deployments`); a selection (`?s=`, the JSON below as
  base64url) chooses the rows, filters, columns, sort and limit. `?format=` is `csv` (the default),
  `tsv`, `json`, `jsonl`, `yaml`, `php`, `js`, `markdown` or `html`; `?header=0`, `?compact=1` and
  `?shape=table` (`{columns, rows}` instead of one object per row) tune it. `?preview=1` answers with
  JSON: the first 25 rows, the rows left after each step, every field, and every value each filter
  can take, with its count.

  ```json
  {"version": 1, "grain": "host",
   "where": {"profile": ["simtabi"], "env": ["prd"], "exclude": "staging"},
   "columns": [{"field": "alias"}, {"field": "key.fingerprint", "as": "fingerprint"}],
   "sort": [{"field": "alias"}], "limit": 0}
  ```

  `grain` is `profile`, `key`, `host`, `token`, `op`, `event`, `snapshot` or `deployment`. `where`
  takes `profile`, `key` (a key's name or `profile/name`), `env`, `role`, `tag`, `status`, `target`,
  `kind`, `area`, `source`, `command` (a command or its subcommands), `since` and `until` (a time or
  a prefix such as `2026-11`), and `include`/`exclude` text. `narrow: true` cuts a profile's key
  list and counts down to the keys chosen. A selection of another `version` is refused.
- **The records as JSON.** `/api/dataset` serves the home's profiles, keys, hosts (every SSH setting,
  and the env, role, code and number their alias carries) and tokens (never a value);
  `?parts=tokens,ops,events,snapshots,deployments` chooses the slower parts, and `?parts=core` reads
  none of them.
- **A CSV is previewed before it downloads.** Its button opens a dialog with a tab per list on the left,
  each a table with a tick box per row:
  - **Profiles**, for a list with a profile column (hosts, keys, profiles, the Termius CSV).
  - **Keys**, for a list with a key column (hosts, keys): the keys of the profiles ticked, grouped under
    their profile when several are shown. Ticking a profile brings its keys in; unticking it takes them
    out.
  - **Keys in the profiles CSV.** It has one row per profile, with a `key_names` column listing the
    profile's keys, so it has a Keys tab too, counting the hosts each key is used by. Ticking keys
    keeps the profiles holding one. *Count only the ticked keys* (on by default) then cuts each
    profile's `keys`, `hosts` and `key_names` down to the ticked keys; off, a profile is kept with its
    whole counts. The query is `?key=…&narrow=1`.
  - A list with a profile column but no key column (the Termius CSV) has no Keys tab, and says so,
    pointing at the lists that do.
  - **Columns**, each with its value in the first row, so a cryptic name says what it holds.

  Everything is ticked to start with, which means everything, including profiles and keys added later.
  The box in a table's header ticks all, none or (half-ticked) some of the rows the search shows; each
  tab counts what is ticked (`1/4`). Ticking nothing in a list disables *Download* and says why. A
  row count shows only when some row has more than one, and a list longer than eight gets a search box.

  On the right, a box leaves out rows containing some text, and the first 25 rows show as the file will
  hold them. The summary counts rows and columns (`1 of 9 rows · 7 of 7 columns`), with a badge per
  reason rows are left out (`4 in other profiles`, `4 for other keys`, `2 containing "staging"`), whose
  × undoes that choice. The choice is remembered for that list in the browser, and *Reset* forgets it.
  The server does the shaping, so a script can too: `?profile=simtabi,adelsaiq` and
  `?key=simtabi_simbi-prd-web-00-ed25519` keep those rows (even with that column dropped),
  `?drop=user,port` leaves columns out, `?exclude=staging` leaves out rows containing that text, and
  `?preview=1` answers with the preview as JSON, including every profile and key with its row count and
  the rows left after each filter.
- **The command palette**, Ctrl+K (⌘K on a Mac), `/`, or the search box in the header, jumps to
  any page, opens the add-a-profile, host or token panel, or starts a download, as you type. It
  groups what matches under Pages, Actions and Downloads, shows each entry's shortcut, and says so
  when nothing matches.
- **Keyboard shortcuts.** `?` lists them all. `g` then a letter goes to a page (`g h` Hosts, `g k`
  Keys, `g t` Auth tokens, `g ,` Settings, …) and `n` then a letter opens a form (`n p` profile,
  `n h` host, `n t` token). They never fire while you type in a field or while a dialog or panel is
  open, and the switch in the `?` sheet turns the single-key ones off for that browser; Ctrl+K
  always works.

| Page | Shows |
|---|---|
| Overview | the state rows (`overview.Rows`), each problem with the command that fixes it; counts; key health, auth tokens and snapshots; quick actions; hosts by profile; the newest file events; keys due for rotation; recent changes |
| Profiles | each profile's category, key scope, key store, and host and key counts |
| Hosts | every host by profile. A host's page shows its manifest entry, its deployments, the block rendered into `~/.ssh/config`, and the host keys pinned for it |
| Keys | every key with its fingerprint and expiry, and the reserved keys (loose keys kept on purpose). A key's page shows its public key and fingerprint, each with a copy button, where the key is deployed and every operation that touched it. A `.pub` that is not a single public key line is not shown |
| Snapshots | every snapshot. A snapshot's page diffs it against `~/.ssh` now or against another snapshot |
| History | the [`history`](history.md), filtered the same way. An operation's page shows its events and the manifest before and after |
| VMs | the VM hosts and their VMs' state, the pending `ssh vm sync` plan, the enabled hypervisors and their VMs, the declared devboxes (planned ones without a VM yet), the reserved keys, the loose keys that could be adopted, and whether the daily sync is scheduled |
| Auth tokens | the installed auth tokens (tool, host, user, whether a secret is set, file - never the secret), each tool's file with its mode and how many entries are installed, configured and resolvable, what `token apply` would skip, and the token audit, as `authama token list`, `token status` and `token audit` print |
| SSH config | every file the manifest renders into `~/.ssh` and whether the files on disk match it, as `authama ssh config show` and `ssh config check` print |
| Doctor, Audit | the reports `authama doctor` and `authama ssh audit` print |
| Validate | every validate check on one page, run when you open it: the manifest loads and passes its rules, `authama settings validate`, `authama ssh validate` (each keypair parses, matches and has safe permissions) and `authama ssh config check`. A manifest that does not load fails the two checks that need it rather than reporting them clean. Reads only |
| Events | every change seen to a tracked file, with the authama operation that made it, as [`authama events`](events.md) lists them; filtered by area |
| About | this build (version, commit, Go, platform), the user and machine it runs as, this computer (OS and version, build, kernel, architecture, model, processor, cores, memory, uptime, read from the OS without running a command), where it keeps things, links to the documentation and the source, and the third-party licences of the bundled front-end assets |
| Owner & PIN | whether the home is [bound to its owner](owner.md), where the sealing key is kept, whether a PIN and fingerprint unlock are on, and whether the sealed files match, naming each one changed outside authama. Changing any of it is done at a terminal, which proves the OS account; the page shows the commands |
| Repair | what is wrong with the home, `~/.ssh` and the token files, the fixes made as one previewed change, and a factory reset; see [repair](repair.md) |
| Settings | the colour theme (light, dark, or following the system); where the config home, state home, `.env` and `authama.json` are, every setting's value in effect and where it came from, and what `authama settings validate` reports, as `settings path`, `settings show --effective` and `settings validate` print them. The values are those the server started with |
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
| Change the naming rules, for every profile or one | Naming | `authama ssh naming set [--profile] …` (only the fields changed) |
| Change how new keys are named | Naming | `authama ssh naming key-template <template> --yes` |
| Set or clear a profile's code | Naming (a profile's menu) | `authama ssh profile edit <name> --code` |
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
| Rebuild one key's `.pub`, a profile's, or every key's | Keys (row menu, or *Rebuild public keys*), a key's page | `authama ssh key regen-pub <profile/key \| profile \| --all> --yes` |
| Import this machine's ssh config, or the config home's | Profiles → *Import an ssh config* | `authama ssh import <config> [--force] --yes` |
| Move the key root | Keys → *Move the key root* (the new name typed twice) | `authama ssh key root <dir> --yes` |
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
and the Termius CSV (`authama export hosts --format termius`, with or without git hosts) as downloads. Nothing is
written to the home, so `--read-only` keeps them.

## Jobs

Deploying, rotating and rolling back reach the hosts that use a key, and pinning host keys asks
hosts for theirs, so no copy can preview them. Each runs as a **job**:

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
| Pin one host's keys, after you have seen their fingerprints | the host's page | `authama ssh knownhosts pin <alias>`, answered yes |
| Pin a profile's hosts, or every host (trust on first use) | Hosts → Pin host keys | `authama ssh knownhosts init <profile> --yes`, `authama ssh knownhosts init --all --yes` |

- **Not in dev mode.** Deploy, rotate and rollback reach real servers, so under `--dev-root` the key
  and profile pages say so instead of offering them, as the CLI refuses them. A bundle restore is
  local, and pinning host keys only reads them and writes the sandbox's own `known_hosts`, so both
  are offered.
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
- **Rebuilding a `.pub` reads the private key where it is.** A preview's copy holds no private
  key, so the rebuild derives each public key from the real one, as `ssh-keygen -y` does for the
  command: it reads the key and copies nothing of it. The preview shows the `.pub` it would really
  write. A key with a passphrase is skipped with the command to derive it by hand, as in the
  terminal; nothing asks for a passphrase.
- **Import** reads only the configs this machine has: the managed `~/.ssh/config`, and a config
  home's `ssh/config`. The form names one; it never takes a path, so another machine's copy is
  imported from the terminal. A manifest with profiles is replaced only when you tick *Replace*,
  as the terminal needs `--force`, and it is backed up first. A key the import would copy in from
  outside the managed tree is listed in the preview, which copies none.
- **Cancel** stops a job at its next step, never inside one. A rotate waiting on you is answered no.

Some changes are terminal-only:

- rotating with any option but `--allow-unverified`, overwriting a key (`keygen --force`), and
  passphrases;
- revoking a key from its targets (`--revoke`), and restoring a bundle from anywhere but `dist/`, or
  with an identity that has a passphrase;
- `--no-key-backup`;
- importing from any folder but `~/.ssh` and the config home.

`--read-only` serves none of the changes in the table above, and the pages show no forms.

## What it will not do

- **Hold the lock while you read.** It holds the lock only while it checks and makes a change you
  confirmed.
- **Open a snapshot that holds private keys.** A snapshot written before private keys were left out
  is listed but never opened. `authama ssh snapshots prune --keep 0` removes it.
- **Show private keys.** Pages read public keys, `known_hosts` and config only.
- **Create the history from a page view.** A page reads `log/history.db` if it exists. Only a change
  creates the database or imports into it, and only while it holds the lock.

## Security

- **PIN.** When the home has an owner PIN (see [owner binding](owner.md)), the sign-in link opens an
  unlock page first, and no other page opens until the PIN is given. Every change and job confirmation
  asks for it again once five minutes have passed. Wrong PINs share the command line's lockout.
- **Bounded requests.** A request body over 1 MiB is refused with 413 before it is parsed; headers
  are held to 64 KiB, a body must arrive within 30 seconds, and `Permissions-Policy` turns off every
  device API (camera, microphone, location, USB and the rest).
- **CSV downloads** never hold a secret, and a cell that a spreadsheet would read as a formula (one
  starting `=`, `+`, `-` or `@`) is written with a leading apostrophe.
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
