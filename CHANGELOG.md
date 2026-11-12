# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Force a rotation.** `authama ssh rotate --force` (and *Force: replace what cannot rotate in
  place* on Rotate keys and on a rotation's review) replaces, on this computer only, each key that
  cannot rotate in place: one never deployed, or whose host cannot be reached. Every other key
  rotates as before. A replaced key keeps its predecessor under `old/` and has to be deployed next,
  since until then a host that trusted the old key cannot be logged in to with it; so a forced
  review is titled *(forced)*, warns of the lockout, starts from a red button, asks in a red dialog
  and names the rollback from `old/` as its undo. A dry run is never forced.

- **The job console draws a tree.** A key's line keeps its coloured badge; each host under it hangs
  off a `├─` / `└─` branch with a coloured ✓, ✗ or ! for how it went, and each group gets a little
  air. The dry run says why a key would fail: a key that answers but does not log in was never
  deployed there (deploy it first), or was removed there since (deploy it again).
- **A job's output reads like a console.** Each line's status word (ok, FAIL, paste, skip,
  WARNING) sits in a fixed column as a coloured badge, the host or key it is about is in its own
  colour, nested lines are indented under theirs, and failures are tinted; it follows the newest
  line unless you scroll up, and a copied line reads as it was printed. A job page is titled for
  the job (*Dry run: rotate 10 keys*) with its full command beneath, the Output card counts what
  passed and failed, and a finished job says *Finished: 7 passed, 3 failed* rather than *Finished*.
  `rotate --check` prints its status words in an aligned column too, and a target that takes keys
  by hand reads `paste`.
- **Test a rotation before running it.** `authama ssh rotate --check` (and **Dry run: test every key**
  on a rotation's review) checks every key and changes nothing: each server it is used on answers and
  the current key still logs in, and each target that takes keys by hand is named. A key that would
  not rotate cleanly is marked failed, with why; the job page shows each key as it is tested.
- **A clean rotation, without the backup.** *Skip the encrypted backup* on Rotate keys, or *Rotate
  without a backup* on a review blocked by a missing recipient, rotates with `--no-key-backup`:
  no bundle is written, and each replaced key is still kept under `old/` for a rollback until the
  next rotation.
- **A fix takes you back to the job.** After *Create a backup key* (or another fix) started from a
  job's review, the done page offers **Review the job again**, with its checks run again, instead
  of leaving the old review, which still showed the check failing, behind the panel's back arrow.
- **Edit settings from the Settings page.** Every setting sits in a card for its section with its
  description, its value, and where that came from; **Edit** opens a form whose input fits it (a
  switch-like On/Off, a menu of its choices, a number, or text with the default as a hint), previewed
  before it is written into `authama.json`, and **Go back to the default** takes it out again. The
  server uses a saved value at once, unless the environment or `.env` sets it, which still wins. A
  filter narrows the page. The same from a terminal: `authama settings set <key> <value>` and
  `authama settings unset <key>`.
- **A job's failed check says how to fix it, and offers the fix.** A rotate with no backup recipient
  offers **Create a backup key** (`authama settings backup-key`: an age key made with `age-keygen`,
  its identity kept owner-only where `paths.age_identity_file` already points (else in the config
  home), and `backup.age_recipient` and
  `paths.age_identity_file` set to it, never shown in a page) or **Use a key you have**, which opens
  that setting in Settings. A job that backs up first also checks that `age` is installed.
- **Jobs have guardrails, and run one item at a time on screen.** A job's review is titled for
  the job (*Rotate 10 keys*), checks what it needs before it may start (no other job running, a
  backup recipient for one that backs up first) and keeps Start off until they pass, numbers its
  steps, and lists the keys it works through. Start asks once more in a dialog, red for a job that
  cannot be undone. The job page lists each key as waiting, running, done or failed as it goes, and
  a job waiting for you says which step it is on, with the text to copy.
- **Buttons say what they do, not the command.** The command a change or job is the same as is shown
  for reference with a copy button; the Run button beside it is gone, and a destructive change
  confirms in a dialog instead of a button reading *Yes, authama …*.
- **A clearer token audit.** The Auth tokens page's Audit card counts what to fix, what is
  owner-only and what is not set up; a credential file others can read has *Make it owner-only*, a
  `.gitignore` missing secret files offers the lines to copy, files read as `0600` rather than
  `0o600`, Composer's note sits with its file, and the tools with no file fold into a list that opens
  *Add a token* for each.
- **A checklist for every key and token.** `authama checklist` and **Security > Checklists**
  list, for each key and token, every place it is used and every step to rotate or revoke it
  completely. A key's follows its life: where it is while in use, and what still has to go once
  it is revoked or a rotation retires it (targets it could not be removed from, hosts still
  pointing at it, the private key, ssh-agent, encrypted bundles, old snapshots). A token's
  revocation checklist starts with `authama checklist revoking`, and links to its issuer's
  tokens page. Steps authama cannot see are ticked by hand and kept in `.state/checklists.json`.
  Each key and token row has a **Checklist** entry, and each key page a summary card.
- **Choose which keys to rotate, revoke or delete.** `ssh key revoke` and `ssh key delete`
  take several keys, profiles or host aliases, `--category` or `--all`, as `ssh rotate` does.
  The Keys page's **Rotate keys**, **Revoke keys** and **Delete keys** open a picker of every
  key, whole categories and profiles, and single keys, showing why a key cannot be chosen.
- **A redesigned key picker.** Rotate, Revoke and Delete keys open a picker that puts *every key*
  in a card of its own and hides the rest while it is ticked, shows categories as chips with their
  key counts, lists each key with its type, hosts and expiry, counts what is chosen in each profile
  (a profile's box shows a dash while only some of its keys are), filters a long list by key,
  profile or host, and clears in one click. The button names what it will act on, *Review 3 keys
  to rotate*, and each form lists what happens, in order, with its options as switches.
- **Progress, with the time left.** A rotate, revoke or delete of several keys shows a bar on its
  job page with the key under way, *3 of 10 · 30%*, and the time left at the pace so far; every
  job shows a spinner and the time it has run. Any preview, change or side-panel page that takes
  more than two seconds shows a card with the time so far, and, once the browser has timed that
  action before, a percentage and the time left.
- **An Actions menu** beside the Overview opens Rotate keys, Revoke keys, Delete keys, Mint missing
  keys, Pin host keys, Add a host and Add a token from any page, and the tour points it out.
- **A Reset page** (System → Reset, and Actions → Reset…) lists every way to start something again,
  one row each, grouped by how much each resets: rotate every key, revoke keys, settings to their
  defaults, initialise the home again (manifest, inventory and `.env`, backed up first, after typing
  `init`), old snapshots, the history, unbind this home, and factory reset. Each row says whether it
  can be undone, and its details what it resets, keeps, how to undo it and the command.
- **Start the history again.** `authama history reset` (and *Reset → Start the history again*, after
  typing `history`) moves `history.db` and `audit.log` aside, kept as `<name>.reset-<time>`, so the
  next change starts a new history. A new setting, `history.permanent`
  (`AUTHAMA_HISTORY_PERMANENT`), keeps the history for good: the command refuses and the page offers
  no reset.
- **The main menu is grouped by what each page is about.** Actions is a menu like the others rather
  than an outlined button and ends with Reset; SSH holds the rendered SSH config beside the hosts it
  comes from; Auth tokens is an entry of its own; Snapshots and Export sit together under Backups &
  export; System gains Reset. The command palette follows the same groups and gains Reset, Rotate
  keys and Revoke keys.
- **Pin host keys counts its hosts**, so its job page shows a percentage and the time left too.
- **A page's More menu says what each entry does** under its name, and Keys and Hosts put Rotate
  keys and Pin host keys beside their first action instead of in the menu.
- **Get started.** A new home's Overview lists the steps to set it up, each with a button that
  opens its form, ticked as they are done, until every one is.
- **Notifications say authama, with its logo, and stay quiet in tests.** Desktop notifications now go
  through go-notifykit: on macOS a banner shows `authama` and the web UI's logo mark instead of
  "Script Editor", Linux's `notify-send` gets the same icon, and nothing is shown under `go test`
  or in CI. `AUTHAMA_NOTIFICATIONS=off` still turns them off. New verbs `ssh notify send`,
  `ssh notify ping` and `ssh notify doctor` show one, hand one to the running web UI, and say why
  notifications do or do not work; `authama serve` listens for pings on a socket only the user can
  open.

- **Docker devboxes say whether Docker works, not only whether it is installed.** `ssh vm sync`
  now asks a running guest's Docker engine for its version as the guest user, over the session
  `docker` itself uses. A guest with Docker still gets its context when the engine does not
  answer, and the host's note names the cause and the fix (the user is not in the `docker` group,
  the engine is stopped, rootless Docker without `DOCKER_HOST`). authama prints the fix and never
  changes the guest's groups or services.
- **Docker contexts in `doctor` and on the VMs page.** `doctor` lists the Docker contexts of VM
  hosts and runs `docker --context <alias> version` for each running VM (never starting one): a
  gone host, a context pointing elsewhere, or an engine that does not answer is a warning with
  its fix, and `--strict` fails on it. When docker hides the guest's reason behind its generic
  `http://docker.example.com` error, doctor asks the guest itself, so a refused socket reads as
  "denied" with the `usermod` fix rather than as a stopped engine. `--json` gains `docker_contexts`. The web UI's VMs page
  shows the same list, with *Check the engines* to run the check on demand.
- **Shared SSH connections for VM hosts (`vm.multiplex`, on by default).** A VM host's block
  renders `ControlMaster auto`, `ControlPath ~/.ssh/authama-cm/%C` and `ControlPersist 10m`, so a
  run of `docker` or `ssh` commands asks the hypervisor for the address once. `config render` and
  `vm proxy` create the socket directory owner-only; a value in the host's `raw_options` wins;
  `vm sync` ends a host's shared connection before replacing its pin or removing it, and its probe
  always opens a fresh connection. Not rendered on Windows. Set `vm.multiplex` false to turn it off.

- **Fix buttons on the Overview.** Each row that needs attention keeps its command and, where the
  web UI can make the fix, adds a button: *Preview the fix* for `ssh reconcile` and
  `doctor --fix`, which previews the change in place, and a link to Doctor, Keys or the Hosts
  page's *Pin host keys* panel for the fixes that need a choice. `init` and `migrate` stay on
  the CLI. A test holds every fix in `overview.go` to this list.
- **Progress on slow CLI steps.** On a terminal, `ssh knownhosts pin` and `ssh reconcile` show
  a one-line spinner with the elapsed time on stderr when a scan or the minting takes more than a
  second, cleared before anything else is printed or asked. Nothing is drawn on a pipe, in a log
  or under `TERM=dumb`, and no escape codes are written.
- **Empty lists say how to start.** `ssh snapshots list`, `ssh validate` and `ssh vm sync` with
  nothing to show now name the first step, as the web UI's empty states do. Each line still
  starts with the words it printed before.

- **`ssh git status --json`.** One document, `{"version": 1, "default_mode": …, "checkouts": […]}`,
  with each checkout's path, state (`managed`, `unmanaged`, `no-origin`), URL, mode, owner, repo,
  profile and where the profile came from (`remote` or `owner`). Fields are only ever added; a
  rename or removal bumps `version` and is noted here. The text output is unchanged.
- **Move the key root from the web UI.** *Move the key root* on the Keys page runs `ssh key root`
  as a previewed change. It moves every key, so it is made only when the new directory name is
  typed again. The name is one directory under `~/.ssh`, checked as the manifest checks
  `defaults.key_root`, and the preview lists every key the rename would move.
- **Import from the web UI.** *Import an ssh config* on the Profiles page runs `ssh import` as a
  previewed change, from this machine's `~/.ssh/config` or the config home's `ssh/config`, chosen
  by name; the web never takes a path. The config home is `$XDG_CONFIG_HOME` when it is absolute,
  else `~/.config`, resolved as authama resolves its own config home. Replacing a manifest that has profiles needs *Replace*, as
  the terminal needs `--force`, and is backed up first. A preview's copy holds no private key, so
  each key the import would copy in is listed and stood in for by an empty file, and apply copies
  the real one.
- **Validate from the web UI.** A new *Validate* page under **System** runs every validate check on
  one page when you open it: the manifest loads and passes its rules, `settings validate` (authama.json
  against its schema, `.env`'s syntax and permissions), `ssh validate` (each keypair parses, its public
  key matches and its permissions are safe) and `ssh config check`. Each result names the command that
  runs the same check. A manifest that does not load fails the key and config checks too, rather than
  reporting them clean. It writes nothing, so read-only servers offer it.
- **Naming from the web UI.** A new *Naming* page under **SSH** shows what `ssh naming show` and
  `ssh naming check` print: the rules in effect, each profile's code and own rules, and the servers
  whose alias breaks them. It changes the rules for every profile or one profile (`naming set`), the
  key template (`naming key-template`) and a profile's code (`profile edit --code`), each previewed
  first. A rules form posts every field but turns only the ones you changed into flags, so the
  preview reads as the command you would type and a field you left alone is never written. Every
  existing host keeps its name and key name, as in the terminal.
- **Rebuild public keys from the web UI.** A key's page, its row's menu, and *Rebuild public keys* on
  the Keys page (one profile, or every key) run `ssh key regen-pub` as a previewed change. The
  preview's copy holds no private key, so each public key is derived from the real one, read in
  place and copied nowhere, and the preview shows the `.pub` it would really write. A key with a
  passphrase is skipped with the command to run by hand, as in the terminal.
- **Check connections from the web UI.** The Overview and Hosts pages run `ssh net` on request and
  show each host's status in the command's words, warning when a host that needs a VPN does not
  answer; a host's page checks that host alone. No page probes as it loads, the check takes only
  manifest names, and read-only servers offer it.
- **Pin host keys from the web UI.** A host's page scans it, shows each key's fingerprint, and pins
  the keys only once you trust them, as `ssh knownhosts pin` does. **Hosts → Pin host keys** runs
  `ssh knownhosts init` for a profile or every profile (trust on first use). Both run as jobs, with
  the same notice and service as the CLI, and both work in dev mode, which reaches no server. The
  web pins manifest hosts only: no free hostname or port goes to `ssh-keyscan` from a form. A host
  with nothing pinned now shows the shared empty state.
- **Transforms in `authama export`.** A selection can now template a column (`{profile}/{alias}`),
  map values, format text, dates and numbers, split and join lists, dedupe rows, and group them with
  counts, sums, minimums, maximums, lists and a totals row. `--template`, `--dedupe`, `--group-by`,
  `--aggregate`, `--totals` and `--transform` (one step as JSON) add them at the command line, and a
  saved selection holds them under `transforms`. A column can name a field a transform wrote. The SSH
  client formats refuse transforms, because they write the hosts themselves.
- **Transforms on the Export page.** A *Transforms* card adds, edits, reorders and removes the same
  steps, and shows how many rows each one leaves. The preview, the download, *Copy link* and *Copy as
  command* all carry them.
- **Saved export selections.** `authama export selections save <name> --from sel.json` keeps a
  selection by name, and `authama export <name>` exports it. It goes in your own store (a private
  file under the state home) or in the manifest (`defaults.export.selections`, so it travels with
  it). The new `export.save_to` setting picks the default store. Both stores are read, and on a
  name in both yours wins. `list`, `show` and `delete` complete the set, and a manifest save or
  delete asks first, like any manifest change.

- **Pivot, saved selections on the Export page, compare, and scheduled exports.** A `pivot` step
  (`--pivot rows:column`) makes a row per value of one field and a column per value of another,
  counting by default, with optional totals. The Export page gains a *Saved* card. It opens, saves
  and deletes saved selections: yours at once, the manifest's through a change preview. Its compare
  button shows the rows only one of two selections holds, side by side. The *Add column* list
  groups related fields such as `key.*` and `profile.*`. `authama export schedule add <name> --to
  <folder>` writes a saved selection to a folder every day once `authama export schedule install`
  has registered the daily job.
- **Hosts for ssh and SSH clients in `authama export`.** `authama export hosts --format` takes
  `ssh_config`, `termius` and every `ssh host export` client (`hosts-csv`, `windows-terminal`,
  `iterm2`, `putty`, `remmina`, `securecrt`, `mobaxterm`), filtered like any list. `ssh_config` is
  the managed config of the hosts kept, byte for byte `ssh config export --flat` when every host is
  kept; the others are byte for byte the old commands' files. Hosts come in manifest order unless
  `--sort`, git hosts need `--include-git`, and remmina's files go out as one zip. `/exports/run` and
  the Export page's Hosts tab offer the same formats, and the page says how many hosts a format
  leaves out.
- **Copy as command on the Export page.** It copies the `authama export` command that writes the file
  the page previews: the list, filters, columns, sort, format and options, quoted for the shell.
- **About shows this computer.** A *This computer* card lists the OS and its version, build and
  kernel, the architecture and computer name, how long it has been up, and the hardware: model,
  processor, cores (performance and efficiency on Apple silicon) and memory. It is read on each load
  from sysctl on macOS, `/proc`, `/sys` and os-release on Linux, and the registry on Windows,
  without running a command; anything an OS does not expose is left out.

- **`authama export <list>`**: any list of the home (hosts, keys, profiles, tokens, history,
  events, snapshots, deployments) filtered (`--profile`, `--key`, `--env`, `--role`, `--tag`,
  `--status`, …), with the columns chosen (`--columns field=heading`, related fields by path),
  sorted and limited, in any of the nine formats, to stdout or `--out`. `--selection` takes a
  selection saved from the Export page, and `--fields` lists a list's fields. With no options it
  prints exactly what the list's `--csv` prints.
- **Keyboard shortcuts in the web UI.** `?` opens a sheet listing them all; `/` opens the command
  palette like Ctrl+K; `g` then a letter goes to any page (`g h` Hosts, `g k` Keys, …) and `n` then
  a letter opens the add-a-profile, host or token form. They never fire while typing in a field or
  with a dialog open, and the sheet's switch turns the single-key ones off for that browser.
- **An Export page.** Export, in the menu (`web.menu_export` hides it), makes a file of any list:
  a tab per kind of row (hosts, keys, profiles, auth tokens, history, events, snapshots,
  deployments), filter tables with counts (profiles, keys grouped by profile, environments, roles,
  tags, status, tools, kinds, areas, sources), include and exclude text and a date range, columns
  to tick, rename and reorder (by keyboard too), a sortable preview with a badge per reason rows
  were left out, and any of the nine formats to download, copy, or share as a link. The choice is
  kept in the page's address, so a reload or a shared link shows the same file.
- **Host exports for SecureCRT and MobaXterm.** `authama ssh host export securecrt` writes SecureCRT's
  settings XML (File > Import Settings) and `host export mobaxterm` a `.mxtsessions` file (Sessions >
  Import), each with a folder per profile and each host's own key. Termius imports both, so every
  import it offers (`~/.ssh`, CSV, SecureCRT, PuTTY, MobaXterm) now has an authama export.
- **The web UI follows a new build by itself.** `serve` watches its own binary; after `make
  install`, once the new file holds still, it restarts itself from it on the same address, hands
  every signed-in session to the new process, and open pages reload into the new build within a
  few seconds and say so. A page with unsaved input offers **Reload** instead. `web.auto_restart`
  (on by default) turns it off.
- **Restart the web UI from the page, or every one from the terminal.** The older-build banner has
  **Restart now**: the server relaunches from the binary now installed, on the same address with
  the same sign-in, and the page reloads itself. About marks the running build *current* or
  *older*, and a green banner confirms a restart. `authama serve --restart` stops every authama web
  UI answering on the usual ports and starts the new build; `--stop-all` only stops them. To this
  machine, `/healthz` now also says the server's pid, version and start time, so any one found can
  be stopped; an older server that does not say is named with the command that stops it.
- **Any list exports in nine formats, filtered and with the columns you choose.** `GET /exports/run`
  takes a built-in preset (`?preset=hosts`, one per list) or a selection (`?s=`, base64url JSON):
  rows of profiles, keys, hosts, tokens, history, events, snapshots or deployments; filters on
  profile, key, env, role, tag, status, tool, kind, area, source, command and a time range; columns
  that follow relations (`key.fingerprint`, `profile.code`); sort and limit. `?format=` is csv, tsv,
  json, jsonl, yaml, php, js, markdown or html, each escaped for what reads it. `?preview=1` answers
  with the first rows, the rows left after each filter, every field and every filter value with its
  count. Each preset as CSV is byte for byte its list's CSV.
- **`GET /api/dataset`** serves a home's records as related JSON (profiles, keys, hosts with every
  SSH setting and their naming parts, tokens without their values, and with `?parts=` history,
  events, snapshots and deployments). No secret is on any record, by type and by test.
