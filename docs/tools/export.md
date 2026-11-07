# `export` — any list, and copies of the config for other tools

`authama export` writes any list in any format (below), and hosts also as `ssh_config` or as an SSH client imports
them. Three older commands write the same host files: `config export --flat` for anything that reads one `ssh_config`,
`termius export` for the Termius app, and `host export <client>` for Windows Terminal, iTerm2, PuTTY, Remmina,
SecureCRT, MobaXterm and any client that imports a CSV. `termius export` and `host export` are deprecated in favour of
`export hosts --format <client>`; they keep working and write the same bytes, and are not removed before v1.0.0.

All are opt-in and read-only towards `~/.ssh`. They write under `~/.config/authama/exports/` (mode 0600, since
the files list every host and key path), and they refuse any target inside `~/.ssh`, however the path is spelled: containment is
checked by inode, so another case on APFS or a symlinked directory is caught too. The split config, `~/.ssh/config` and `profiles/<category>/config`, stays exactly as it was.

## `authama export <list>`

Any list of the home, filtered and with the columns you choose, in any of nine table formats, and hosts in nine more
for ssh and SSH clients: the files the web UI's Export page makes, at the command line. Nothing in an export holds a
secret.

```bash
authama export hosts --profile simtabi --env prd --format json
authama export keys --columns ref=key,fingerprint,expires_on --sort expires_on --format markdown
authama export profiles --key simtabi_simbi-prd-web-00-ed25519 --narrow --format yaml
authama export hosts --fields                              # every field a host row has
authama export --selection sel.json --format php -o hosts.php
authama export hosts --profile simtabi --format ssh_config -o simtabi.ssh_config
authama export hosts --env prd --format putty -o prd.reg
```

