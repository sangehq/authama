# `export` — copies of the config for other tools

Three commands write a copy of your hosts for another tool: `config export --flat` for anything that reads one
`ssh_config`, `termius export` for the Termius app, and `host export <client>` for Windows Terminal, iTerm2, PuTTY,
Remmina and any client that imports a CSV.

All are opt-in and read-only towards `~/.ssh`. They write under `~/.config/authama/exports/` (mode 0600, since
the files list every host and key path), and they refuse any target inside `~/.ssh`, however the path is spelled: containment is
checked by inode, so another case on APFS or a symlinked directory is caught too. The split config, `~/.ssh/config` and `profiles/<category>/config`, stays exactly as it was.

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
differs, `TermiusColumns` in `src/internal/core/renderer/export.go` is the one place to change.

## `authama ssh host export <client>`

```bash
authama ssh host export windows-terminal        # ~/.config/authama/exports/windows-terminal/authama.json
authama ssh host export iterm2
authama ssh host export putty --skip bastion    # leave a host out (repeatable)
authama ssh host export remmina --include-git   # also git hosts
authama ssh host export csv --out - > hosts.csv # one-file exports can go to stdout
```

| Client | Writes | Then | Key |
|---|---|---|---|
| `windows-terminal` | `authama.json`, a JSON fragment: one profile per host, named `profile / alias`, running `ssh.exe <alias>` | copy it to `%LOCALAPPDATA%\Microsoft\Windows Terminal\Fragments\authama\` and restart Windows Terminal | from `~/.ssh/config`, through ssh |
| `iterm2` | `authama-hosts.json`, Dynamic Profiles running `ssh <alias>`, tagged with the profile and the host's tags | copy it to `~/Library/Application Support/iTerm2/DynamicProfiles/`; iTerm2 loads it at once | from `~/.ssh/config`, through ssh |
| `putty` | `authama-putty.reg`, one session per host (UTF-16 with CRLF, as regedit writes) | double-click it, or `reg import authama-putty.reg` | `PublicKeyFile` names `<key>.ppk`; the report prints the `puttygen` command that makes each |
| `remmina` | one `authama-<alias>.remmina` per host | copy them to `~/.local/share/remmina/`, or import them from Remmina's menu | `ssh_privatekey`, with `ssh_auth=1`; Remmina also wants the `.pub` beside it |
| `csv` | `hosts.csv`: `alias, group, hostname, port, user, key_path, tags` (tags `;`-separated) | import it, mapping the columns | `key_path` |

`windows-terminal` and `iterm2` launch `ssh`, which reads your `~/.ssh/config`, so a host reached through a
ProxyCommand (a VM, Cloudflare Access) works there as at the terminal, and re-exporting updates the same profiles:
Windows Terminal derives each profile's id from its name, and iTerm2's `Guid` is `authama-<alias>`. The other clients
are handed a host, port, user and key path and do not run a ProxyCommand, so such hosts are left out and listed under
`skipped:`. Git hosts are left out of every export unless `--include-git`.

Royal TS reads the CSV with its column-mapping dialog; map `hostname` to its `URI` column. Royal TS's and Xshell's CSV
imports have no key column, so attach each key in the client. Tabby, WinSCP and Termius import an OpenSSH config, so
give them the file `config export --flat` writes. MobaXterm, SecureCRT and Xshell's own session files are not written:
their formats are undocumented or keep keys in a store of their own.

---

[← Docs index](../../README.md#documentation)