- **The installers can install from a release of your own.** `AUTHAMA_RELEASE_BASE` (`-ReleaseBase`
  for `install.ps1`) names where the binary and `checksums.txt` are read from instead of the GitHub
  release: a URL, or a directory (`file:///path` for `install.sh`). CI uses it to install a binary
  it just built, since binary releases are on hold and the release carries none; the checksum is
  verified, not skipped.
- **The web UI says when it is older than the installed binary.** A running `authama serve` keeps
  the code it started with, so after `make install` the browser showed the old UI with nothing to
  say why. Every page now shows a banner naming the binary that changed and how to restart.
- **Keys can be chosen in the profiles CSV too.** It gains a `key_names` column listing each
  profile's keys, and its download dialog a Keys tab, grouped by profile, with the hosts each key
  is used by. Ticking keys keeps the profiles holding one; *Count only the ticked keys* (on by
  default, `?narrow=1`) cuts each profile's `keys`, `hosts` and `key_names` down to them, and off
  keeps a profile's whole counts.
- **A CSV with no key column says where keys are chosen.** The Termius CSV has profiles but no key
  column, so it has no Keys tab; the dialog now says so, with a link to the lists that have one.


### Removed

- **`authama tui`, the terminal menu.** The web UI is the interactive surface: a bare `authama` on
  a terminal opens it, and every menu entry was already a CLI command. `docs/tools/tui.md` lists
  where each entry went. History rows the menu recorded keep their `tui` source.
- **The `atm-ui` shell alias now runs `authama serve`.** A block installed earlier keeps calling
  `authama tui`, which is now an unknown command; `token shell-aliases --install` rewrites it.
- **The deprecated command spellings.** `authama ssh host export <client>` and
  `authama ssh termius export` are gone: `authama export hosts --format <client>` writes the same
  files (`hosts-csv` for the generic CSV, `termius` for the Termius CSV). `termius export --open`
  has no replacement. The hidden `authama tui` stub that pointed at the web UI is gone too, and so
  is `authama ui`, the deprecated alias of `serve`: run `authama serve`, or a bare `authama`.
- **The top-level SSH verbs.** `authama deploy`, `authama reconcile` and every other SSH verb no
  longer answer outside `authama ssh`. Two stay, hidden: `vm` and `audit`, because a
  `~/.ssh/config` rendered before this release runs `authama vm proxy`, and a notifier installed
  before it runs `authama audit --notify`. Rendering and installing now write `ssh vm proxy` and
  `ssh audit --notify`; run `authama ssh config render` and `authama ssh notify install` to move
  over.
- **The `SSH_MANAGER_*` and `SSHMGR_*` environment variables.** The 14 names releases before 0.1.1
  read were honoured with a deprecation notice; they are now ignored, and the notice is gone. Rename
  any you still set (`SSH_MANAGER_HOME` → `AUTHAMA_CONFIG_DIR`, `SSH_MANAGER_<NAME>` →
  `AUTHAMA_<NAME>`); see `docs/configuration.md`.
- **The deprecated default-root wrappers** in `internal/` (`GlobProfileTree`, `IsArchivedPath`,
  `ComputeStates`, `IterManagedPaths`, `LooseKeys` and the rest), which nothing outside their tests
  called.

### Changed

- **A job's review, a running job and an operation in the history are pages of their own**, no
  longer slide-in panels. The review keeps its checks, steps and keys on the left and a column on
  the right that stays in view, with what the job reaches, its backup, its undo, its options and
  its Start button; turning an option reviews the job again. Cancel goes back to the page the job
  was chosen on. A job's page puts its progress and item list in a column beside the output. Change
  previews still open in the slide-in panel. Rotate keys' steps follow its switches: a clean
  rotation drops the backup step, and Force adds its own.
- **`settings init --force` keeps the file it replaces**, as `authama.json.before-reset-<time>`
  beside the new one, so resetting the settings to their defaults can be undone.
- **Web UI: names open in the side panel, and Export's filters slide in on a phone.** A host's
  or key's name in a list (Hosts, Keys, Connections, Validate, Audit, the overview, and the links
  between a host and its key) now opens it in the slide-in panel over the list, like the row
  menu's Open; Ctrl- or Cmd-click and the panel's "Open as a page" still go to the page. Below
  the `lg` breakpoint, Export's saved selections and filters move into a panel opened by a
  Filters button, and the row kinds scroll on one line instead of wrapping into three.
- **Web UI: paths under your home show as `~/…`** on Settings, About, Auth tokens, the overview's
  State card and the Doctor report, with a copy button beside each for the full path. The token audit lists problems
  first, each finding with an icon for its outcome.
- **Web UI on a phone:** a page's actions take their own row under the title, the second action
  and More show their icons alone, and the overview's counts sit two to a row.
- **Web UI: tooltips only where there is no label.** A button or link that shows its name, a
  nav item, a menu entry and a dropdown toggle no longer show a tooltip, which covered the menu it
  opened and repeated the label; their explainer is read to a screen reader instead, and each
  navigation menu entry shows what its page holds as a line under its name. Icon-only buttons
  keep a tooltip, after half a second, one at a time, put away by any click, Escape, scroll or
  opening menu, panel or dialog, never shown while a menu is open and never on a touch screen.
  Each tooltip opens on the side with room, clear of its control and inside the window. A tooltip goes the moment the pointer leaves its control or the window, the window loses focus or the tab is hidden, and a watchdog removes one whose control was redrawn or removed; a clicked control shows none. No element carries a native `title` any more; preview cells wrap instead of cutting off text
  you could only read on hover. A copy button beside each `~/…` path copies the full path.
- **The web UI's tab icon and header share one logo mark.** The favicon is authama's key,
  drawn in white on the primary rounded square. The header shows that mark followed by the
  product name, Authentication Access Manager, on wide screens and `authama` on narrower
  ones. The About page uses the same mark. A test checks that every page links the
  embedded SVG favicon, that it is served as `image/svg+xml` and that it is the logo mark.
- `src/go.mod` retracts v0.1.0 to v0.1.3, which the Go module proxy cached from the deleted
  pre-restart history. The next release is v0.1.4 (`docs/release.md`).

- **A key that was never deployed now reads `not-deployed`, not `needs-redeploy`.** `list`,
  `view`, `audit`, `export`, the key minting lines and the web UI use the new status for a key with
  no deployment recorded; `needs-redeploy` now means deployments are recorded but none is
  verified. Scripts that matched `needs-redeploy` for fresh keys should match both values.

- The Audit page's deployments, the reserved keys and the declared devboxes show an empty list the
  way every other page does, with the commands that fill them set as code. The CSV dialog and the
  Export page now count, tick and untick their lists through one shared model, so the two
  pickers cannot drift apart.
- `ssh net` probes up to eight hosts at a time instead of one after another, so unreachable hosts
  cost one timeout between them rather than one each. Rows keep the manifest's order.
- **The lists' CSV downloads (`/exports/hosts.csv` and the rest) now come from the export
  pipeline**, the same one behind the Export page and `authama export`, for every option their
  links carry. The files are unchanged byte for byte, which a test checks for every list and option.
  The download dialog's preview and the history list's host, token, profile and key filters
  still use the lists' own tables.
- **Every control in the web UI explains itself.** Each button, menu entry, row action, tab and
  menu heading shows a tooltip on hover and focus saying what it does and what to expect (that a
  form only previews, that a copy changes nothing, what a job step does), which is also its
  accessible description. The command palette describes the highlighted entry, and sortable table
  headers say they sort. A test fails on any control without one.
- **The web UI header is two bars, not three.** The strip under the menu that read *Running as
  <user> on <host>* with the build's *current* mark is gone. The user and host now head the account
  menu, and About shows the build's status (*current* or *older build*), when it was built and when
  the server started, beside its version and commit. A read-only server shows a *read-only* badge
  by the name; dev mode keeps its banner, and an older build keeps its banner with **Restart now**.

- **The command palette is redesigned.** It sits high on the page at a capped height instead of
  filling the screen, groups entries under Pages, Actions and Downloads with compact rows, shows
  each entry's shortcut, follows the pointer, takes Home and End, and says when nothing matches.
  The header's *Search* button is now a search box that opens it, with a search icon on small
  screens.

- **The web UI uses Tabler's own components throughout.** Detail pages (a host, a key, an
  operation, a snapshot, a job) carry Tabler's breadcrumb back to their list; every panel form
  shares one footer, with its fields stacked and required ones marked; empty sections use the
  shared empty state; a job's question is marked with Tabler's card status bar. CSS no longer used
  by any page (log, editor and finding styles from another project, the hand-made tabs, clear
  buttons and dev banner) is removed, with the rule that forced panel columns to full width.
### Fixed

- **Create a backup key failed when pressed twice.** The second press found the key the first had
  made and refused. It now uses the key already there and sets the settings up from it.
- **A dialog opened inside the side panel could not be pressed.** The panel's stacking context
  held it under the page's backdrop. Every modal, wherever it is written, now opens from the page's
  body, in front of every slide-in panel, and goes back (or away, if its panel moved on) when it
  closes.
- **"Not found" after a server restart on a preview page.** A restart (after `make install`, say)
  reloads every open page into the new build; a preview, a job's review or a checklist tick answers a
  form, so its reload asked for `/changes/preview` by address and got *Not found*. The reload now goes
  back to the page the form was on, and those addresses say the page answered a form and nothing was
  changed (410) instead of 404. A test follows every link on every page and fails on a 404 or 500.
- **A rotation forgot which servers still have the old key.** Removing the outgoing key from
  each target was reported once and then lost. Rotation now records the targets it removed
  the key from, those it could not, and where the by-hand ones are managed, in `retired.json`.
- **Web UI: text typed into an Export step could vanish.** Committing a step's first field asks
  for a new preview, and its answer redrew the steps from the saved selection, wiping a second
  field still being typed in. A redraw now keeps uncommitted text, the focus and the caret, and
  Enter or leaving the field commits it.
- **Web UI: accordions showed no sign that they open.** Tabler 1.7 draws an accordion's chevron
  only inside `.accordion-button-toggle`, which no accordion had, so Export's Profiles, Keys,
  Roles, Tags and Status groups and About's licences read as plain headings. Each now shows a
  chevron that turns when it opens; a test fails on any accordion without one.
- **The installers retry GitHub when it answers 504.** `install.sh` and `install.ps1` retry the
  version lookup and every download on a timeout, a 408, a 429 or a 5xx, with backoff, before
  giving up. A 404 still fails at once, so the API's "no release" falls through to the
  `releases/latest` redirect as before.
- **The installers install the highest version tag when there is no release.** With binary
  releases on hold the repository publishes none, so `install.sh` and `install.ps1` found no
  version and stopped with "could not determine the version to install". After the latest release
  and its redirect, they now read the tags and take the highest `vX.Y.Z`, never the Go module's
  `src/` alias or a pre-release.

- `ssh vm add --hypervisor orbstack|podman`, a devbox on either, and the same web form sent you to
  `ssh vm docker-context`, which needs a VM host those machines can never be. The refusal now
  says where their Docker is: OrbStack's own `orbstack` context, or Podman's socket for
  `DOCKER_HOST`. The `ssh vm --help` text no longer says they get a Docker context.
- On the Export page, committing a transform step's second field before the preview had answered
  the first wrote the step back as it was, so a template step lost its column name and stayed
  unapplied. The column headings, and moving or removing a step, had the same flaw. Each now
  reads the selection as it is when the change happens.
- A word a nested group has no verb for (`authama ssh host export`, `ssh termius export`, `ssh profile
  nosuch`) printed the group's help and exited 0, so a script still calling a removed verb succeeded
  and did nothing. Every group without an action of its own now refuses it with exit 1 and a
  suggestion, as `ssh` and `token` already did; the group on its own still prints its help.
- The web UI's list of terminal-only changes still named pinning host keys, which the web has done
  since #140.
- A bare `authama`, which opens the web UI, ran inside a history scope labelled `tui`, the scope
  `serve` was already kept out of because a scope around the server claims whatever it sees
  outside a change. The bare run now gets none: each change made in the web UI is its own
  operation with `web` as its source, as under `serve`.
- After answering a job's question in the side panel, the clicked button's tooltip stayed on
  screen once the panel reloaded without the button. The panel now hides any open tooltip before it
  swaps its content.

- Tabs on the Export page and in the CSV dialog used Bootstrap's `nav-underline`, which picked up
  Tabler's grey hover box; they are Tabler's own `nav-bordered` tabs now. Three controls showed two
  tooltips at once (a `title` beside the explainer); a revoked key's disabled Deploy button showed
  none, and now explains why through its wrapper.

- The Overview's quick actions (Reconcile, Re-render, Apply the token config) and Re-render on the
  SSH config page never showed their explainers: the tip was on the form's hidden input, not the
  button.

- `make uninstall` also removes the Touch ID helper `authama-touchid` that `make install` puts
  beside `authama`.

### Documentation

- The release command in `docs/release.md` and `release.yml`'s header names the repository
  (`gh workflow run release.yml --repo simtabi/authama --ref vX.Y.Z -f confirm=release-go`), so it
  works from any directory, not only inside a checkout.

- `docs/release.md` and `docs/migration.md` no longer say the Homebrew tap renames `sshmgr` to
  `authama` or that the Scoop `sshmgr` manifest is published: the cask, the rename and both
  manifests were withdrawn while binary releases are on hold.

- The keepalive workflow calls the reusable workflow at a commit, not at `main`, so an upstream
  change cannot alter this repository's CI unseen.
- The migration guide no longer offers Homebrew, Scoop or `go install` as a way to install, nor says
  the installers carry both binaries: releases are on hold, so it points at building from source.
  The README's `export` line names `authama export <list>`. A test fails on a docs page that offers
  a release channel as current, outside the release page and the historical upgrade notes.

- Installation covers checking what is installed, restarting the web UI after an update (with the
  commands to stop a server on the usual ports), and uninstalling, including what is kept.

## [0.1.0] - 2026-11-06

The first release of authama: SSH keys and config, and package-manager and git auth tokens, from one manifest, at the command line and in a local web UI. It brings together what earlier builds of this history shipped as 0.1.0 to 0.1.4, and the work after them.

A key you keep on purpose is no longer a warning: reserve a loose `~/.ssh` key for a devbox you have
not made yet, or declare the devbox without a VM, and `doctor` lists it as information.

Local VMs look after themselves: `ssh vm sync` keeps every VM host matching its VM, the hypervisors
you use are configured (with a generic command driver for any other tool), a loose devbox key can be
adopted with a guided command, and `doctor --fix` runs the same upkeep.