| Part | What it takes |
|---|---|
| list | `hosts`, `keys`, `profiles`, `tokens`, `history`, `events`, `snapshots`, `deployments`; or `--selection` with a selection saved as JSON (the Export page's *Copy link* holds one) |
| filters | `--profile`, `--key` (a key's name or `profile/name`), `--env`, `--role`, `--tag`, `--status`, `--tool`, `--kind`, `--area`, `--source`, `--command`, `--since`/`--until` (a prefix such as `2026-11`), `--include`/`--exclude` text |
| columns | `--columns field,field=heading,…` by field, in order; related fields by path (`key.fingerprint`, `profile.code`) |
| order | `--sort field,-field` (`-` descends), `--limit n` |
| format | `--format csv` (default), `tsv`, `json`, `jsonl`, `yaml`, `php`, `js`, `markdown`, `html`; `--no-header`, `--compact`, `--shape table`. For hosts also `ssh_config`, `termius` and the clients below; `--include-git` |
| output | stdout, or `--out <file>` (mode 0600) |

`authama export hosts` and `authama export keys` with no options print exactly what `ssh list --csv` and
`ssh key list --csv` print: the same pipeline.

### Transforms

Transforms change the rows after the filters and before the columns, so a column can name a field a
transform wrote. They are declarative: a template names fields and nothing else runs.

```bash
authama export hosts --template 'ssh=ssh {user}@{hostname} -p {port}' --columns alias,ssh
authama export hosts --dedupe profile --columns profile,alias
authama export hosts --group-by profile --aggregate count,list:env --totals --format markdown
authama export hosts --transform '{"kind":"map","field":"status","values":{"needs-redeploy":"pending"}}'
```

| Transform | What it does |
|---|---|
| `template` | writes `as` from text naming fields in braces, `{profile}/{alias}`; `{{` and `}}` are literal braces |
| `map` | replaces a field's values listed in `values`; `default` replaces every other value |
| `format` | `upper`, `lower`, `trim`, `date` (a Go layout, `2006-01-02` by default), `relative` (`3 days ago`), `thousands` (`1,234`) |
| `split` | makes a field a list, split on `sep` (spaces by default); with `rows`, one row per item |
| `join` | makes a list text, joined by `sep` (`, ` by default) |
| `dedupe` | keeps the first row of each distinct `by` (every field when `by` is empty) |
| `group` | a row per distinct `by`, with `aggregates` (`count`, and `sum`, `min`, `max`, `list`, `first` of a field); `totals` adds a last row over every group |

`map`, `format`, `split` and `join` rewrite their `field` unless `as` names another. The flags add steps in
this order: each `--transform` (one step as JSON, the shape a saved selection holds), each
`--template as=text`, `--dedupe`, then `--group-by` with `--aggregate` (`count`, or `op:field[=as]`) and
`--totals`. A saved selection lists its steps under `transforms`, and they run in that order. `--fields`
lists the fields after the transforms. A totals row stays last whatever the sort, and `--limit` does not
count it. A step naming a field that does not exist yet fails, naming the step. The SSH client formats
below take no transforms, since they write the hosts themselves.

The web UI's Export page has the same steps in its **Transforms** card: add one from the list, fill in its
fields, and the preview shows the rows it leaves. A step stays unapplied, and says so, until it has what it
needs. Move a step up or down, or remove it. A group starts the columns afresh, since it replaces the fields,
and while there are steps the format list leaves out the SSH client formats. *Copy as command* writes each
step as a `--transform`.

### Saved selections

A selection saved by name is exported by that name, like a built-in list:

```bash
authama export selections save prd-hosts --from sel.json             # the default store: yours
authama export selections save prd-hosts --from sel.json --to manifest
authama export prd-hosts --format json
authama export selections list
authama export selections show prd-hosts
authama export selections delete prd-hosts --from manifest
```

Two stores hold them, and both are read:

| Store | Where | Who sees it |
|---|---|---|
| `user` | `<state home>/exports/selections/<name>.json`, mode 0600 | this account only |
| `manifest` | `defaults.export.selections` in the manifest | every machine that shares the manifest |

A save goes to the store `--to` names, else the one the `export.save_to` setting names, else `user`. On a
name in both stores, yours wins: `list` marks the manifest's as hidden, and deleting yours brings it back.
`delete` asks `--from` when both stores have the name. A manifest save or delete is a manifest change: it
asks first at a terminal (`--yes` skips that), takes the lock and a snapshot, and is audited. A name is 1 to
63 of `a-z`, `0-9`, `.`, `_` and `-`, starting with a letter or digit, and cannot be a built-in list's. A saved
selection of a version this authama cannot read is listed with the reason and never used. `--selection`
takes a saved name too, when no file has that name.

### Hosts for ssh and SSH clients

| `--format` | Writes | Same bytes as |
|---|---|---|
| `ssh_config` | the managed config of the hosts kept: each block as the flat export writes it, under its profile's banner, the git ssh-mode block when its `github.com` host is kept, `Host *` last | `ssh config export --flat`, when every host is kept |
| `termius` | the Termius CSV import | `ssh termius export` |
| `hosts-csv` | the generic client CSV (`alias, group, hostname, port, user, key_path, tags`) | `ssh host export csv` |
| `windows-terminal`, `iterm2`, `putty`, `securecrt`, `mobaxterm` | the client's file, named as the client expects (`authama-putty.reg`, …) | `ssh host export <client>` |
| `remmina` | a zip holding one `authama-<alias>.remmina` per host | `ssh host export remmina`, zipped |

The filters choose the hosts; `--columns` does not apply, since each client reads its own fields. With no `--sort`,
hosts come in manifest order, as the old commands list them; with one, in the sort's order. Git hosts are left out
unless `--include-git`, and every client but `windows-terminal` and `iterm2` leaves out hosts behind a ProxyCommand,
which it would not run; `ssh_config` keeps both, since ssh itself reads it. What was left out, and what is left to do
in the client, goes to stderr, so `--out -` or a pipe gets a clean file. The client table below says what each file
holds and where it goes.

The Export page offers the same formats on its Hosts tab, with an *Include git hosts* switch, and says how many hosts
the chosen format leaves out. *Copy as command* copies the `authama export` command that writes the file the page
previews, with its list, filters, columns, sort, format and options.

## Why a flat copy

With [categories](../recipes/group-profiles-into-categories.md), `~/.ssh/config` holds `Match all` and one `Include`
per category, and the hosts live in the included files. OpenSSH follows that; some importers do not. Termius reads only
the top-level file, so it finds no hosts there and falls back to bare addresses from `known_hosts`, with no user or key.

## `authama ssh config export --flat`

```bash
authama ssh config export --flat                     # ~/.config/authama/exports/ssh_config
authama ssh config export --flat --out ~/flat.conf   # another file
authama ssh config export --flat --out -             # stdout
```

The flat export is always the whole config: it takes no `--skip` or `--include-git`, because it is a copy for a
reader of ssh's own format, not a host list for another app.

| In the export | Value |
|---|---|
| Hosts | every profile's hosts inline, in manifest order, whatever their category |
| Order | the git ssh-mode `Host github.com` after the profiles, `Host *` last, as `config render` orders them |
| Paths | every `~/.ssh/...` and `~/...` made absolute (`IdentityFile`, `UserKnownHostsFile`, `IdentityAgent`, path-valued raw options) |
| Not present | `Include`, `Match`, the managed header (a copy must never be mistaken for a file authama owns) |

`ssh -G -F <export> <alias>` resolves a host from the copy the same way `ssh -G <alias>` does from the real config. The
copy goes stale when the manifest changes; export again rather than editing it.

## `authama ssh termius export`

Deprecated: `authama export hosts --format termius` writes the same CSV. This command keeps working.

Termius keeps hosts in its own encrypted vault and has no command line or API to add them, so authama cannot write into
it. It writes the CSV Termius's importer reads instead:

```bash
authama ssh termius export                      # ~/.config/authama/exports/termius-hosts.csv
authama ssh termius export --open               # macOS: also open Termius and show the file in Finder
authama ssh termius export --skip uni           # leave a host out (repeatable)
authama ssh termius export --include-git        # also git hosts
authama ssh termius export --out - > hosts.csv  # CSV on stdout, the report on stderr
```

| Column | Value |
|---|---|
| `Groups` | the profile |
| `Label` | the host alias |
| `Tags` | the host's tags (a comma inside one tag becomes a space) |
| `Hostname/IP` · `Port` · `Username` | from the host; `Protocol` is `ssh` |
| `Password` | always empty |

Left out, and listed under `skipped:`:

- **VM hosts** — reached only through authama's ProxyCommand, which Termius ignores;
- **git hosts** — user `git`, or a `github`/`gitlab` provider: not shells (`--include-git` adds them);
- **`--skip` aliases** — an alias that names no host is an error, not a silently ignored typo.

A value starting with `=`, `+`, `-` or `@` is prefixed with `'`, so the file cannot run a formula if opened in a
spreadsheet.

**The CSV carries no keys.** Termius's CSV import takes labels, addresses, ports, groups, tags and credentials but has
no key column ([Import existing hosts](https://docs.termius.com/getting-started/import-existing-hosts)); keys live in Termius's Keychain and are linked to each host
([Keychain](https://support.termius.com/hc/en-us/articles/4401872025113-Keychain), [SSH keys and certificates](https://docs.termius.com/keychain/ssh-keys-and-certificates)). The export prints each host's key path, then:

## `authama ssh termius keys`

```bash
authama ssh termius keys                              # every exported host: HOST, TERMIUS LABEL, PRIVATE KEY path
authama ssh termius keys db-01               # copy its private key to the clipboard, print the label
authama ssh termius keys db-01 --clear-after 60s   # ...and empty the clipboard after 60 seconds
authama ssh termius keys --clear                      # empty the clipboard now
authama ssh termius keys db-01 --print       # key on stdout instead (warns on stderr; avoid)
authama ssh termius keys github-me                    # a git host: naming it is enough
authama ssh termius keys --include-git                # list git hosts too, as with export --include-git
authama ssh termius keys --skip uni                   # leave a host out, as with export --skip
```

The label is the key file's name (`clientco_db-01-ed25519`). The key is copied with `pbcopy` (macOS),
`wl-copy`, `xclip` or `xsel` (Linux), or `clip` (Windows), and never printed without `--print`. In Termius:
**Keychain → New Key** (or **Add key** on SSH key selection), Label = the label, paste into **Private key**, leave
**Certificate** empty, **Save**; pick it on the host and **Continue & Save**. A key kept in 1Password is refused, to
add from 1Password instead. Full walk-through: [Import your hosts into Termius](../recipes/import-hosts-into-termius.md).

The column layout is the one third-party exporters reproduce from Termius's import template (Termius 9.x). Termius
publishes the template only inside the app (Hosts → New Host → Import → CSV → download template); if its header ever
differs, `TermiusColumns` in `src/internal/util/encode/clients.go` is the one place to change.

## `authama ssh host export <client>`

Deprecated: `authama export hosts --format <client>` writes the same files (the generic CSV is `hosts-csv` there).
This command keeps working.

```bash
authama ssh host export windows-terminal        # ~/.config/authama/exports/windows-terminal/authama.json
authama ssh host export iterm2
authama ssh host export putty --skip bastion    # leave a host out (repeatable)
authama ssh host export remmina --include-git   # also git hosts
authama ssh host export securecrt               # SecureCRT, and Termius's SecureCRT import
authama ssh host export mobaxterm               # MobaXterm, and Termius's MobaXterm import
authama ssh host export csv --out - > hosts.csv # one-file exports can go to stdout
```

| Client | Writes | Then | Key |
|---|---|---|---|
| `windows-terminal` | `authama.json`, a JSON fragment: one profile per host, named `profile / alias`, running `ssh.exe <alias>` | copy it to `%LOCALAPPDATA%\Microsoft\Windows Terminal\Fragments\authama\` and restart Windows Terminal | from `~/.ssh/config`, through ssh |
| `iterm2` | `authama-hosts.json`, Dynamic Profiles running `ssh <alias>`, tagged with the profile and the host's tags | copy it to `~/Library/Application Support/iTerm2/DynamicProfiles/`; iTerm2 loads it at once | from `~/.ssh/config`, through ssh |
| `putty` | `authama-putty.reg`, one session per host (UTF-16 with CRLF, as regedit writes) | double-click it, or `reg import authama-putty.reg` | `PublicKeyFile` names `<key>.ppk`; the report prints the `puttygen` command that makes each |
| `remmina` | one `authama-<alias>.remmina` per host | copy them to `~/.local/share/remmina/`, or import them from Remmina's menu | `ssh_privatekey`, with `ssh_auth=1`; Remmina also wants the `.pub` beside it |
| `securecrt` | `authama-securecrt.xml`, SecureCRT's settings XML: a session folder per profile, an SSH2 session per host | SecureCRT: **File → Import Settings**; Termius: **Import → SecureCRT** | each session's own `Identity Filename V2` (SecureCRT 8 and later read OpenSSH keys) |
| `mobaxterm` | `authama.mxtsessions`: a bookmark folder per profile, an SSH session per host | MobaXterm: **Sessions → Import sessions from file**; Termius: **Import → MobaXterm** | the session's private key field (MobaXterm reads OpenSSH keys) |
| `csv` | `hosts.csv`: `alias, group, hostname, port, user, key_path, tags` (tags `;`-separated) | import it, mapping the columns | `key_path` |

`windows-terminal` and `iterm2` launch `ssh`, which reads your `~/.ssh/config`, so a host reached through a
ProxyCommand (a VM, Cloudflare Access) works there as at the terminal, and re-exporting updates the same profiles:
Windows Terminal derives each profile's id from its name, and iTerm2's `Guid` is `authama-<alias>`. The other clients
are handed a host, port, user and key path and do not run a ProxyCommand, so such hosts are left out and listed under
`skipped:`. Git hosts are left out of every export unless `--include-git`.

Royal TS reads the CSV with its column-mapping dialog; map `hostname` to its `URI` column. Royal TS's and Xshell's CSV
imports have no key column, so attach each key in the client. Tabby, WinSCP and Termius import an OpenSSH config, so
give them the file `config export --flat` writes. Xshell's own session files are not written: its format keeps keys
in a store of its own.

**Termius imports all five ways it offers**: `~/.ssh` (the config authama renders, or `config export --flat`), CSV
(`ssh termius export`), SecureCRT (`host export securecrt`), PuTTY (`host export putty`, the `.reg`) and MobaXterm
(`host export mobaxterm`). Key paths in every file are this machine's: edit them when the client runs elsewhere.

---

[← Docs index](../../README.md#documentation)