The final rename: authama is the only primary name. Release assets, packages, the Homebrew cask and
the Scoop manifest are all `authama`; `sshmgr` remains only as a deprecated alias that still runs
every command. Upgrading is covered in [docs/migration.md](docs/migration.md#to-012-authama-everywhere-sshmgr-deprecated).

The rename to authama is complete: one config home, `AUTHAMA_*` variable names, and settings
files that authama loads itself. Everything from 0.1.0 keeps working and moves over on the first run.

First release under the authama name, with version numbers restarted at 0.1.0. authama (Authentication
Access Manager) is one static, dependency-free binary that manages two kinds of credentials:

- **SSH** (`authama ssh …`, formerly ssh-manager 3.6.1): per-profile keys, a split `~/.ssh/config`
  rendered from one manifest, a single pinned `known_hosts` trust store, staged zero-downtime key
  rotation with rollback, deploy to 25 providers, expiry reminders, encrypted bundles, snapshots, devbox
  and Cloudflare Access hosts.
- **Auth tokens** (`authama token …`, formerly auth-token-manager 0.3.0): Composer `auth.json`, `.npmrc`,
  Docker `config.json`, git credentials and `.netrc`, built from a config that names environment
  variables, never secrets; list, copy, set, delete, audit and import, owner-only atomic writes.
- **Shared:** `doctor`, `init`, `migrate`, `history`, a terminal UI (`tui`) and a local web UI (`serve`).

Two binaries ship: `authama`, and `sshmgr`, the same program under its original name so existing scripts,
aliases and `ProxyCommand` lines keep working. Config stays in `~/.config/ssh-manager`, and a 3.x setup
needs no migration. See [migration](docs/migration.md).

### Added

- **Servers named by rules you set.** `defaults.naming` holds the alias template
  (`{code}-{env}-{role}-{nn}` by default), the allowed environments and roles, a key-name template,
  how strictly they apply (`off`, `warn`, `error`), the first number (`00` or `01`) and what a
  profile without a `code` uses for one; a profile's own `naming` overrides them, for a client with
  its own convention. Parts can be optional (`{env?}`) and the number any width (`{nnn}`).
  `authama ssh host add simtabi --env prd --role web` names the server `simbi-prd-web-00`, or the next
  free number, and tags it. `authama ssh naming show`, `check`, `set` and `key-template` manage the
  rules, and no change to them, or to a profile's code, renames a key already minted.
- **A CSV export can hold some profiles, and some keys within them, chosen in tables.** The download
  dialog has a tab per list (profiles, keys grouped by profile, and columns with their first value),
  each a table whose header box ticks all, none or some of the rows shown. Everything starts ticked;
  ticking nothing disables the download and says why. The summary has a badge per reason rows are left
  out, each one click to undo (`?profile=`, `?key=` for a script).
- **Host exports for more SSH clients.** `authama ssh host export <client>` writes a Windows Terminal
  JSON fragment, iTerm2 Dynamic Profiles, a PuTTY `.reg` (with the `puttygen` command for each key),
  Remmina profiles, or a generic CSV with the key path (Royal TS, Xshell, SecureCRT, a spreadsheet).
  The Windows Terminal and iTerm2 profiles run `ssh <alias>`, so ProxyCommand and VM hosts work there
  too; the others leave such hosts out and say so.
- **Touch ID and Windows Hello stand in for the owner PIN at the terminal.** On a Mac,
  `authama-touchid` (a small Swift helper, built beside `authama` by `make build` and carried in
  `Authama.app`) asks LocalAuthentication; on Windows, authama asks Windows Hello through Windows
  PowerShell's WinRT `UserConsentVerifier`, with no helper to install. `authama owner fingerprint on`
  turns it on, as it already did with fprintd on Linux. Over SSH or Remote Desktop it reports itself
  unavailable, and any failure falls back to the PIN. The helper is ad-hoc signed, not notarized.
- **Five more token targets: Maven, Yarn Berry, Helm and the GitHub CLI.** `maven` writes a
  `<server>` into `~/.m2/settings.xml`, spliced so every other byte stays put and a brace is escaped
  as Maven needs; `yarn` writes `npmRegistries` in `~/.yarnrc.yml`; `helm` sets the login on a
  classic chart repository in `repositories.yaml`, and `helm-oci` writes an OCI registry login in
  `registry/config.json`; `gh` writes an account's token into `hosts.yml` where gh reads it outside
  the keyring. The YAML files are edited through a node tree that keeps key order and comments,
  with `go.yaml.in/yaml/v3`, a recorded dependency.
- **The web UI's address is configurable.** `web.addr`, `web.port` and `web.base_path` in
  `authama.json` (or `AUTHAMA_WEB_ADDR`, `AUTHAMA_WEB_PORT`, `AUTHAMA_WEB_BASE_PATH`) change the
  default `http://127.0.0.1:7822/`; a configured port that is taken falls back like the default, and
  `--addr`/`--port` still win. With a base path such as `/authama` the UI is served at
  `http://127.0.0.1:7822/authama/`, every link, form, asset, redirect and the session cookie carry
  it, and a test crawls every page to check none escapes it.
- **Actions explain themselves.** Page actions, Preview and Review buttons, quick actions,
  destructive menu items, Run and CSV show a tip on hover and keyboard focus saying what they do,
  which screen readers announce too. The wording lives in one catalogue, and a test fails on a tip
  a template names that the catalogue lacks, or one nothing uses.
- **A host's or key's row actions stay on the list.** Open, History and Rotate or roll back open
  in the side panel instead of leaving the page, like the row's other actions already did.

- **Keys in a directory of your choosing.** `defaults.key_root` names the directory under `~/.ssh`
  that holds the profile directories (`profiles` by default), and `authama ssh key root <dir>` moves
  the whole tree there in one rename, re-points the inventory and renders the config again. Every
  scan, snapshot, audit, permission check and backup follows it; a backup restores under the root
  its manifest names. `authama doctor` reports keys left behind by a hand edit
  (`stranded_key_root`).
- **Keys where you already keep them.** A declared key's new `path` adopts an existing private key
  under `~/.ssh` (`id_ed25519`, `work/deploy_key`) in place: it renders, deploys, rotates and audits
  there, and authama never moves, renames or deletes it. Rotation archives the old pair in the
  profile's `old/` and writes the successor to the same path. `authama ssh key adopt` declares and
  records one. See [docs/configuration.md](docs/configuration.md#adopted-keys).

- **`authama repair`** finds what is wrong with the home, `~/.ssh` and the token files and fixes
  what is safe: missing directories, a corrupt history database (renamed aside, kept), files a
  crashed command left, wrong modes, missing `.pub` files, a drifted `~/.ssh/config`, and token
  files others can read. What it cannot fix safely - an unreadable manifest, a changed seal - it
  reports with the command that deals with it. `--check` is for CI. The web UI's new Repair page
  does the same as a previewed change.
- **`authama reset`**, a factory reset that deletes nothing: it unbinds the owner, then renames the
  config and state homes aside; `~/.ssh` is not touched. It asks for the word "reset", and the PIN
  when one is set. Also on the Repair page.
- **The owner binding from the web UI**: bind the home, set, change or remove the PIN, toggle
  fingerprint unlock, accept outside changes and unbind, on the Owner & PIN page. A PIN is only
  set when `owner pin reset` could replace it, so it cannot lock you out.

- **One layout for every page's actions.** Page headers hold the main actions and a More menu;
  every form opens in a slide-in panel, from the header or from a row's menu, which fills it in for
  that row. Profiles, Hosts, Keys, VMs, Snapshots and a host's page no longer stack form cards below
  their tables. The rendered SSH config and a host's block are highlighted, the Settings page marks
  the theme in effect, and the Audit page's alert buttons sit in its Expiry card.

- **The web UI works in place.** Previewing and confirming a change, a job's review and live
  output, an operation in the history and a snapshot's diff open in one side panel over the page,
  with Back and *Open as a page*; the page underneath reloads when a change was made. Every such
  page still works on its own without JavaScript. A change's review leads with its command,
  highlighted, with copy and Run.
- **Diffs are numbered and highlighted**, as a table of old and new line numbers in the file's own
  syntax (JSON, INI, TOML, YAML, properties, ssh_config, netrc, shell), with added and removed counts
  and, for JSON, whether the new content is valid and where it is not. They replace the old diff,
  whose lines rendered with a dark empty line between each.
- **CSV downloads are previewed and shaped**: choose the columns, leave out rows containing some
  text, see the first rows, then download; the choice is remembered per list. The server takes
  `?drop=`, `?exclude=` and `?preview=1` on every CSV, the Termius export included.
- **The tokens.json editor checks the JSON as you type**, names the line and column of a mistake,
  will not send it while invalid, and formats it.
- **The menu is regrouped**: SSH, Config & backups, Security (with a new Owner & PIN page), Activity
  and System.

- **Owner binding** (`authama owner`). `owner init` keeps a sealing key in the OS credential store
  (macOS Keychain, the Linux Secret Service, DPAPI on Windows, else an owner-only file) and seals the
  state files with it, bound to the OS user and machine. authama reseals after its own operations,
  so a file changed outside authama, or a home copied to another account or machine, is named by
  the next mutating command (which then does not reseal over it), by `doctor` (which fails, with a
  new `owner_seal` JSON key) and by the web UI's overview. `owner pin set` adds a PIN, stored as a
  PBKDF2-SHA256 hash: the web UI asks for it after the sign-in link and on every change confirmed
  more than five minutes after it was last given, and the command line asks before revealing or
  copying a secret and before a destructive command. Five wrong PINs start a growing wait.
  `owner pin reset` replaces a forgotten PIN by proving the OS account. On Linux, fprintd can stand
  in for the PIN at the terminal. See [docs/tools/owner.md](docs/tools/owner.md).

- **Four more token tools**: `pypi` (`~/.pypirc`, for `twine`, keeping `index-servers` in step),
  `cargo` (`credentials.toml`, for crates.io and named registries), `gem` (`~/.gem/credentials`) and
  `gradle` (`gradle.properties`, as `<repo>Username`/`<repo>Password`). Each merges into an existing
  file keeping everything authama does not own, is listed, audited, deleted and set like the others,
  and has a starter template and a docs page. pip, uv, Go, pnpm and Bun read a file `netrc` or `npm`
  already writes, which the token docs now say. Dev mode hides `CARGO_HOME` and `GRADLE_USER_HOME`.

- **Key lifecycle.** `authama ssh key lifetime` sets how long a key lives before rotation is due,
  in days (1 to 1825) or as a preset (`annual`, `semiannual`, `quarterly`, `monthly`), per key or as
  the default, and counts the expiry date again. `authama ssh key revoke` removes a key from every
  target it is deployed to and records its fingerprint in `revoked.json`, so `deploy` and
  `rollback` refuse it; the declaration, hosts and files stay, and each target it could not be
  removed from is named. `recycle` is another name for `rotate`. A key's page in the web UI sets its
  lifetime and revokes it, and shows its revocation; its row menu links to both. See
  `docs/tools/key-lifecycle.md`.

- **`make install` and `make uninstall`.** `install` builds, then copies `build/authama` to
  `~/.local/bin` (`BINDIR=` for elsewhere; `%LOCALAPPDATA%\Programs\authama` on Windows), and says
  whether that directory is on `PATH` and whether another `authama` shadows it. `uninstall` removes
  that one file after checking it answers `version` as authama, and prints where the config and
  state homes are; it never touches them, `~/.ssh` or a token file.
- **Row actions are one menu.** Every table row's actions - open, copy, history, replace, delete -
  are in one Tabler dropdown at the row's end, with the destructive one last and confirmed in a
  dialog. Restoring a bundle now confirms first too.
- **Copy from the web UI**: a key's public key, its fingerprint and identity file path, a host's
  hostname and `ssh <alias>`, a token's host and file. Public values only: a private key or a
  token's secret is copied only from the terminal. A `.pub` that is not a single public key line is
  never shown.
- **Edit `tokens.json` in the web UI**, in a dialog, previewed as a diff like any change. Only a file
  holding variable names opens, and an edit that adds a value is refused: names must be upper case,
  at most 48 characters, and not start like a known token (`ghp_`, `glpat-`, `npm_`, …); a credential
  header must take its value from `${VAR}`.
- **Help where it is needed**: a hint under each form field, a popover beside terms that need one, a
  tooltip on each icon button, and a guided tour of the header and overview (account menu → *Take
  the tour*), which offers itself once on a browser's first visit. A list's address takes `?q=` to
  open it already searched.

- **CSV export everywhere a list is shown.** `--csv` on `ssh list`, `ssh key list`, `ssh audit`
  (one row per key and deployment), `ssh snapshots list`, `token list`, `history` and `events`, and a
  download on each matching web page, plus the profiles list, with the same columns. History and
  Events downloads keep the page's filters, and the history's carries each operation's subjects and
  events. A token list never puts a secret in one (`--csv --reveal` is refused), and a cell a
  spreadsheet would run as a formula is neutralised.
- **Reading a secret is on the record.** `token copy`, and `token show` or `token list` with
  `--reveal`, each record an operation with a `token.copied` or `token.revealed` event naming the token
  and its file, never the value. Token commands are about a token subject, `TOOL/HOST`, which
  `authama history --token` (and a Token filter on the History page) finds; `--until` is on the page
  too. The history database moves to schema 2 to allow it, rebuilt in place with every row kept; an
  older authama opens a schema-2 database read-only.
- **The events log is tamper-evident**: each line is chained by SHA-256 to the one before, and
  `authama events --verify` (and the Events page) catches a line edited, removed or inserted, or a log
  cut short.
- **A command palette** in the web UI (Ctrl+K, ⌘K on a Mac): every page, the add forms and the
  downloads, as you type. A link to a form in a panel (`/hosts#add-host`) now opens the panel, which
  the dashboard's buttons needed and did not do.
- **`authama events` and the web UI's Events page track every change to the files authama is
  responsible for**: everything under `~/.ssh`, the config home, and each tool's credential file,
  whoever made it. A scan records size, mode, modification time and a SHA-256 per file, never its
  contents, and logs what was added, changed, removed or had its mode changed. The server scans every
  30 seconds, links each event to the operation that made it, and shows the newest in the header bell.
- **`authama ssh import` reads any layout, flat or nested.** PATH may be a config file or a folder: its
  `config` and what that Includes, a config home's `ssh/config`, or every `config` and `*.conf` up to
  four levels down. From another machine's copy of `~/.ssh`, an `IdentityFile ~/.ssh/<key>` is found
  in that copy. The help's own example, a folder, used to fail as "no ssh config file".
- **Manual pages and manual-style help.** `authama man [DIR]` writes a troff page for every command
  (`make man` writes them to `build/man`), and every `--help` prints the same sections: NAME, SYNOPSIS,
  DESCRIPTION, ALIASES, COMMANDS, OPTIONS, GLOBAL OPTIONS, EXAMPLES, SEE ALSO. Every `list` also
  answers `ls`, and every `delete` `rm`.
- An **About** page, with the third-party licences in it, and Tabler's header: a theme toggle, a bell
  with the newest file events, and an account menu named for the system user the server runs as, with
  a strip in the menu bar saying which user and machine. A Tabler footer with the copyright, version and
  links; choosing to follow the system theme is on the Settings page.
- **The UI's script is typed ES modules configured by data attributes**: a `Component` base with typed
  options from `data-*`, a fluent `El` builder, and an `App` that chains `.use(...).start()`; the
  table's API chains too. `tsc --strict` checks them in `make e2e-web`, with no build step between the
  source and the binary.
- **The dashboard** gains key health, auth tokens and snapshot cards, quick actions, hosts by profile
  and the newest file events. **Tables** sort by any column, with search, page size and pages for a
  long list; hosts are one table with a profile column. **Destructive actions** ask in a dialog before
  their preview, and adding a profile, host or token opens in a slide-in panel.
- **Auth tokens are managed from the web UI.** The Auth tokens page adds or replaces a token (a
  Composer service or npm, Docker, git or netrc, with the user name, npm scope and Docker
  credential-store mode `token set` takes), writes a starter `tokens.json` (`token init`), imports an
  auth-token-manager config (`token import`), and shows `token status` beside the list and the audit.
  The secret is the one value a page sends: the server holds it in memory under a single-use reference
  bound to the browser session, so no page, preview or confirmation carries it, and a secret on any
  other form is refused.
- **Profiles are managed in full from the web UI:** a new profile can be filed in a category and given
  a key comment, and a profile's key comment can be changed (`profile add --category --key-comment`,
  `profile edit --key-comment`).
- An **SSH config** page: every file the manifest renders and whether `~/.ssh` matches it, as
  `ssh config show` and `ssh config check` print, with the re-render and the flat export beside them.
- `authama auth`, the same group as `authama token`; every `token` command answers under either name.
- `help` in every command group: `authama ssh help deploy`, `authama auth help set` and
  `authama settings help init` print that command's help, as `authama help ssh deploy` does. A group used
  to read `help` as an unknown command.
- A **Settings** page in the web UI: where the settings files are, every setting's value in effect and
  where it came from, and what `settings validate` reports. When there is no `authama.json`, it offers a
  previewed *Create a starter authama.json*, the same change as `authama settings init`.

- **`authama ssh key reserve <name|path> [--for ...] [--note ...]`** and **`key unreserve <name>`**. A
  key pair directly in `~/.ssh` that no host uses and authama does not manage can be kept on purpose.
  The reservation is a new optional `reserved_keys` section in `manifest.json` (what it is for, a note,
  the public key's SHA256 fingerprint and the day); the key file is never moved, changed or read beyond
  its `.pub`. `--dry-run` and `--yes`; recorded in `history`.
- **Planned devboxes.** A `devboxes` entry in `authama.json` with a `key` and no `vm` keeps that key
  reserved for the devbox: only `profile` and `key` are required, `vm sync` adds no host for it and
  notes it, and giving it its `vm` later lets sync add the host and adopt the key. The schema allows it.
- **`doctor`** prints a "reserved keys (kept on purpose, not dangling)" block instead of listing those
  keys as loose, and `--strict` passes. A reserved key whose file is gone, was replaced by a different
  key (fingerprint), or has since been adopted is a warning with its fix. `doctor --json` gains
  `reserved_keys`, after the 0.1.3 keys.
- Reserved keys show in **`key list`** (under the table), **`show <name>`**, the TUI banner and the web
  overview (a `reserved` row), the web **Keys** page (with Reserve and Unreserve, previewed like every
  change) and the **VMs** page (reserved keys, and "no VM yet" for a planned devbox).
- **Browser tests for the web UI.** `e2e/web/` holds Playwright tests that start `authama serve`
  against a throwaway home (a temp `HOME` and XDG homes, seeded from the example manifest) and drive
  every page, a change from preview to apply, a stale preview being refused, and a bundle restore job
  whose event stream is read to the end. An axe pass over the same pages fails on serious or critical
  violations in the light and the dark theme. Run it with `make e2e-web`; CI runs it on Linux in the
  new `web-e2e` job.

- **`authama ssh vm sync`** asks each VM host's hypervisor what became of its VM. A running VM gets its
  host key pinned when nothing is pinned, its public key re-installed in the guest when the guest
  refuses it, and Docker context `ssh://<alias>` refreshed. A VM that is gone has the manifest backed up,
  then its host entry (never its key files) and the Docker context authama made removed (exported
  first). A suspended or stopped VM is reported and never started, stopped or deleted. Removal needs
  a successful, non-empty listing; a second run changes nothing. `--dry-run`, `--yes`, `--quiet`,
  `--no-probe` and `--repin`.
- **A daily sync**: `ssh vm sync --install-schedule` (and `--remove-schedule`) runs `sync --quiet --yes`
  at 09:30 with launchd, systemd or cron, or Task Scheduler.
- **Configurable hypervisors.** `vm.hypervisors` in `authama.json` or `AUTHAMA_VM_HYPERVISORS="parallels,vmpal"`
  in `.env` names the hypervisors you use; the others are not queried. A `hypervisors` section adds a
  **command driver** for any VM tool: `list`, `address` and optional `start` and `exec` argv arrays with
  `{vm}`. Drivers live in a registry, so a new built-in is one registration.
- **Declared devboxes.** A `devboxes` section (profile, hypervisor, VM, guest user, key) lists the VM hosts
  sync should keep; sync adds a missing one and copies its loose key pair into the profile. Never pruned.
- **`authama ssh vm adopt-key`** turns a loose `~/.ssh` key into a VM host's managed key: it copies the
  pair (verified; the originals stay unless `--move`), adds the host and re-renders the config. At a
  terminal it picks the key, hypervisor, VM and user interactively.
- **`doctor`**: VM hosts and their state, unpinned VM hosts, stale authama pins in `known_hosts`, and the
  enabled hypervisors. A loose key whose name or comment suggests a devbox gets an `adopt-key` fix.
  New JSON keys `unpinned_vm_hosts`, `stale_pins` and `vm`, after every existing key.
- **Web UI: a VMs page** (`/vms`) with the VM hosts, a previewed sync (and `--repin`), the hypervisors
  and their VMs, the declared devboxes, an adopt-key form and the daily-sync schedule.
- Settings `vm.prune_gone` (`AUTHAMA_VM_PRUNE_GONE`) and `vm.repin_changed` (`AUTHAMA_VM_REPIN`).
- Docs for shell completion, the web UI's third-party licences page, and the six service packages
  `architecture.md` did not list.

- **`<config home>/.env` is loaded at startup.** A variable already in the environment is never
  replaced. Supports comments, an `export ` prefix, and single- or double-quoted values; a missing file is
  fine, and a malformed line is skipped with a warning naming its line number, never its content. A
  `.env` readable by group or others is not loaded (`chmod 600` or `doctor --fix`). `--no-dotenv` or
  `AUTHAMA_NO_DOTENV=1` skips it for a run.
- **`authama.json`**: optional non-secret settings in the config home, validated against
  `docs/schema/authama.schema.json` (JSON Schema 2020-12). An invalid file stops the command with
  every problem listed.
- Precedence, documented and tested: command-line flags > environment > `.env` > `authama.json` >
  defaults.
- **`authama settings`** (`path`, `show [--effective]`, `validate`, `init`): where the files are,
  every setting's value and source (secrets never printed), schema validation, and a starter file.
  It is a separate group from `ssh config`, which renders `~/.ssh/config`.
- A separate **state home** for history, the audit log, snapshots and bundles:
  `$AUTHAMA_STATE_DIR`, else `$XDG_STATE_HOME/authama`, else `~/.local/state/authama`.
- `make settings` and `make generate` (regenerates the CLI reference and the settings schema).

### Changed

- **The owner PIN is hashed with Argon2id** (t=3, 64 MiB, p=4), memory-hard where PBKDF2 costs only
  time. A PIN set by an earlier release still unlocks and is rehashed as Argon2id the next time it
  is given correctly. This links `golang.org/x/crypto`, a fourth recorded dependency.
- **`build/` holds one folder per OS and nothing else.** `make build` writes
  `build/<your-os>/authama` (`build/macos/authama`, `build/windows/authama.exe` - which also fixes
  `make install` on Windows, which looked for an `.exe` the build never wrote); `make cross` writes
  every target to `build/{os}/authama_{os}_{arch}` with `build/checksums.txt` listing `{os}/{file}`
  so it verifies as-is; `make dist` adds `build/archives/`. There is no `build/dist/`: GoReleaser
  works in `.goreleaser-dist/` (`make release-snapshot`), and `make man` packs its pages into
  `build/archives/authama_man.tar.gz`. A test fails on anything else in `build/`.
- **A double-click app for the web UI.** `make dist` packages `Authama.app` for macOS
  (`build/archives/authama_macos_app.zip`, opening Terminal running `authama serve`) and adds
  `Start authama.cmd` to the Windows zips. See [docs/installation.md](docs/installation.md).

- **`--revoke` revokes.** `key delete`, `host delete` and `profile delete --revoke` called a stub
  that revoked nothing, while their help said it removed the key from its targets. They now ask each
  deployment's host's provider to remove it, as rotation does, and report each target they could
  not revoke from rather than counting it as revoked.

- **The web UI's slide-in forms are one column, each hint under its field**: the token, host and
  profile panels no longer squeeze two fields side by side in a panel 400 pixels wide, where labels
  wrapped and fields fell out of line. The token form starts with the tool and shows only the fields
  that tool uses; a hidden field is disabled, so it is never sent. Each panel has a Cancel button.
- **The overview's three summary cards share one layout**, a count, a bar and three badged rows, so
  they line up: the token card's tool count comes from the catalog rather than a fixed 5, and the
  snapshot card shows kept of the most kept, the oldest and newest, and how many still hold private
  keys. The header bell's dot means an event in the last day, not that there was ever one.
- **`token set --service` beside a `--target` other than composer is refused** instead of writing a
  Composer token silently; the web form no longer drops the tool it was given either.

- **Running `authama` on its own, on a terminal, opens the web UI**, as `authama serve` does with its
  defaults; `--dev-root` and `--no-dotenv` still apply. The menu it used to open is `authama tui`. With
  no terminal a bare run still prints help, since `serve` would hang a script.
- **The web UI is redesigned on Tabler's own layout**, as its preview dashboard has it: a header with
  the name, the mode badges and the theme switch, then a menu with SSH, Auth and System dropdowns that
  folds behind a button on a narrow screen. Every page header names its section, has a one-line summary
  and links to its forms; the overview's counts are linked cards; empty lists use Tabler's empty state.
  The custom stylesheet is down to the contrast fixes the accessibility pass needs. Every page, form,
  change and job works as before.

- `vm adopt-key` and `vm sync` release a key's reservation when they adopt it; the adopt-key picker
  labels reserved keys. doctor's fix for a loose devbox-looking key also mentions `key reserve`.
- The issue-template security link names `security@simtabi.com`, the disclosure inbox, instead of
  the community address.
- `.github/pull_request_template.md` is `.github/PULL_REQUEST_TEMPLATE.md`, the name the org
  standard requires; the lowercase-markdown hygiene test lists it as a conventional name.
- CI verifies the gitleaks tarball against its published SHA-256 before installing it.
- CI, the release workflow and the Makefile set `GOPRIVATE`/`GONOSUMDB` for `github.com/simtabi/*`,
  so our own builds never feed a version into the public Go proxy or checksum database.
- The release workflow no longer starts on a tag push. Binary releases are on hold, and a release
  runs only when dispatched by hand on a `v[0-9]*` tag with `-f confirm=release-go`; a test fails
  if an automatic trigger returns.
- The README and installation guide build authama from source only: no download, package-manager
  or `go install` path is offered while binary releases are on hold. The 0.1.0 to 0.1.3 channels
  are described as frozen, not removed.
- Tests generate every token, password and passphrase at run time (`internal/util/testcred`)
  instead of spelling them as literals, and a hygiene test fails on a credential literal in any
  test file, the TypeScript browser tests under `e2e/web` included.
- A CI run whose only failures are jobs GitHub never gave a runner (the macOS arm64 pool's "not
  acquired" failure) is re-run once on its own by the new `requeue.yml`. A run with any real failure,
  a second attempt, or a run superseded by a newer one is left alone.

- **`doctor --fix`** also runs the VM sync and removes stale authama pins from `known_hosts`, after a
  confirmation that lists them (declining fixes permissions only). `doctor --dry-run` shows the plan;
  `--yes` skips the confirmation. Every change is backed up first.
- Docker contexts authama creates carry the description `managed by authama`, so only those are ever
  removed.
- The code of conduct is `CODE_OF_CONDUCT.md` again, the name the Simtabi conventions require
  verbatim; 0.1.2 had lowercased it with the other Markdown files.
- CI also runs weekly on `main` (Monday 06:00 UTC), so drift after a merge shows up; the release
  workflow pins GoReleaser to `~> v2` instead of `latest`.
- The README's H1 is the command name, and its feature list moved to the top of `docs/features.md`.
- `docs/reference/cli.md` ends with the docs-index footer every other page has.
- **`init --backup` help and the `ssh` group's help** say where the backup goes and that the bare
  top-level verbs still work.

- **Release assets are named `authama_*`**: archives (`authama_<os>_<arch>.tar.gz`/`.zip`), the
  bare binaries, the Linux packages (`authama_linux_<arch>.{deb,rpm,apk}`) and the source tarball
  (`authama_<version>_source.tar.gz`). The bare `sshmgr_<os>_<arch>` binaries are still attached for
  the deprecated alias. `install.sh` and `install.ps1` fetch `authama` and treat `sshmgr` as
  optional; against a release from before 0.1.2 they still fall back to the old asset names.
- **The Linux packages are named `authama`** and declare `Replaces`/`Conflicts`/`Provides: sshmgr`,
  so installing one upgrades an installed `sshmgr` package in place.
- **Homebrew: the cask is `simtabi/tap/authama`.** The tap maps `sshmgr` to `authama`
  (`cask_renames.json`), and the `sshmgr` cask is gone.
- **Scoop: the manifest is `simtabi/authama`.** `simtabi/sshmgr` is still published, marked
  deprecated, and tells you to switch on install (Scoop has no renames).
- **`sshmgr` is deprecated.** It runs the same commands but prints one line on stderr pointing at
  `authama`, except for `vm proxy` (ssh reads its stderr) and shell completion. It will be removed in
  a future release.
- **VM `ProxyCommand` points at `authama`**, even when the config is rendered through `sshmgr`: the
  `authama` beside it, else the one on `PATH`. Run `authama ssh config render` once to rewrite older
  lines.
- **`known_hosts` pins are tagged `authama`.** Pins tagged `sshmgr` by earlier releases are still
  recognized as this tool's own (`clean`, `ssh show`, pruning).
- **The expiry reminder job is `authama.expiry`.** `authama ssh notify install` removes the old `ssh_manager.expiry` job (launchd, systemd or cron, or the
  Windows scheduled task) after installing the new one.
- **The remote `authorized_keys` lock, temp and backup files are `~/.ssh/.authama.lock`,
  `authorized_keys.authama.XXXXXX` and `authorized_keys.authama.bak.*`** (were `ssh-manager`). An older release running against the same server at the same moment does not
  share the lock, so upgrade every machine that deploys to a shared server.
- Web UI and docs show `AUTHAMA_AGE_RECIPIENT` and `authama` throughout; the remaining
  `sshmgr`/`ssh-manager` spellings are the deprecated alias and the migration and compatibility
  paths.
- Markdown file names are lowercase: `code-of-conduct.md`, `third-party-notices.md` (in every
  archive) and `.github/pull_request_template.md`.

- **Config home is `~/.config/authama` on every OS**, Windows included
  (`%USERPROFILE%\.config\authama`, no longer `%APPDATA%`). `$AUTHAMA_CONFIG_DIR` overrides it, then
  `$XDG_CONFIG_HOME/authama`.
- **First-run migration.** When no config dir is set and `~/.config/authama` does not exist, the
  0.1.0 home (`~/.config/ssh-manager`, on Windows also `%APPDATA%\ssh-manager`, or an older
  `~/.config/sshmgr` / `~/.sshmgr`) is copied in, its logs, history, snapshots and bundles move to
  the state home, and the old directory is kept with a `MIGRATED-TO-AUTHAMA.txt` marker. Logged
  once. `authama migrate` now copies rather than moves.
- **`AUTHAMA_*` variable names.** `SSH_MANAGER_*`, `SSHMGR_DEV_ROOT` and `SSHMGR_GIT_MODE` still
  work, with a deprecation notice; the new name wins when both are set.
- The managed block in `~/.ssh/config` is branded authama; blocks written under the ssh-manager or
  sshmgr markers are recognised and rewritten in place on the next render.
- Bundles are named `authama-<stamp>.age`; `ssh restore` still reads `ssh-manager-*.age`.
- Provider key titles, desktop notifications, the recover message, temp directories and the web
  session cookie use the authama name; a re-deploy renames an `ssh-manager …` key title.
- `--dev-root` help names `$AUTHAMA_DEV_ROOT` and the legacy `$SSHMGR_DEV_ROOT`.
- `install.ps1` installs into `%LOCALAPPDATA%\Programs\authama` and takes the 0.1.0
  `Programs\ssh-manager` folder off the user `PATH` (its files are left alone). Both installers
  say where settings live.
- The package homepage (Homebrew, Scoop, winget, deb/rpm/apk) is https://github.com/simtabi/authama.
- `.env-example` uses the `AUTHAMA_*` names and quotes values with spaces or special characters.

- **Versions restart at 0.1.0.** The ssh-manager 1.x–3.6.1 and auth-token-manager tags and GitHub
  releases were removed; their history is kept below.
- **The Go module is `github.com/simtabi/authama/src`** (no `/v3`):
  `go install github.com/simtabi/authama/src/cmd/authama@latest`. The old `.../src/v3` path still resolves
  through the Go proxy for 3.6.1 and earlier, but is no longer maintained.

### Removed

- **The `sshmgr` alias binary.** It shipped beside `authama` as a deprecated compatibility build
  since 0.1.2 and is no longer built, released or installed: `make build`, both installers, the
  archives, the Linux packages, the Homebrew cask and the Docker dev image carry `authama` alone,
  and the `simtabi/sshmgr` Scoop manifest stops updating. An `sshmgr` already installed is left
  alone; the installers name it and print the command that removes it. Everything older releases
  wrote under the old name is still recognized. Before removing an old copy, run
  `authama ssh config render` (a VM `ProxyCommand` that names it) and `authama ssh notify install`
  (an expiry reminder job); see [docs/migration.md](docs/migration.md#removing-the-sshmgr-alias).

### Fixed

- **The tour could jump back to its first step.** On the overview it starts by itself after the page
  lays out; a tour started by hand in that moment was restarted by the timer. It is left alone now.
  The tour also sits above the hover explainers, so a tooltip under the pointer cannot cover Next.
- **Launching the web UI again started another server every time**, and none ever stopped: after
  twenty launches ports 7822-7841 were all taken and the next one failed. `authama serve` now opens
  the server already running for the home (checked through a new `/healthz` and an owner-only
  record), `--status` and `--stop` report and stop it, `--new` starts another on purpose, and a
  full port range falls back to any free port.
- **`token apply` skipped every PyPI, Cargo, RubyGems and Gradle entry** as an "unknown target":
  resolving a config from the environment knew only the first five tools. They resolve now.
- **Saving the manifest turned numbers and booleans in `raw_options` into strings**
  (`ServerAliveInterval 60` became `"60"`) whenever anything else in it changed. A value that was
  not changed is written back exactly as it was read.
- **Menu labels showed `&amp;`** ("Config &amp; backups", "Owner &amp; PIN"), and several pages'
  section names did not match the menu they are in.

- **A token's History link signed the browser out.** The history's token filter used `?token=`,
  which is also the sign-in link's parameter, so following History from a token's menu answered
  401 "Sign-in link required" to a browser that was signed in. The filter is `?credential=` now,
  and a `?token=` that is not the server's sign-in token no longer overrides a live session.

- The Auth tokens page and the dashboard said a credential file was readable by others whenever the
  token audit failed, including when what failed was the `.gitignore` check where the server runs.
- A web preview now shows `authama.json` and `tokens.json` with the other config files. It was left out of the preview's
  copy, so a change writing it would have previewed as writing nothing.

- **Changes made from the web UI in Chromium were refused as cross-origin.** The server sent
  `Referrer-Policy: no-referrer`, under which Chromium sends `Origin: null` on the page's own form
  posts, and the same-origin check refuses that. It now sends `Referrer-Policy: same-origin`, which
  still sends no referrer to another site. Found by the new browser tests.
- **Four contrast failures in the web UI**, found by the axe pass: the outline delete button on a
  card footer in the light theme, the Cancel link button on a review page in both themes, and `code`
  in the overview's attention rows and in alerts in the dark theme.

- **Docs corrected against the code.** SECURITY.md no longer says a key passphrase goes on
  `ssh-keygen`'s command line (it goes through askpass and stdin) or that each profile has its own
  `known_hosts` (there is one hashed store). `.state/`, `dist/` and the `init --backup` copy are
  placed in the state home, where they live. `serve` shows its real start-up line, `settings init`
  is described as the minimal starter it writes, and install examples pin v0.1.2.
- **winget is no longer listed as an install method.** No release has published it: its GoReleaser
  manifest waits on a `simtabi/winget-pkgs` fork that does not exist yet. `docs/release.md` now says so.
- **The age identity has no default path.** `docs/configuration.md` said `paths.age_identity_file`
  defaults to `<config home>/age-identity.txt`; nothing reads that file unless `--identity` or
  `AUTHAMA_AGE_IDENTITY_FILE` points at it, and the docs now say so.
- **The web UI places bundles in the state home's `dist/`**, where they are, not the config home's.
- The 0.1.2 entry named `known-hosts show`; the command that shows pins is `ssh show`.

- `doctor --strict` exits non-zero on any warning and never reports a clean tree while listing
  warnings.
- `history` records a host's resolved alias instead of the raw `--host` spec, and `history --host`
  still finds entries written by 0.1.0.
- The `.env` parser no longer takes `KEY=   # note` as the value `# note`.

- **`--no-passphrase` works again on `keygen`, `reconcile` and `rotate`.** v1 accepted it; the 2.0.0 Go
  rewrite dropped it without a note, so scripts that passed it failed with "unknown flag". It is back,
  hidden, as the spelling of the default (no passphrase), and cannot be combined with `--passphrase`.
- **A custom token catalog can be used again.** auth-token-manager read its service catalog from
  `$AUTH_TOKEN_MANAGER_CATALOG`; the port embedded the catalog and lost the override. `authama token`
  now reads `$SSH_MANAGER_TOKENS_CATALOG`, and the old name with a deprecation notice. A missing file
  warns and falls back to the built-in catalog; an invalid one stops the command.

### Security

- The web server refuses a request body over 1 MiB (413) before parsing it, holds headers to 64 KiB
  and a body to 30 seconds, and sends a `Permissions-Policy` that turns off every device API.

- The web UI's sign-in redirect is now guarded inline, on the exact value passed to `http.Redirect`, by a
  pattern equal to `isLocalURL` (`/` alone, or `/` not followed by `/` or a backslash). Behaviour is unchanged:
  a token link on a path a browser reads as another host still lands on `/`. A test holds the pattern and
  `isLocalURL` to the same answer. Resolves CodeQL `go/unvalidated-url-redirection` alert 12.

### Compatibility

- A manifest with `reserved_keys` is refused by authama 0.1.3 and earlier (unknown fields are an
  error). Without reservations the manifest is written exactly as before.

### Documentation

- Configuration, installation, migration, architecture and every recipe and tool page describe the
  new homes, `.env` loading, `authama.json` and the `AUTHAMA_*` names. The `set -a; . .env` workaround
  is gone (it still works, but is not needed).
- Every install channel is documented: the install scripts (with a pinned version
  and a per-user directory), Homebrew, Scoop, `go install`, and the `.deb`/`.rpm`/`.apk`
  packages. The README gains a feature overview.
- The quick start adds `--provider github`, so the host it creates is the GitHub
  host the later `ssh list --type vcs` step expects.
- Branding is consistent (Authentication Access Manager, `authama`, with `sshmgr`
  as the compatibility name) across CONTRIBUTING, SECURITY, the issue templates
  and the docs; examples use placeholder identities only.
- A test parses every `authama`/`sshmgr` command shown in the docs against the
  real command tree, so a renamed verb or flag fails CI.

- **[release](docs/release.md#homebrew-and-scoop)**: the Scoop manifest no longer needs a manual pull request.
  The bucket's workflow opens and merges the `sshmgr-X.Y.Z` branch the release pushes.
- **[migration](docs/migration.md#history-audit)** records an audit of every tag, release note and commit
  of sshmgr and auth-token-manager against this release, with what was restored and what was dropped on
  purpose.

### Changed (CI)

- Pull requests test on Linux and Windows and run a darwin cross-check (build and vet for macOS arm64
  and amd64); the full macOS test runs weekly and on dispatch. The required checks follow.

## Pre-0.1.0 history (formerly ssh-manager 1.x–3.6.1 and auth-token-manager 0.x)

These releases came before the restart at 0.1.0. Their tags and GitHub releases were removed on
2026-11-02; each ssh-manager heading links to the commit its tag pointed at. auth-token-manager's
repository is no longer published; its code was ported at `20dda1e` (see [migration](docs/migration.md)).

### ssh-manager

#### ssh-manager 3.6.1 - 2026-11-01 ([tree f317e01](https://github.com/simtabi/authama/tree/f317e01e2010d16dc16e66516f93b1bd6a3d7b53))

##### Changed

- **The repository is now `simtabi/authama`** (was `simtabi/ssh-manager`), and the Go module path follows it:
  `github.com/simtabi/authama/src/v3`, so `go install github.com/simtabi/authama/src/v3/cmd/authama@latest`
  (or `.../cmd/sshmgr`) works from this release on; v3.6.0 and earlier still resolve only under the old
  path. GitHub redirects the old URLs, so existing clones, the installers' old raw URLs and release
  download links keep working. Both binaries, the config home and the Windows install folder are unchanged.

##### Fixed

- **Releases publish the Scoop manifest without breaking the bucket's branch protection.** v3.6.0's release
  failed pushing straight to `simtabi/scoop-bucket`'s `master`, which requires a pull request; the manifest now
  goes to an `sshmgr-X.Y.Z` branch and is merged by pull request. The release body and provenance
  attestation steps now run even when GoReleaser fails after publishing - v3.6.0 shipped without an
  attestation because of that failure.

#### ssh-manager 3.6.0 - 2026-11-01 ([tree 7848e5e](https://github.com/simtabi/authama/tree/7848e5e44213bd2368352560225f27d183dafaac))

##### Added

- **Auth token management, merged from auth-token-manager.** The new `token` group builds the credential
  files Composer (`auth.json`), npm (`.npmrc`), Docker (`config.json`), git (`.git-credentials`) and curl
  (`.netrc`) read, from a config that names environment variables: `apply` (with `--dry-run` and
  `--exports`), `set`, `list` (`--reveal` asks first), `show`, `status`, `copy`, `delete` (asks first at a
  terminal), `audit`, `examples`, `init`, `providers`, `import` and `shell-aliases`. Ported from
  simtabi/auth-token-manager@20dda1e2c56ee23a87c0768edb6fadbd0c298227: rendering is checked byte-for-byte
  against golden files from the Python code, and its pytest suite is ported to Go. Files are written
  atomically at `0600` and merged with what is there; a malformed file, a value that could inject a second
  credential, or a malformed host is refused rather than written. A Docker registry kept in a credential
  store is logged in (`docker login --password-stdin`) or refused, set by `--creds-store-mode` or
  `_settings.docker_creds_store`; with no terminal and no setting it refuses. Token writes appear in
  `history` and `audit.log` without the secret. `--dev-root` sandboxes every token file. See
  [docs/tools/tokens.md](docs/tools/tokens.md) and the [migration guide](docs/migration.md).
- **A new name: Authentication Access Manager, `authama`.** The binary is `authama`; `sshmgr` still ships
  beside it as the same program. SSH commands are grouped under `authama ssh` (the top-level spellings still
  work, hidden from help); `version`, `doctor`, `init`, `migrate`, `tui`, `history`, `serve`, `ssh` and
  `token` are the top level. Command names are distinct and prefix matching is off, held by a test. The
  module path, config home, `SSH_MANAGER_*` variables, managed-block markers and `known_hosts` tags are
  unchanged. Completion scripts are named for the binary that generates them.
- **Commands a token manager needs beyond auth-token-manager's flags.** `token show TARGET HOST` prints one
  entry (secret masked unless `--reveal`); `token status` is a read-only overview of the config in use, each
  tool's file and mode, and which variables are unset (it always exits 0; `token audit` is the check);
  `token providers` also lists the well-known npm, Docker and git hosts; `token import FILE` imports any
  config, with `--dry-run`; and `token delete` asks first at a terminal. Rotating or editing a token is
  `token set` again for the same host.
- **A complete command reference.** [docs/reference/cli.md](docs/reference/cli.md) lists every command with its
  flags and examples, generated from the command tree and held current by a test; every command now has
  examples in `--help`, each checked to parse. It ends with a guide to choosing between similar commands.
- **Auth tokens in the TUI and the web UI.** The TUI gains an *Auth tokens* group (list, status, audit, copy,
  set, apply, delete); `serve` gains a Tokens page listing installed tokens (never their secrets) with a
  previewed delete and apply. *Add or replace a token* asks what auth-token-manager's menu asked: the
  provider from the catalog, cloud or self-hosted (a typed URL loses its scheme and trailing slash), and the
  credential its mechanism needs, Bitbucket key and secret and Composer custom headers included; it offers
  *Replace* for each installed entry (the old *Edit*) and reminds you to add a self-hosted GitHub or GitLab
  host to `composer.json`'s `*-domains`.
- **`token shell-aliases`** installs `atm`, `atm-ui`, `atm-doctor` and `atm-init`, now calling `authama`.
- **A Go dev container.** `Dockerfile` and `compose.yaml` at the root, with `make docker-shell`,
  `docker-run` and `docker-down`. Its home is `./.authama-home` (`AUTHAMA_DEV_HOME`), never your real one.
- **Releases and installers carry both binaries.** GoReleaser builds `authama` and `sshmgr` into every
  archive and package; `install.sh` and `install.ps1` install both, verify each checksum, and honour
  `AUTHAMA_INSTALL_DIR` (and the old `SSHMGR_INSTALL_DIR`).

- **The web UI makes changes, each previewed first.** Profiles, hosts and VM hosts can be added, edited
  and deleted from the pages; a declared key no host uses can be deleted; and `~/.ssh/config` can be
  re-rendered. A preview runs the change for real on a throwaway copy of the home that holds no
  private key, and shows every file it would write. Confirming takes the lock, previews again, and makes
  the change only if nothing moved since; otherwise it shows the new preview. The lock is held only for
  that. Each change is in the history with source `web`, and the CLI and the web leave the home
  byte-identical (a test holds every change to its command). `--read-only` serves no change.
- **The web UI mints keys, prunes snapshots and fixes modes.** Reconcile, keygen (missing keys only)
  and key add preview with throwaway keys and say so; confirming mints the keys that stay. They never
  overwrite, take a passphrase or pin, and are refused for 1Password profiles. Snapshot pruning,
  `doctor --fix` and `providers --export` are there too, with what they do outside the files listed in
  the preview and bound to the confirmation.
- **The rest of the everyday changes are in the web UI.** `host apply` (a box of specs), moving a profile to a
  category, deleting a profile with its key files (always after an encrypted backup, so it needs
  `SSH_MANAGER_AGE_RECIPIENT`), `clean`, an encrypted `bundle`, a VM host's Docker context, a test notification
  and the audit's desktop alert. A preview now names the files it cannot show - the private keys a purge deletes
  or a category move carries - and what a change does outside the home is listed and bound to the confirmation.
  The flat `ssh_config` and the Termius CSV download from the hosts page and write nothing, so `--read-only`
  keeps them.
- **Deploy, rotate and roll back from the web UI, as jobs.** A key's page reviews each one, then runs it with
  its output streamed as it comes. One job runs at a time, holds the lock until it ends, and is recorded in the
  history. Rotate pauses on a target that takes the key by hand, with the new public key to copy, and waits
  for you. Cancel stops a job between steps. A profile's keys, or every key, rotate as one job, and an encrypted
  bundle in `dist/` restores as one, with the identity from `SSH_MANAGER_AGE_IDENTITY_FILE`.
- **Snapshots restore from the web UI.** A snapshot's page previews the restore as a diff against `~/.ssh` now,
  and confirming snapshots `~/.ssh` first, as `sshmgr snapshots restore` does. A snapshot that still holds
  private keys is refused.
- **`sshmgr serve` (alias `ui`) - a web UI on this machine.** An overview built from the same rows the
  menu's banner prints, the profiles, hosts (with the rendered block and the pinned host keys) and keys, the
  snapshots with a diff against `~/.ssh` or another snapshot, the operation history with each operation's
  manifest diff, and the doctor and audit reports. It listens on `127.0.0.1:7822`, signs a browser in with a
  one-time token in the printed address, answers only its own Host names, and sends a strict CSP. A snapshot
  that still holds private keys is listed but never opened. It takes no lock, so it never holds up a command.
  Tabler 1.7.1 is embedded; `THIRD_PARTY_NOTICES.md` ships in every archive.

- **`sshmgr history` - every change sshmgr has made.** A SQLite store at `log/history.db` records one operation
  per command that takes the lock: the command and its arguments (names only; other values read `<set>`), how it
  ended, the snapshot it took, the audit events it raised, and the manifest before and after. `--host`,
  `--profile`, `--key`, `--since`, `--until`, `--command`, `--source`, `--limit` and `--json` filter it, and
  `history show <op_id>` diffs the manifest across one operation. The first run imports `audit.log`, the
  inventory's dates and the snapshots already on disk. `audit.log` is written as before; `SSH_MANAGER_HISTORY=0`
  turns the store off. The binary now links `modernc.org/sqlite`, a pure-Go driver, so every release target still
  builds with `CGO_ENABLED=0`.

- **`key_comment` - choose the comment new keys carry.** A template on `defaults` or on a profile, with
  `{profile}`, `{alias}`, `{key}`, `{date}` and `{device}`; unset keeps `{profile}/{alias} {date}`, the comment
  every earlier release wrote. `profile add|edit --key-comment` sets it (`""` clears it on edit), and the manifest
  schema lists it. It applies to keys minted from then on; an unknown placeholder is refused on load. `show
  <profile>` names it when a profile uses its own.
- **`profile add --key-scope` and `profile edit --shared`**, so both commands take both spellings: `--shared` is
  shorthand for `--key-scope shared` (`--shared=false` for `per_service`), and the two are refused together only
  when they disagree.

##### Changed

- **`doctor` checks token-file modes.** A credential file others can read is a warning, so a machine that
  passed before still passes; `doctor --strict` (as in CI) fails on it, and `--fix` tightens it.
- **Machine-readable output is unchanged.** `doctor --json` keeps every key, in the same order, and only
  gains a `tokens` member at the end; `history --json` records the same bare command names (`host add`,
  `reconcile`), never the `authama ssh` spelling. A test pins both shapes. Only human-readable messages use
  the new spellings.
- **`ui` is deprecated.** It was an alias of `serve`; it still starts the web UI, hidden from help, with a
  notice naming `serve` (web) and `tui` (terminal), since `ui` no longer says which.
- **Messages teach the new spellings** (`authama ssh deploy …`), so scripts that parse message *text* (not
  `--json`) may need updating.
- **`install.ps1` is checked by PSScriptAnalyzer in CI** as well as parsed.
- **Commands no longer hold the lock while they wait for your answer.** `reconcile`, `keygen`, `key delete`,
  `key regen-pub`, `host apply`, `clean` and `import` used to take the lock first and keep it while the question sat
  on screen, so every other sshmgr command waited for as long as you took. They now ask first, then take the lock
  and work out again what they showed you; if it moved meanwhile - another command changed the manifest - they stop
  with "something changed while you were deciding" and change nothing. With `--yes` or no terminal, nothing asks,
  and they lock first exactly as before. `keygen --force` now writes its key backup under the lock, after the
  questions rather than between them.
- **`host apply` asks before changing an existing profile.** It applied a batch to an existing profile without
  asking, unlike every other manifest edit; it now shows the batch and asks, on a terminal. `--yes` and scripts are
  unaffected.
- **A deploy with nobody to answer cannot prompt (W12).** Under `sshmgr serve`, `ssh-copy-id` and the ssh
  fallback run with stdin closed and `BatchMode=yes`, and their output goes to the job's page. A target that
  needs a password fails at once instead of prompting in the terminal `serve` was started from, or hanging for
  two minutes. The CLI's deploy keeps its prompt.
- **The menu no longer holds the lock for the rest of the session.** A `sshmgr` menu took the lock with its first
  change and kept it until it quit, so an open menu held up every other sshmgr command. Each change now releases it
  when it ends. Reconcile from the menu also asks before it takes the lock, and stops with "something changed
  while you were deciding" if the manifest moved meanwhile.

- **`config render`, `doctor --fix`, `providers --export`, `bundle`, `git use`, `vm docker-context`, `init`,
  `migrate` and `notify install` take the advisory lock.** Each changes state and none was serialized with the
  other commands; `config render` now also re-reads the manifest after its confirmation, under the lock.
- **A snapshot that fails is on the record.** The guard used to drop the error; the operation now carries a
  `snapshot.failed` event with the reason. The command still runs.

- **`termius keys ALIAS` finds a git host without `--include-git`.** Naming the host is the choice; the flag now
  only widens the list. `--skip` still leaves a named host out.
- **Restore checks a key in a rendered slot by parsing it.** It used to lay down any file that began `ssh-` or
  `-----BEGIN `; it now reads the public key's wire format, or the private key's PEM block and nothing after it
  (an OpenSSH key through its clear public half, so a passphrase-protected key still passes).

##### Fixed

- **A bundle could overwrite another written in the same second.** Bundles were named for the second they were
  written in, and the second truncated the first. A `bundle` followed by a purge's backup, or a purge followed by a
  rotation, could lose the backup holding the only copy of the keys the purge deleted. A bundle is now created
  exclusively, and a name already taken gets the next `-N` suffix.
- **`rotate` in dev mode wrote a key backup, then refused.** It now refuses first, in the CLI, the menu and the web
  UI, so a sandbox run leaves no bundle behind. With no recipient set it says why it can never run, not that the
  backup is missing.

- **Rotation keeps a key's declared type.** A key declared under a profile's `keys` with its own `type` (say
  `ecdsa`) came back from `rotate` as `defaults.key_type` (`ed25519`) under its `ecdsa` name, and its inventory
  record said `ed25519`. Rotation now mints, and records, the type `reconcile` used.
- **A hardware key that fell back to software is reported and recorded as software.** With no security key
  answering, an `ed25519-sk` key is minted as `ed25519` (its comment ends `(sk-fallback)`); the inventory still said
  `ed25519-sk` and nothing was printed, so the key looked hardware-bound. The record now names the type minted, and
  `reconcile`, `rotate` and the TUI print a line saying the key is software and how to replace it.
- **A rotated key's inventory comment matches the key.** The record said `profile/<key name> date` while the
  new `.pub` said `profile/<alias> date`; both now come from the same template. A 1Password key's rotation record
  uses the same comment its first record had, rather than `profile/<key name> date`.
- **The TUI's Change entries apply the same guards as their CLI commands.** Rotate skipped the encrypted key
  backup `sshmgr rotate` requires, so a TUI rotation could discard the previous key with nothing to restore it
  from; it now writes the backup first and stops without `SSH_MANAGER_AGE_RECIPIENT`. Pin wrote without asking,
  under a heading promising every entry asks first; it now asks, and is listed as `knownhosts init --all`, the
  command it runs (`knownhosts pin` still selects it). Deploy neither asked nor snapshotted, and ran in dev mode;
  it now does all three as `deploy` does. Reconcile read the manifest before taking the lock, pinned only when it
  minted a key, and skipped the unused-key warning; it now matches `reconcile`.
- **`snapshots restore` and `snapshots prune` take the advisory lock**, in the CLI and the TUI, so neither can run
  under another sshmgr command writing the same tree.

#### ssh-manager 3.5.0 - 2026-10-26 ([tree 05d3fa6](https://github.com/simtabi/authama/tree/05d3fa6e03f61e0312f26321d3d62a92fe152698))

##### Added

- **`sshmgr termius keys [ALIAS]` - get each key into Termius.** Termius's CSV import has no key column, so imported
  hosts stop at SSH key selection. With no alias it lists every exported host with its Termius key label (the key
  file's name) and private key path; with an alias it copies that host's private key to the clipboard (pbcopy,
  wl-copy/xclip/xsel, or clip) and prints the label to paste into Keychain → New Key. The key is never printed unless
  `--print` is given (with a stderr warning); `--clear` and `--clear-after DUR` empty the clipboard; `--include-git`
  and `--skip` select hosts as on `termius export`. `termius export`
  now points to it, and the export docs and Termius recipe describe the full flow with Termius's docs as sources.
- **`sshmgr host apply` - add or update hosts from compact specs.** A spec is
  `<alias> <host>[:port] <user> [role]`, e.g.
  `sshmgr host apply clientco web-01 203.0.113.21 ploi web`. Give
  one positionally, many with repeated `--host "..."`, or a file with
  `-f FILE` (`-` for stdin; one spec per line with `#` comments, or a JSON
  array of spec strings or `{alias,host,user,port,role,tags}` objects). A new
  alias is added the way `host add` adds it; an existing one has its hostname,
  user and port updated the way `host edit` does, and the role and any `--tag`
  are merged into its tags (never removed). Each host is reported added,
  updated (with what moved) or unchanged, so re-running the same specs is a
  no-op. The batch passes the manifest's validation and is saved at once, so
  one bad spec changes nothing. `--create-profile`/`--category` create a
  missing profile (and are undone if the batch is refused, so no empty
  profile is left behind), `--reconcile` mints the new keys afterwards, and
  `--dry-run`/`-y` behave as on `reconcile`.

- **`sshmgr host edit --tag` and `--untag` - change a host's tags.** Tags could be set by `host add` and merged
  by `host apply`, but nothing took one off. `--tag` (repeatable) now replaces the host's tags, `--tag ''` removes
  them all, and `--untag` (repeatable) removes one after any `--tag`; a tag the host lacks is ignored. Without either
  flag, `host edit` leaves tags alone, as before.
- **`doctor` reports the tools the manifest relies on.** A VM host whose hypervisor CLI is not on PATH, and `git`
  missing once `defaults.git` is set, are listed as `MISSING tool` and fail the verdict (JSON `needed_tools`): both
  stop a connection or a `sshmgr git` command that the rest of the report calls healthy. A missing clipboard tool is
  reported as advice (JSON `clipboard`), since `termius keys --print` works without one.

##### Fixed

- **An alias, hostname or user that ssh refuses is rejected when it is added.** `host add`, `host apply` and
  `import` accepted shell metacharacters such as `;`, `&`, `|`, `$` or backticks, which ssh refuses at connect time
  ("hostname contains invalid characters"). The host could never connect, and `config check` failed for every host
  because `ssh -G` stopped on it. Validation now refuses the characters OpenSSH's own `valid_hostname` refuses in the
  alias and hostname, and those `valid_ruser` refuses in the user (`$` and `\` stay allowed there, for `DOMAIN\user`).
  A manifest that already holds such a value no longer loads, so the error now names the file, the profile and the
  host to fix, and `import` skips a refused host and reports it instead of refusing the whole config.
- **`restore` reports a config it could not re-render.** The bundle no longer carries the rendered files, so a
  restore whose manifest failed to load left `~/.ssh` without its category configs while reporting success. It now
  exits non-zero with the reason after listing what it restored.
- **`--help` no longer shows a backticked phrase as a flag's value name.** pflag reads the first backticked word of a
  usage line as the value's name, so `--provider`, `--proxy`, `--vm` and `termius keys --include-git` printed as
  e.g. `--provider sshmgr providers`. A test now walks every flag.
- **`reconcile` no longer claims to fix permissions it did not change.** The
  fixer counted every managed path it visited, so an already-correct tree
  printed `perms fixed on 53 paths` on every run, indistinguishable from real
  drift. It now counts only paths whose mode actually changed and prints
  `perms: ok` when there were none. Nothing was resetting modes between runs.
- **The git ssh mode `Host github.com` block is rendered once, with its
  profile.** When `defaults.git.profile` is filed under a category, the block
  now goes into that category's `profiles/<category>/config` instead of the
  root `~/.ssh/config`. The category Include still precedes `Host *`, so ssh
  resolves `github.com` exactly as before. It is also skipped when a host is
  already aliased `github.com`.
- **`config check` reports duplicate Host blocks.** A `Host` pattern the
  managed files define more than once is listed as `DUPLICATE` (ssh only reads
  the first) and the config is not reported in sync.
- **OrbStack's Include is moved to the top.** OrbStack appends
  `Include ~/.orbstack/ssh/config` to the end of `~/.ssh/config` and says it
  only works at the top; a render now moves that line (with its comments) to
  the top of the file. No other foreign content moves.
- **Releases carry their notes again.** `changelog: disable: true` made
  GoReleaser skip the pipe that reads `--release-notes`, so v3.2.0 and v3.4.0
  were published with empty bodies while the run reported success. The config
  no longer disables it, the release workflow sets the body from the tagged
  CHANGELOG section after GoReleaser as a backstop, and a test fails if either
  regresses.

#### ssh-manager 3.4.0 - 2026-10-24 ([tree 087ead1](https://github.com/simtabi/authama/tree/087ead11e86b96056bbd029daf412459dfe68688))

3.3.0 was never tagged; its changes (git remote modes, devboxes) ship in this release.

##### Added

- **Cloudflare Access hosts.** `sshmgr host add|edit ... --proxy cloudflared`
  (manifest `proxy: {kind: cloudflared}`) renders
  `ProxyCommand "<cloudflared>" access ssh --hostname %h`, so `ssh <alias>`
  reaches a server through Cloudflare Access with no SSH port open. `doctor`
  reports a proxied host when `cloudflared` is not installed; the Termius export
  skips it, since Termius ignores ProxyCommand.
- **Exports for other tools.** `sshmgr config export --flat` writes the whole
  config as one file, every host inline with absolute key paths and no
  `Include` or `Match`, for a tool that reads a single `ssh_config`.
  `sshmgr termius export` writes a Termius CSV import (group = profile,
  label = alias, tags, address, port, user; never a password) and lists the key
  to attach to each host; `--open` opens Termius and shows the file on macOS,
  `--skip` leaves a host out, and VM and git hosts are left out and reported.
  Both write under `~/.config/ssh-manager/exports/` (0600) and refuse any target
  inside `~/.ssh` (checked by inode, so case and symlinks cannot get round it),
  so the split config, keys and trust store are never changed.
  Termius's own importer reads only the top-level `~/.ssh/config` and never
  saw the hosts in category files.
- **Git remote modes.** `sshmgr git status` shows how each checkout's GitHub
  origin authenticates; `sshmgr git use profile|ssh|https [PATH...]` switches it:
  the profile's own alias (`git@github-<profile>:owner/repo.git`, the default),
  plain `git@github.com` through a rendered `Host github.com`, or HTTPS through
  your credential helper. The mode comes from the argument, `--mode`,
  `SSHMGR_GIT_MODE`, then `defaults.git.mode`; the owner picks the profile through
  `defaults.git.orgs`, then `defaults.git.profile`, and several candidates are an
  error rather than a guess. Each new URL must pass `git ls-remote` before origin
  is rewritten; `--dry-run` changes nothing.
- **Devboxes.** A host with `vm: {hypervisor, name}` is a local VM:
  its block renders `ProxyCommand "<sshmgr>" vm proxy <alias>`, which asks
  Parallels, VMPal, VMware, UTM or Lima for the VM's current address on each
  connection and starts a stopped VM. `sshmgr vm list` shows every installed
  hypervisor's VMs (OrbStack and Podman included, which keep their own SSH and
  keys), `vm add` adds one as a host, and `vm docker-context` creates an
  `ssh://<alias>` Docker context.
- `doctor` fails when a rendered VM ProxyCommand names an sshmgr binary that no
  longer exists (`stale_proxy` in `--json`); `config render` rewrites it.

##### Fixed

- **Audit of 2026-10-21: security.** Dev mode no longer lets `rotate`,
  `rollback`, the TUI deploy or `--revoke` reach real servers, and dev-mode
  `migrate` no longer moves the real `~/.sshmgr` into the sandbox. `restore`
  lays down only the members `bundle` writes, so a forged bundle cannot replace
  `authorized_keys` or `config`. Option keys must be bare ssh keywords
  (`ProxyCommand=x` and `"ProxyCommand "` bypassed the dangerous-option check)
  and `XAuthLocation` is refused. Caller headers are dropped on a cross-origin
  redirect, and a pagination link that leaves the API origin is refused rather
  than sent the token. Import splits `Keyword=value`, so `ProxyCommand=...`
  is dropped like the plain form.
- **Audit of 2026-10-21: correctness.** A rollback checks every server before
  moving a key and swaps the pair into `old/`, so an unreachable target no
  longer destroys the rotated-in key, and an incomplete rollback exits non-zero.
  Mutating commands take the lock before reading state, so concurrent runs no
  longer drop each other's inventory records. Hostnames match known_hosts
  case-insensitively (`clean` had pruned a live pin) and `@revoked` lines no
  longer count as pinned. Aliases, hostnames and users that ssh reads as syntax
  (`"`, `'`, `#`, `!`, `,`, `=`) are refused, and `config check` fails when ssh
  cannot parse the config. Import resolves hosts the way ssh does: first value
  wins, `Host *` applies, quotes are removed, and the same alias in two
  profiles imports once instead of saving an invalid manifest. 1Password
  rollback works after a second vault rotation. Keys minted before a failed
  mint, keys on disk with no record, and the key `keygen --force` replaces all
  keep their inventory records; `deploy` records the manifest's key type and
  rotation. `rotate <profile|alias>` works, a shared profile needs a key name,
  and an unknown command is reported ahead of its flags.
- **Audit of 2026-10-21: what ssh would not use.** `config check` and `doctor`
  report a hand-written Host block above the managed block that shadows a
  managed alias (ssh reads it first), instead of calling the config in sync;
  sshmgr still never edits content outside its block. A raw option that repeats
  a line sshmgr writes (HostName, User, Port, UserKnownHostsFile) is no longer
  rendered as a second, ignored line, and `doctor` names the field to use. A
  `snapshots restore` keeps a `.pub` that would not match the private key on
  disk, says so, and now works in dev mode.
- **Bundles no longer carry rendered config.** `bundle` leaves out the
  `profiles/<category>/config` files and legacy `profiles/*/known_hosts` that
  sshmgr renders, and `restore` skips them in older bundles (naming each) and
  re-renders from the restored manifest. Before, a forged bundle could lay down
  a category config with a ProxyCommand, which `~/.ssh/config` Includes. A key
  that happens to be named `config` is still bundled and restored.
- **Hosting panel targets no longer force an unverified rotation.** A
  `ploi`, `forge` or `cpanel` target only accepts a pasted key, and `rotate`
  either aborted or, with `--allow-unverified`, committed before the key was
  pasted, so SSH to that server failed until it was. At a terminal, `rotate` now
  shows the staged public key and where it goes, waits for the paste, and logs
  in with the staged key; only a working login counts as verified. The committed
  report says where the old key still has to be removed. Without a terminal,
  nothing changes.
- `knownhosts pin` reported `pinned 0 host key(s)` and exited 0 for a host that
  returned no SSH key (a closed port, or a web server on 443). It now names each
  such host with the reason, pins the rest, and exits non-zero; a missing
  `ssh-keyscan` is reported too.
- The manifest JSON schema rejected `category`, `key_store`, `op_vault`, `keys`
  and `onepassword_agent`, which the code accepts; it now lists every field, and
  a test compares the two.
- The `knownhosts` docs and feature catalog still described per-profile trust
  stores at mode 0644; there is one hashed store at 0600.

#### ssh-manager 3.2.0 - 2026-10-19 ([tree b7169cc](https://github.com/simtabi/authama/tree/b7169cc91f6e8e54810ded30d2ad3707207aafca))

##### Added

- **Keys can live in 1Password.** A profile with `"key_store": "1password"` and
  an `op_vault` keeps its private keys as SSH Key items in that vault, created
  with the `op` CLI; only the public halves are written to disk. Its hosts render
  `IdentityFile <key>.pub` and `IdentityAgent` pointing at the 1Password agent.
  `rotate` stages the replacement in the vault, verifies it through the agent,
  and keeps one `<key>.old` predecessor; `rollback` restores it. Items are
  archived, never deleted. Rotating a key after switching its profile to
  `1password` moves it into the vault. `validate` checks the item matches the
  `.pub`, and `doctor` checks for `op`, the agent socket, and that the agent
  serves each vault (`agent.toml`). Two 1Password profiles may not keep the same
  key name in one vault, and 1Password keys must be `ed25519`.
- **Batch rotation.** `rotate` accepts several keys, profiles or host aliases,
  `--category <name>`, or `--all`, with `--dry-run` to list the batch. One
  confirmation, backup and snapshot; each key rotates on its own, so one failure
  leaves only that key as it was.
- **`key regen-pub`** rebuilds `.pub` files from the private key, or from the
  vault item for a 1Password profile, for the same selections. It reports each
  key as unchanged, created or rewritten, and never touches a private key.
- **`op://` secret references.** A provider token whose value starts `op://` is
  read with `op read`, beside the existing `cmd:` form.
- **Profiles can be grouped into categories.** A profile with `"category":
  "clients"` keeps its keys under `profiles/clients/<profile>/` and renders its
  `Host` blocks into `profiles/clients/config`, which the root config Includes
  under `Match all`. `profile add` and `profile edit` take `--category`; the edit
  moves the key directory and re-points inventory records, or only the records
  when the directory was already moved by hand. Profiles without a category, and
  every existing manifest, render exactly as before.

##### Fixed

- **The install command could not fetch its script.**
  `https://opensource.simtabi.com/install/ssh-manager` — the URL in the README,
  the docs and the installers' own usage lines — returns 404, and always has;
  that portal subtree was never published. Both install commands now point at a
  URL that resolves, and a test asserts every documented raw URL names a file
  this repository actually ships. The install scripts are also uploaded as
  release assets, so a future release carries them alongside the binaries.
- **`curl … | bash` failed outright on a stock Mac.** The installer expanded an
  empty array under `set -u`, which bash 3.2 — still what macOS ships as
  `/bin/bash`, and what `#!/usr/bin/env bash` finds there — rejects as an unbound
  variable. It died before downloading anything, for everyone without
  `GITHUB_TOKEN` set (which made the array non-empty and hid it). A second bug in
  the same six lines piped `curl` into `grep -m1`: grep closes the pipe on its
  first match, curl exits 56, and `pipefail` turns that into a failed install.
  CI now runs the installer on every non-Windows runner, so the macOS job
  exercises bash 3.2 — neither bug is a syntax error, so shellcheck saw neither.
- The PowerShell installer matched its checksum line by unanchored substring.
  `checksums.txt` lists a binary, its archive and the archive's SBOM, so a bare
  name matches three lines and `-First 1` took whichever came first. Safe for the
  `.exe` names it asks for and nothing else, which is not a property to rest a
  checksum gate on; it is anchored on the filename field now, like the shell one.

##### Removed

- **Every remaining reference to the previous implementation's language.** No
  source file mentions it, and neither do the docs, the test names or the tag
  names. Comments that compared behaviour to it now describe the behaviour —
  "the spelling v1 wrote for a true value" rather than a foreign function name —
  so they can be read without knowing what came before.

- **The last interpreter dependency.** `pre-commit` needs a runtime this project
  does not otherwise use, so CI carried a language-setup step and a job of its own
  to run four rules about whitespace. Its checks are kept, not dropped: `gitleaks` already ran as a
  separate job over the full history (which the hook could not do), `gofmt` and
  `go vet` are already in `make ci`, and the file-hygiene rules — trailing
  whitespace, final newline, LF endings, merge markers, oversized files, JSON
  validity — are now Go tests, so they run in `make check` too rather than only
  for people who had installed pre-commit. The interpreter-related entries in `.gitignore`,
  `.editorconfig` and `dependabot.yml` went with it.

##### Changed

- **GitHub verify checks the account.** When a deploy's API token and an ssh
  login are both available, GitHub must greet the key as the account the token
  belongs to. A token for the wrong account (a personal token left in a machine
  user's variable) used to add the key there and pass verification.
- **`reconcile` refuses to mint a key that only changed directory.** A declared
  key found in the other layout (flat when the manifest says categorized, or the
  reverse) used to be planned as a fresh key, leaving two keys for one identity.
  It now stops and names where the key is. `doctor` reports such a key as
  untracked at its real path, and its missing finding no longer advises
  `reconcile`.
- **`config render` prunes only `profiles/*/config` files it owns:** those with
  the managed header, or in a flat profile directory (the v1 per-profile slot).
  A hand-written category file beside profile directories is left alone.
- Comments citing the previous implementation's source files are gone. They named
  paths that exist nowhere, and the ones that were resolvable required knowing a
  tag to resolve them. What each test asserts is now stated in the test.

#### ssh-manager 3.1.1 - 2026-08-06 ([tree 3b9cfe0](https://github.com/simtabi/authama/tree/3b9cfe0930a2ea80d0ab3c2aa4b0875b268d143c))

##### Security

- **Restoring a snapshot or a bundle can no longer write outside its
  destination.** Both extractors validated member names and then checked that
  the joined path was textually inside the target — which a symlink already
  present in that target defeats, because the escape happens when the OS
  resolves the path rather than in how it is spelled. An archive with nothing
  but ordinary relative names could overwrite a file anywhere the user could
  write. Both now confine every write with `os.Root`, so containment is enforced
  by the kernel on the resolved path; the name validation stays, for a better
  error than a refused syscall. Reported by CodeQL as `go/zipslip` once the Go
  code was being scanned, and confirmed by a test that fails against the
  previous implementation.

#### ssh-manager 3.1.0 - 2026-08-06 ([tree 6c0098b](https://github.com/simtabi/authama/tree/6c0098b46f7ed5fdb2fa07396cc24ea138439275))

##### Added


- **The interactive menu opens with a banner.** It names the binary and build
  (version, platform, commit, date), the config home and `~/.ssh` it will
  operate on, what it found there (profiles, hosts, keys), and the state of the
  tree: agent, config drift, `known_hosts` coverage, permissions, dangling keys
  and rotations due. Both locations are environment-overridable, so without them
  a session pointed at a sandbox and one pointed at a real tree were
  indistinguishable. Rows marked `!` carry the command that fixes them, and the
  banner is redrawn after any action that changes `~/.ssh`.
- **Dev mode: `--dev-root <dir>` (or `$SSHMGR_DEV_ROOT`) runs the whole tool
  against a scratch directory** instead of `~/.ssh`, so every verb that mints,
  renders or pins can be exercised without touching the tree you use. Previously
  impossible: the config home had `$SSH_MANAGER_HOME` but `~/.ssh` had no
  override at all, so the only option was moving `$HOME` — which also moves the
  ssh-agent socket and the launchd session. One root holding `ssh/` and
  `config/`, not two independent overrides, because the half-applied case looks
  sandboxed until it overwrites a real key. Announced on stderr by every command
  and led with in the TUI banner; `deploy` and `notify install` are refused,
  since neither takes effect inside a directory.
- `make ci-linux` runs the CI gate inside a Linux container on the Go version
  `go.mod` asks for. `make check` only tests the machine you are on and
  `lint-all` cross-compiles without running anything, so a Linux-only failure was
  invisible until CI reported it — and unreachable when CI could not.
- A project icon at `docs/assets/sshmgr.svg` — a single scalable mark for the
  docs site, the repository's social preview and a favicon.

##### Changed

- **`make build` empties `build/` first, every time.** A stale binary is not
  hypothetical: a bug was reported against this tool from a binary built one
  minute before the fix landed, and it was indistinguishable from the fix not
  working. `make clean` now removes everything in `build/` except the files git
  tracks there, rather than the two paths it happened to know about.
- **The Go module moved to `src/`.** The repository now keeps its Go code under
  `src/` and its own files — README, LICENSE, CHANGELOG, `docs/`, `.github/` — at
  the root, with build output in a single root `build/`. Import paths become
  `github.com/simtabi/ssh-manager/src/v3/...`; `make` is unchanged and remains
  the supported way to build.
- **Each release is now tagged twice on the same commit.** `vX.Y.Z` drives the
  release as before; `src/vX.Y.Z` is how Go resolves a module that lives in a
  subdirectory, and the release workflow creates it, so you still tag once.
- **Every verb that changes `~/.ssh` now confirms first.** `reconcile`, `keygen`,
  `import`, `deploy`, `clean`, `config render`, `knownhosts init`, `migrate` and
  `notify install` ask before acting; the deletes and `rotate` already did.
  `reconcile` shows its dry run as the question. Each takes `--yes`/`-y`.
  The prompt appears only when standard input is a terminal — a script, cron job
  or CI step proceeds as before, because there is nobody to ask and declining on
  its own would be a silent no-op rather than a safeguard.
- **The interactive menu is grouped by consequence.** Ten flat entries put
  *Reconcile* one keystroke from *Expiry status* with nothing to say which of
  them wrote to disk; they are now under *Inspect*, *Change `~/.ssh`* and
  *Recover*. Each entry shows the CLI verb that does the same thing, and that
  verb is an accepted answer — `5`, `reconcile` and the full label all work.
  An unrecognised answer re-prompts instead of ending the session, and `q`
  quits.
- **A bare `sshmgr` opens the interactive menu** when standard input is a
  terminal, instead of printing help. With any argument it is a plain CLI, and
  without a terminal it still prints help — a menu reading from a pipe would
  take whatever is on it as answers.
- Build output is consolidated under `build/`: `make build` writes
  `build/sshmgr` instead of `bin/sshmgr`, and `build/dist/` holds the release
  artifacts as before. There is no `bin/` any more.

##### Fixed

- **A `go install` build no longer reports the wrong version.** It applies no
  ldflags, so the binary fell back to the hardcoded `2.0.0-dev` — two major
  versions stale, from v3 sources, on the install path the README documents.
  Go already records the real version inside the binary; it is now read from
  there, along with the commit, build time and a `-dirty` marker for an
  uncommitted tree. Such a build also has no VCS stamp, so the commit and time
  are recovered from the pseudo-version, which already ends in both — it no
  longer calls itself a "dev build" while holding a published version.
- **`go install` works again.** The module declared
  `github.com/simtabi/ssh-manager` while being tagged `v2.0.0` and `v3.0.0`, and
  Go requires a major-version suffix from v2 onward — so it rejected both tags
  and fell back to the newest one it would accept, `v0.1.0` from before the rewrite,
  which contains no `cmd/sshmgr`. The documented install command has been
  failing since v2.0.0. The module path is now
  `github.com/simtabi/ssh-manager/src/v3`.
- `scripts/build-all.sh` no longer rewrites the tracked release matrix when
  given a `TARGETS=` override — building one target while iterating would have
  shrunk the next release to that single target. Its build loop also read
  `targets.txt` rather than the matrix it had just selected, so the override was
  ignored in any case where the two differed.
- `make feature-check` ran 9 tests, not the 80 in the package, because its
  `-run 'TestCommandSurface|TestVerbs'` regex did not match
  `TestTheCommandSurfaceMatchesTheOneItReplaced` — the one test that pins the
  command surface against the implementation it replaced. A target
  documented as "exercise every command with assertions" was excluding the
  assertion that matters most. It now runs the package.
- The pre-commit `gofmt` hook can now fail. It ran `gofmt -l`, which lists
  unformatted files and exits 0, so it reported problems and passed.
- `make clean` now removes the release artifacts. It ran `rm -rf bin dist`,
  targeting a top-level `dist/` that GoReleaser stopped writing to when the
  build moved under `build/` — so it deleted the development binary, reported
  success, and left every packaged artifact in place.
- `scripts/build-all.sh` no longer deletes `build/targets.txt`, which is tracked,
  as a side effect of a release build.


#### ssh-manager 3.0.0 - 2026-08-06 ([tree a860264](https://github.com/simtabi/authama/tree/a860264c44ade2610d38e147491b360aa946cc5e))

The previous implementation is gone from the tree, the verification matrix is
closed, and the documentation describes what ships.

Major rather than minor for two reasons, both below under **Changed**: the
`~/.ssh` layout moved, and `knownhosts init --user` was removed. Neither needs
action beyond running `reconcile` once — but a script that passed `--user` will
fail, and that is what a major version is for.

##### Changed

- **`~/.ssh` layout.** One inline `config` holds every `Host` entry, grouped by a
  profile banner with `Host *` last, instead of a root config plus a per-profile
  file stitched together with `Include`. One hashed `known_hosts` replaces the
  per-profile trust stores; every line ssh-manager writes carries an `sshmgr`
  comment tag so it only ever prunes its own. `profiles/<name>/` now holds keys
  only. Identity isolation is unchanged and stronger: it comes from `IdentityFile`
  plus a **per-host** `IdentitiesOnly yes`, which the rendered config states
  explicitly for every host. Run `reconcile` once and the old files are migrated.
- `known_hosts` is mode 0600, not 0644. ssh has never needed the trust store
  world-readable, and its contents are an inventory of every host you connect to.
- A `key_name` may now repeat across profiles. v1 rejected that manifest; one
  person working under two organisations uses the same file name in both, so the
  ban prevented a normal setup. A key's identity is the pair `profile/key`.
- **`knownhosts init` no longer takes `--user`.** There is one store now, so
  there is nothing left for it to select — but the flag existed in 2.0.0, so a
  script that passes it will fail rather than being ignored. Drop it; `--all`
  covers every host in the manifest, which is what `--user` aggregated.

##### Added

- `key add`, `key list`, `key delete` — declare, mint and remove a key
  independently of the host that uses it, with a guarded delete that refuses
  while any host still resolves to it.
- `show <selector>` — reconciles the manifest, the key files, the rendered config
  and the trust store in one view, because an auth failure is usually a
  disagreement between them.
- `clean [--dry-run] [--adopt]` — prunes stale pins and inventory records left
  behind by deletions.
- `doctor --strict` — escalates every dangling-key state to fatal, for CI.
- `--yes` on the destructive verbs, so they are usable from a script, and
  `--no-key-backup` to accept an overwrite that cannot be undone.
- `SSH_MANAGER_OLD_KEY_MAX_AGE_DAYS` — when an archived predecessor is called stale.

##### Fixed

- **Rotation could lose a key on a crash.** Commit renamed four files with no
  rollback; an interruption between them left the tree without a usable pair.
  Rollback was worse: it deleted the live pair before moving the predecessor in.
  Both now preserve by hard link and rename over atomically.
- **`keygen --force` destroyed same-named keys in other profiles.** The overwrite
  set was keyed by bare name, so confirming one key regenerated every key that
  shared its name — silently invalidating identities the user was never asked
  about.
- **`init --force` could reset the manifest while reporting a backup it never
  wrote.** Every error from the backup copy was swallowed. A failed backup now
  stops the reset.
- **`migrate` flattened symlinks into world-readable copies** when the legacy
  home and the new one were on different filesystems. Symlinking a config file
  out to a dotfiles repo is ordinary; the migration left the user editing a stale
  copy.
- **A locked password store stayed locked.** `cmd:` secret lookups memoised
  failures, so unlocking and retrying returned the same empty token for the life
  of the process — invisible in a one-shot run, permanent in the TUI.
- **The TUI froze on the second mutating action of a session.** The advisory lock
  is per file descriptor, so a second acquire in one process waited forever on a
  lock that process already held.
- **A crafted `authorized_keys` line could panic the tool** on 32-bit builds
  (386, armv6, armv7 all ship): an attacker-controlled length prefix was bounds-
  checked in `int`, where it wraps.
- `sshmgr view` could pair one inventory record's fingerprint with another's
  status, expiry and deployments when two records pointed at the same key path.
- `sshmgr diff` counted a shared key once per host that used it.
- `import` reported more hosts than it wrote when a config repeated a `Host` block
  — which is what appending to a config rather than editing it produces.
- `sshmgr clean` dropped the inventory record of a key that had merely become
  unwired, leaving the key on disk with nothing tracking its expiry.
- `manifest.json` and `inventory.json` claimed atomic persistence while using a
  truncating write, and kept whatever mode the file already had — so a manifest
  loosened once stayed loose through every later save.
- The TUI menu, the passphrase prompt and `notify test`'s diagnostic reached past
  the command to the process's own streams, so a caller that redirected them was
  ignored.
- `host edit`'s flags, and `--provider`, `--token-env` and `--key-name` on
  `host add`, had no help text at all.

##### Security

- Passphrases reach `ssh-keygen` through the askpass protocol rather than the
  command line, where any local user could read them from the process list.
- `authorized_keys` parsing rejects an oversized wire-format length prefix
  instead of trusting it.
- The shipped example `config/manifest.json` no longer contains real
  infrastructure. It held three live addresses, a university host with its login
  name, and the VPN endpoint fronting it — a target list, published as
  documentation.
- CodeQL scans Go. It was configured for the wrong language, so the Go code had never been
  analysed.

##### Removed

- The v1 implementation and its test suite, preserved at the `v1-final`
  tag. The binary has had no interpreter since 2.0.0; this removes the parity
  reference that was still in the tree.
- The `.build/` shell harnesses. Their assertions are Go tests now, so they run on
  every platform instead of macOS alone — and the last interpreter executing in CI
  went with them.

#### ssh-manager 2.0.0 - 2026-06-17 ([tree f9b67c6](https://github.com/simtabi/authama/tree/f9b67c65a7ee7de90afc1726c2fe1cd72357ba85))

ssh-manager is **rewritten in Go** as a single self-contained binary - no interpreter
runtime to install. Behavior is unchanged: the same `manifest.json` /
`inventory.json` / `providers.json` formats, the same `~/.ssh` layout and
managed-block markers, the same `sshmgr` command surface and `SSH_MANAGER_*`
environment variables. A v1 home works as-is.

##### Changed

- **Pure Go, no interpreter.** Every verb (init, import, migrate, reconcile, keygen,
  config, diff, validate, doctor, profile, host, providers, net, knownhosts,
  snapshots, bundle, restore, recover, load, deploy, rotate, rollback, list,
  view, expiry, audit, notify, tui) is native Go. The binary is a ~10 MB static
  executable with no embedded interpreter (was ~30-50 MB with a frozen interpreter).
- **More platforms.** Cross-compiles to macOS (Apple Silicon **and Intel**),
  Linux (amd64/arm64), and Windows (amd64) from a single build - Intel mac, which
  the bundled-interpreter build couldn't ship, is supported again.
- Provider key management (GitHub `gh`, GitLab `glab`, DigitalOcean / Vultr /
  Hetzner / Linode / Scaleway REST, and a config-driven generic REST), the age
  bundle/restore, the desktop notifier, and the scheduled job (launchd / systemd /
  cron / schtasks) are all reimplemented natively. Bundles and `~/.ssh` snapshots
  remain interoperable with v1.

##### Notes

- The list/view/expiry views now print plain text rather than the v1 `rich`
  tables; the data shown is the same.
- The v1 implementation remains available at the `v0.1.0` tag and as the
  parity reference in `src/`. (Since removed - see Unreleased. The reference is
  now the `v1-final` tag.)

#### ssh-manager 0.1.0 - 2026-06-16 ([tree 5ca46aa](https://github.com/simtabi/authama/tree/5ca46aabf620de5fee3a81087c5429811939c10c))

First public release. ssh-manager manages SSH keys and `~/.ssh/config` from a single
manifest - reproducible output, profile-based isolation, and safety guarantees
(atomic writes, advisory locking, snapshot-before-mutate, load-bearing perms).

##### Added

**Core lifecycle**

- **One per-user home in the OS-standard config location** holds all of a user's
  state - config (`manifest.json`, `inventory.json`, optional `providers.json`),
  secrets (`.env`, `age-identity.txt`, 0600), logs (`log/audit.log`), `snapshots/`,
  and transient `.state/`. The home is `$XDG_CONFIG_HOME/ssh-manager` (else
  `~/.config/ssh-manager`) on Linux/macOS and `%APPDATA%\ssh-manager` on Windows;
  `$SSH_MANAGER_HOME` (alias `$SSH_MANAGER_CONFIG_DIR`) overrides it, and a legacy `~/.sshmgr`
  is auto-migrated. No project-local `./config` mode. `~/.ssh` is unchanged.
- **Layered config, manifest-first.** The manifest is the source of truth, read
  from your home (`init` seeds an empty one). The provider catalog uses your
  `<home>/providers.json` if present, else the full default catalog shipped with
  the package (kept byte-identical to the repo `config/providers.json`) - so
  providers work out of the box and you only create your own file to customize.
- `init` creates and **converges** the home: every run (re)creates the directory
  structure + re-asserts perms and seeds missing files, without touching your
  existing config/secrets. `init --force` resets the seed files to defaults (in place); add `--backup` to
  copy the old ones into `<home>/.state/` first. `init` no longer seeds
  `providers.json` (the shipped catalog is the default); the `.env` template ships
  as package data.
- `migrate [--force]` - guided move of a legacy `~/.sshmgr` to the standard home
  for the stranded both-exist case (auto-migration handles the simple case).
- `doctor --json` - machine-readable report for scripting/monitoring; `doctor`
  also reports the active provider-catalog source and a stranded legacy home.
- **Secret-at-rest indirection.** Any provider `token_env` value may be
  `cmd:<command>` - the token is fetched at use-time from a secret manager
  (1Password, OS keyring, `age`, ...) instead of sitting in `.env` plaintext.
- `init`, `reconcile` (with `--dry-run`), `keygen` (with `--force` to overwrite
  existing keys - prompted, and snapshotted first), `config check/render/show`,
  `import` (onboard an existing `~/.ssh`), and `diff`.
- Profile-based layout: each identity's keys, rendered config, and its own
  `known_hosts` live under `~/.ssh/profiles/<profile>/`; `IdentitiesOnly yes` and
  a per-profile `UserKnownHostsFile` keep profiles from bleeding into each other.
- A single config renderer drives `render`, `check`, and `reconcile`; perms are
  load-bearing (700/600/644) and every state write is atomic under an advisory
  lock. Key generation never clobbers an existing private key unless you ask.

**Deploy & providers**

- `deploy` over a pluggable provider strategy (config-driven via the provider
  catalog - your `<home>/providers.json` or the shipped default): GitHub and GitLab (CLI + token, cloud and
  enterprise/self-hosted); Bitbucket, Gitea, Codeberg, Forgejo, Gogs, SourceHut,
  Azure DevOps, and AWS CodeCommit (web-panel); cloud-VPS account keys for
  DigitalOcean, Vultr, Hetzner, Linode, and Scaleway (REST); and `generic-ssh`
  (`ssh-copy-id` / hardened `authorized_keys` editing). Any REST key API can be
  added with no code via `kind: rest`. Without a token, providers degrade to
  printing the dashboard URL.
- `validate` checks every managed keypair (both keys parse, the public key is
  derived from the private and matches, perms are correct); `providers` lists the
  active catalog and whether each credential is set, and `providers --export`
  materializes an editable copy of the shipped catalog into the home.
- **Network/VPN awareness.** Hosts can be marked `requires_vpn` (with optional
  `vpn_name` + `vpn_url`). `net` shows per-host connection status + a VPN
  indicator, and `deploy`/`rotate` run a bounded SSH-level reachability probe
  first - so a host that's down or behind a disconnected VPN (including a `:443`
  host that accepts TCP but never speaks SSH) fails fast with "connect the VPN at
  <url> and retry" instead of hanging. Every `ssh` / `ssh-copy-id` call is also
  hard-timeout-bounded.
- `doctor` prints the resolved home (the OS-standard dir or `$SSH_MANAGER_HOME`) and `~/.ssh`,
  so it's always clear where state lives.

**Rotation, expiry, backup, recovery**

- `rotate` - zero-downtime, stage → deploy → verify → archive (at most one
  predecessor kept) - and `rollback`.
- `expiry` with an inline banner and a scheduled desktop notifier; `audit` of
  deployments, hygiene, and recent activity.
- `bundle` / `restore` - `age`-encrypted, off-machine backup with true same-key
  recovery; the `.env` is never included.
- `snapshots` - every mutating command snapshots `~/.ssh` first, so any run is
  reversible with one command.
- `recover` - a break-glass snippet (or full interactive tool) to paste into a
  provider console when you're locked out entirely.

**Editing, agent, trust, UI**

- `profile` and `host` add/edit/delete (deletes can revoke + prune); `load`
  (`ssh-add`, Keychain on macOS); `knownhosts init` (initialize trust stores -
  create the file + pin reachable hosts, with a per-store report - scoped to a
  profile, `--all` profiles, and/or `--user` for the per-user `~/.ssh/known_hosts`)
  and `knownhosts pin` (host-scoped, fingerprint-confirmed, via `ssh-keyscan`);
  and an interactive `tui`.
- `--yes`/`-y` on destructive verbs (plus `--revoke` on the deletes) for
  non-interactive / scripted use.

##### Platforms

- **macOS**, **Linux**, and **Windows** are all first-class. macOS/Linux: launchd /
  systemd-user-or-cron schedulers, Keychain / `notify-send`, `UseKeychain` on macOS
  only. Windows: `icacls` owner-only ACLs, `schtasks`, PowerShell toast - validated
  by real-binary tests **and a full reconcile/perms/config e2e** on the
  windows-latest CI runner.

##### Security & robustness

- **Manifest input is hardened.** Names that become filesystem paths (profile,
  alias, key_name) reject `/`, `\`, `..`, leading `-`, whitespace, glob `*?`, and
  control chars; hostname and user reject whitespace, newlines, and leading `-`;
  `raw_options`/`global_options` reject command/code-executing keys (`ProxyCommand`,
  `LocalCommand`, `Match`, `Include`, `KnownHostsCommand`, `PKCS11Provider`,
  `SecurityKeyProvider`). `key_scope` is enum-validated. Closes config-injection /
  path-traversal / wildcard-hijack from a hand-edited or imported manifest.
- **Corrupt inputs fail cleanly, not with a traceback** - corrupt inventory,
  snapshot, or bundle (and a wrong-shape `providers.json`) raise a clear error; this
  matters most on the recovery paths. `validate <unknown>` now errors instead of
  silently passing.
- **No hangs / truncation on the network path** - `gh`/`glab`/`ssh-keyscan` calls
  are timeout-bounded; Linode/Scaleway key listing errors rather than silently
  truncating; REST `kind: rest` requires `https://`.
- **Perms/secrets** - `manifest.json`/`inventory.json` written 0600; snapshot/restore
  refuse a symlinked `~/.ssh`; the audit log redacts secret-named fields; scheduled
  jobs handle executable paths containing spaces.

##### Fixed

- **Robust legacy-home migration.** The `~/.sshmgr` to standard-dir move is now
  serialized under an advisory lock with a re-check, and uses `os.rename` (which
  errors rather than nesting the legacy dir inside an existing destination) - fixing
  a TOCTOU race; a genuine failure is surfaced to stderr and flagged by `doctor`
  (which now also shows a stranded legacy home and the active provider-catalog source).
- **Consistent resolution + catalog perf.** `XDG_CONFIG_HOME`/`APPDATA` are read from
  the same env mapping as the `$SSH_MANAGER_HOME` override (no split between a passed env
  dict and `os.environ`); the provider catalog is memoized by path+mtime and `list
  --type`/category lookups honor the shipped-default fallback (no per-host re-parse).
- **Cross-profile `key_name` collision rejected.** Two profiles reusing one
  `key_name` (e.g. two `shared` profiles both named `deploy`) would make rotate/
  deploy mint into one profile's dir but act on the other's hosts (orphan/lockout).
  The manifest now rejects a `key_name` owned by more than one profile at load.
- **Per-profile trust on deploy/verify/rotate.** `ssh`/`ssh-copy-id` in deploy,
  login-verify, and rotation now use the host's per-profile `known_hosts`
  (`UserKnownHostsFile`), not the default `~/.ssh/known_hosts` - so a key is
  verified against the SAME store `ssh <alias>` uses, and TOFU lands the host key
  in the right per-profile store (isolation no longer bypassed by network ops).
- **Profile named `old` no longer hidden from expiry/audit.** Archived
  predecessors are detected by path structure (`profiles/<p>/old/<name>`) instead
  of a bare `/old/` substring, so a profile literally named `old` is tracked.
- **TUI** gained a "Pin host keys (known_hosts)" action (it had no way to set up
  trust, yet that's required for the first connection). `knownhosts init --force`
  is documented accurately (re-scan + add new keys; never silently replaces a
  changed host key).
- **GitLab adapter parity.** `gitlab` now lists keys via `glab api user/keys`,
  so deploy is idempotent (a key already on the account isn't re-added/errored)
  and `verify`/`remove` match by key body - rotation can verify and revoke GitLab
  keys instead of always aborting or leaving the old key live (mirrors the GitHub fix).
- **Robustness.** A manifest/inventory path that is a directory or unreadable now
  raises a clean error instead of a traceback; a `known_hosts` that is a directory
  no longer crashes `doctor`/auto-pin. `doctor` flags the same Host alias used in
  more than one profile (ssh applies the first match, silently shadowing the rest).
  `SSH_MANAGER_AUTO_PIN` accepts `0/false/no/off`; the auto-pin reachability probe is
  short (it runs under the lock); the HTTP redirect guard no longer drops the token
  on a same-host `:443`-explicit redirect; `known_hosts` `@cert-authority`/`@revoked`
  lines are recognised.
- **Auto-pin known_hosts on reconcile/keygen.** After minting keys, each profile's
  `known_hosts` is created/updated for the hosts it can reach (trust-on-first-use;
  never overrides an existing pin, skips unreachable/VPN-gated hosts). Disable with
  `--no-pin` or `SSH_MANAGER_AUTO_PIN=0`; `knownhosts pin` remains the fingerprint-verified
  path. This removes the "fresh reconcile -> git push fails host-key verification"
  footgun for reachable hosts.
- **Multi-identity guidance.** Documented that hosts sharing one server (e.g. two
  GitHub accounts on `github.com`) should use distinct, profile-prefixed **aliases**
  (`github-personal`, `github-simtabi`) to avoid collisions, and that the SSH `user`
  for GitHub/GitLab/Bitbucket must stay `git` (the key selects the account; a custom
  user breaks `git@host:` remotes). The shipped example manifest uses the prefixed form.
- **Unpinned host keys are now discoverable.** With per-profile
  `UserKnownHostsFile` and OpenSSH's default `StrictHostKeyChecking ask`, a host
  that hasn't been pinned fails non-interactive `git`/`ssh` with "Host key
  verification failed". `doctor` now lists each reachable manifest host lacking a
  pinned key with the remedy (`sshmgr knownhosts pin --all`), and `reconcile`/
  `keygen` print the pin+deploy steps after minting. Docs document the
  reconcile -> pin -> deploy first-use sequence.
- **`~/.ssh/config` foreign content is preserved.** `reconcile`/`config render`
  now rewrite only the marked ssh-manager block (`# Managed by ssh-manager ...` to
  `# End of ssh-manager-managed block ...`) and keep anything above/below it verbatim -
  so a tool-injected preamble such as OrbStack's top-of-file `Include`
  (which it won't re-add if removed) survives a re-render. `config check` no longer
  flags the preserved content as drift. One composer backs both write and check.
- **GitHub/GitLab enterprise & key removal.** `gh`/`glab` are driven by their
  environment (`GH_HOST`/`GH_ENTERPRISE_TOKEN`, `GITLAB_HOST`/`GITLAB_TOKEN`),
  not a nonexistent `--hostname` flag, so enterprise/self-hosted instances work;
  GitHub key listing/removal go through `gh api user/keys` and match by key
  **body** (never by title), so rotation can't revoke the freshly deployed key.
  GitHub deploy is now idempotent.
- **Shared-key delete.** Deleting one host that shares a key revokes it from
  *that* host's target only and keeps the inventory record (and per-host history)
  for the hosts that remain; deleting the profile revokes *every* host (previously
  only the first).
- **`deploy` exit code.** Exits non-zero on any failed deploy (provider/API error
  or unreachable host), not only "unreachable"; a manual/web-panel paste still
  exits 0. Errors and warnings now go to stderr.
- **`import` is non-destructive.** Refuses to replace a non-empty manifest unless
  `--force` (which backs up the previous manifest + inventory first); multi-pattern
  `Host` lines bind to all aliases, `Match` blocks terminate cleanly, and
  code-executing directives are dropped instead of aborting the import.
- **Rotation.** A rotated-in key inherits the requested passphrase
  (`rotate --passphrase`); an aborted rotation pulls the staged key back off any
  target it reached; repeated rotations no longer accumulate stale `/old/`
  inventory records; VPS key titles are body-stamped so providers that enforce
  unique names (Hetzner) accept the staged key.
- **REST / HTTP.** A generic `kind: rest` provider raises instead of reporting a
  phantom success when delete/rename aren't configured; the HTTP client converts
  read-phase transport errors to a clean error, strips credentials on cross-origin
  redirects, refuses an `https -> http` downgrade, and honors `Retry-After`.
- **Permissions.** Restored private keys and snapshot tarballs are created
  owner-only (no world/group-readable window); Windows `icacls` strips broad
  principals; the Windows scheduled task is quoted for `cmd.exe`.
- **Other.** `knownhosts pin <alias>` resolves the manifest hostname/port; a
  malformed date in a hand-edited inventory no longer crashes `expiry`/`audit`;
  the JSON schemas accept the nullable fields the tool actually writes; the
  package version has a single source (v1's package metadata); `--version` flag
  added.

##### Quality

- Typed throughout, linted, and covered by a unit
  suite plus an end-to-end smoke (`make e2e`). Output uses **rich**, prompts use
  one toolkit per concern.
- CI runs the suite on macOS / Linux (3.11-3.13) and the platform layer on
  Windows, plus CodeQL, secret scanning (gitleaks), and pre-commit. Releases
  build and attach artifacts to a GitHub Release on a `v*` tag.

### auth-token-manager

#### auth-token-manager 0.3.0 - 2026-10-13

##### Fixed
- **Piped output is no longer hard-wrapped at 80 columns.** `--env` exports and
  emitted JSON used to be broken across lines when stdout was a pipe, which
  corrupted `eval "$(auth-token-manager --env)"` and machine-read output. Shell
  exports and rendered file content now also bypass markup interpretation, so a
  token containing `[bracketed]` text survives verbatim.
- **`--template --project DIR` no longer touches the global credential files.**
  It now writes only the project-scoped targets (composer, npm) into the project
  dir; previously the docker/git/netrc placeholders were merged into
  `~/.docker/config.json`, `~/.git-credentials`, and `~/.netrc` — where they
  could *replace* real credentials for hosts like `github.com` and `ghcr.io`.
- **A malformed line in `.git-credentials` (e.g. a non-numeric port) no longer
  crashes `--list`/inventory** with a traceback. The line is listed under a
  best-effort host key and can still be `--remove`d.
- **Writing refuses to overwrite a file it could not read.** Previously, if an
  existing auth file was unreadable (permissions, I/O error), the merge treated
  it as absent and overwrote it — silently dropping every credential in it. Now
  it aborts with a clear error instead.
- **Revealed secrets render verbatim in tables.** Values containing `[...]` were
  interpreted as rich markup and silently altered in `--list --show-secrets` and
  the interactive list.
- `--doctor` now checks the `--project` dir's `.gitignore` (not the current
  directory's) when `--project` is given, and ignores commented-out lines.
- fish `--env` exports escape backslashes, so a value ending in `\` can't break
  the quoting.
- **cmd.exe `--env` exports are now safe for values containing quotes.** The
  quoted `set "NAME=value"` form breaks when the value itself embeds `"` (every
  `COMPOSER_AUTH` JSON does): each quote toggles cmd's parsing mid-line, exposing
  slices of the value to `& | < > ^` interpretation. Such values now use the
  unquoted form with every cmd-special caret-escaped, which parses
  deterministically; quote-free values keep the clean quoted form.
- `save_config` no longer leaves a stray `.tmp` file behind if writing fails;
  the interactive edit flow now surfaces file-hardening warnings.
- `make build` failed because the `build` package was never installed; the
  target now installs it into the venv first (and creates the venv if needed).

##### Changed
- `--target` now also filters `--list` and `--stdout`, not just which files are
  written. New optional `targets=` filter on `AuthTokenManager.to_json()`.
- **Bulk "env var not set" skips are summarised.** With a starter config (37
  example hosts), every render printed a 37-line warning wall; more than six
  such skips now collapse into one per-target summary line. Config *mistakes*
  (unknown service, unsupported type) still print verbatim.
- The dev container restarts across Docker daemon/VM restarts
  (`restart: unless-stopped`, configurable via `ATM_RESTART`).
- **Docs no longer call `rich`/`questionary` "optional".** They are required
  dependencies of the package (every pip/pipx/uv install includes them — the
  interactive manager is the default experience); the stdlib fallback exists for
  the no-install paths (zipapp, source checkout), where everything but `--ui`
  works. README, `docs/installation.md`, and a `pyproject.toml` comment now say
  exactly that.
- The release workflow now smoke-tests the standalone binary on **all three**
  OSes (previously Windows was skipped), and runs `--services` + `--list` so a
  build that silently drops the bundled catalog/templates fails the release.

##### Added
- **Fully containerized usage (Docker).** New `Dockerfile` (Ubuntu 24.04,
  non-root, minimal apt set) + `compose.yaml` + `.env.example`: commands run
  inside a container named after the project while the tokens and config files
  land on the **host**, via a home-directory bind mount (directories, not single
  files — the tool's atomic rename would fail with `EBUSY` over a single-file
  mount). Hardened with `cap_drop: ALL`, `no-new-privileges`, `pids_limit`,
  `init`, configurable UID/GID, and an allowlisting `.dockerignore`. New Make
  targets: `docker-shell` (enter), `docker-run`, `docker-check`,
  `docker-build`, `docker-up`, `docker-down`, `docker-clean`. The container
  reuses the project Makefile unchanged via the `VENV` override. Fully
  documented in `docs/container.md`.
- **System-wide distribution.** Documented and wired up every install path:
  `uv tool`/`uvx`, `pipx`, `pip`, **Homebrew** (`simtabi/homebrew-tap` formula in
  `packaging/`), **standalone single-file binaries** (no Python needed — built by
  PyInstaller in `release.yml` for Linux/macOS/Windows and attached to each GitHub
  Release), the `.pyz` zipapp, and the source launcher. New `make binary` target
  and a `[build]` extra. `docs/installation.md` covers them all.
- **Cleaner errors.** Unexpected `OSError`s (e.g. an unwritable target dir) now
  print a one-line `error: …` instead of a traceback; set `ATM_DEBUG=1` for the
  full traceback.
- **Richer token listing + show/hide secrets + clipboard copy.** `--list` (and the
  interactive list) now shows one table with **Tool · Host · What it's for · User ·
  Secret · File**, including a human description of each token and its file path.
  Secrets are masked by default; `--show-secrets` reveals them but **requires an
  interactive confirmation** (and `--yes`/`-y` when piped/non-interactive, so a
  stray flag in a script can't dump secrets to a log). `--list --json` emits a
  machine-readable inventory. New
  `--copy TARGET HOST` (and a "Copy a token" menu item) put a token on the OS
  clipboard **without printing it**. Long values (revealed tokens, file paths)
  **wrap onto multiple lines instead of being truncated** with an ellipsis, in
  both the rich and the no-`rich` table. New public API: `describe()`,
  `inventory(reveal=…)`, and `platform.copy_to_clipboard()`.
- **Interactive credential manager is the default.** Run `auth-token-manager` at a
  terminal and follow the prompts: pick **tool** → **provider** (GitHub, GitLab,
  Bitbucket, Docker Hub, npm, …) → **Cloud or Self-hosted / Enterprise** → URL (if
  self-hosted) → **type the token** (hidden input). It writes straight to the right
  auth file. Also browse / edit / remove installed tokens.
- **Full parity between interactive and flags.** Every action has a non-interactive
  equivalent: `--list` (installed tokens + paths, masked, works without `rich`),
  `--add` (secret read from **stdin** or a hidden prompt — never argv), and
  `--remove TARGET HOST`. `--add` takes `--target` or `--service` (Composer),
  `--host`, `--username`, `--scope`. Dispatch lives once in `apply_credential`.
- **Browse the system.** `inventory()` reads every auth file and reports host /
  type / username / masked token / file path; `remove_installed()` deletes one.
- **Shell aliases manager** (`auth-token-manager-aliases`): install/update/remove
  `atm`, `atm-ui`, `atm-doctor`, `atm-init` via an idempotent managed block
  (conda/`blockinfile` style) in the right rc file; backs up, writes atomically,
  preserves mode. `… print` emits the lines for `eval`.
- **`Makefile`** of common tasks (`make help`) — the `run`/`ui`/`aliases-*` targets
  work straight from a checkout.
- **Runs with or without `pip install`.** `rich`/`questionary` are optional at
  runtime (stdlib fallback); added a repo-root launcher and a single-file `zipapp`.
- **More registry providers** (all in the catalog): Composer now knows `packeton`,
  `nexus`, `gemfury`, and `unlock-sh` (all `http-basic`); Gemfury and unlock.sh are
  also npm provider quick-picks.
- **Per-tool config templates** in `config/templates/` plus `--init --target`.
- **Non-destructive merge** for every target (preserves entries it doesn't manage).
- **OS-aware path resolution** honoring `$NPM_CONFIG_USERCONFIG`, `$DOCKER_CONFIG`,
  `$NETRC`, Git's `$XDG_CONFIG_HOME/git/credentials`, and Windows locations.

##### Changed
- **Externalized the catalog to `config/catalog.json`.** Targets, the Composer
  service catalog + token-method map, enterprise/cloud hosts, the provider catalog,
  per-target fields, and the built-in default config live in one JSON file loaded by
  `catalog.py`; the module constants are typed views onto it. **Adding a
  service/provider is a config edit, not code.** Bundled into the wheel and zipapp.
  Removed the dead `_COMPOSER_TOKEN_SECTIONS`; de-duplicated the default config.
- The interactive UI now stores typed-in tokens directly (no env-var config / no
  `.env`); the flag + env-var-reference mode remains for CI/automation.
- Added a friendly Python-3.10+ guard to the launcher and entrypoint (a clear
  message instead of a traceback on an old interpreter).

##### Fixed
- **`--template` no longer writes placeholder secrets into real auth files** — it
  *prints* the skeleton (writing only with `--project`).
- `--env` no longer claims "wrote a file" for non-Composer targets; `--target X`
  with nothing to write prints a clear message and scopes its warnings.
- `inventory()`/`--list` no longer crashes on a malformed `machine X login` netrc
  line (`IndexError`).
- `to_git_credentials` preserves a given URL scheme (a scheme-prefixed host no
  longer produced a protocol-less, invalid line).
- Removing an npm host now also drops its orphaned `@scope:registry=` line.
- The no-`rich` output path no longer crashes on a console that can't encode a
  glyph (e.g. a legacy-codepage Windows terminal) — it falls back to `replace`.
- `make ui` runs from the venv (with `rich`/`questionary`) and auto-creates it if
  missing, instead of failing under the system python.
- `inventory()`/`--list` now also lists legacy npm `:_auth=` lines and Docker
  `identitytoken` logins, and a git host with a path (e.g. `dev.azure.com/org`)
  round-trips for listing and removal.
- `make ui` now auto-creates a **runtime-only** venv (package + `rich` +
  `questionary`), not the full `[dev]` toolchain.
- `inventory()`/`--list` now also reads the legacy npm split-credential form
  (`:username=` + `:_password=`).
- **netrc data-loss fix:** removing a `machine` no longer destroys a sibling that
  shared the same physical line — `.netrc` parsing/removal is now token-oriented
  (multiple machines per line are handled; `macdef`/comment files fall back to the
  safe line-based path). Multi-machine lines are now also listed correctly.
- **git ports preserved:** a credential host with an explicit port
  (`git.acme:8443`) now round-trips for listing and removal and no longer collides
  with the same host on the default port.

##### Security
- Ran a full security review (no `eval`/`exec`/`shell`/network; secrets never in
  argv or logs; auth files written `0600`/owner-only via atomic temp files).
  Documented that `--env` output intentionally contains the token; made the
  non-secret config save with normal (umask) perms instead of `0600`.

##### Notes
- Tested and CI-covered on Python 3.10–3.14.

#### auth-token-manager 0.2.0 - 2026-06-08

Never tagged. This version is commit `8027c61`; the first tag is v0.3.0.

A ground-up rework: renamed from `composer-auth` to **`auth-token-manager`**,
generalised beyond Composer, packaged for PyPI, and given an interactive UI.

##### Added
- **Multi-provider rendering.** New targets beyond Composer: npm (`.npmrc`),
  Docker (`config.json`), Git (`git-credentials`), and `.netrc`.
- **Interactive manager** (`--ui`) built on `rich` + `questionary`: list / add /
  update / remove entries with live "resolvable" status.
- **Cloud vs self-hosted / enterprise** host selection in the UI for VCS
  services, with a reminder to register self-hosted domains under
  `github-domains` / `gitlab-domains` in `composer.json`.
- **`--doctor`** (replaces `--check`): per-target perms, `.gitignore`, and
  Composer shadowing report across every present target.
- **`--target`** flag to render only selected targets; **`--version`**.
- Cross-platform secret hardening: `0600` on POSIX, an explicit `icacls` ACL on
  Windows; shell-aware env exports (bash/zsh, fish, PowerShell, cmd).
- Proper Python package (`src/` layout, `pyproject.toml`, `py.typed`), `mypy
  --strict` clean, `ruff` clean, and a `pytest` suite.

##### Changed
- Config is now **target-grouped** (`{"composer": {...}, "npm": {...}}`); the
  legacy bare/`credentials` Composer-only shape is still accepted.
- Config filename is `auth-token-manager.json`; env var is
  `$AUTH_TOKEN_MANAGER_CONFIG` (legacy `$COMPOSER_AUTH_CONFIG` still honoured).
- The single class is renamed `ComposerAuth` → `AuthTokenManager`.
- `--stdout` now prints a **redacted** summary across targets (no secrets).

##### Fixed
- `${VAR}` expansion no longer treats a present-but-empty env var as "unset".
- `to_env()` is shell-aware (was POSIX-only, breaking PowerShell/cmd).
- Git credential username/password are URL-encoded.

##### Security
- Owner-only hardening now applies on Windows (ACL), not just POSIX `chmod`.

[Unreleased]: https://github.com/simtabi/authama/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/simtabi/authama/releases/tag/v0.1.0
