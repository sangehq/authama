# Give a host more names, a proxy, and a second route

A managed config file is written from the manifest, so a line typed into it by hand is gone at the next render. This
recipe puts those lines in the manifest instead: extra names on a `Host` line, `ProxyCommand` (including
`ProxyCommand none`), `ProxyJump`, `ConnectTimeout` and any other ssh option, a heading above a group of hosts, a
comment inside a block, and a second route to the same server - a WARP twin on its private address next to the
Cloudflare Access host.

## The blocks

Two servers, each reachable two ways:

```
# simbi prod: Cloudflare Access (works anywhere; browser login once per host)
Host simbi-prd-web-00 simbi-web-cf
    HostName ssh-web-00.simtabi.com
    User ploi
    IdentityFile ~/.ssh/profiles/orgs/simtabi/simtabi_simbi-prd-web-00-ed25519
    IdentitiesOnly yes
    UserKnownHostsFile ~/.ssh/known_hosts
    ProxyCommand /opt/homebrew/bin/cloudflared access ssh --hostname %h

Host simbi-prd-db-00 simbi-db-cf
    ...

# simbi prod: private IPs over Cloudflare WARP (WARP must be connected)
Host warp-simbi-prd-web-00
    HostName 10.116.0.3
    User ploi
    IdentityFile ~/.ssh/profiles/orgs/simtabi/simtabi_simbi-prd-web-00-ed25519
    IdentitiesOnly yes
    UserKnownHostsFile ~/.ssh/known_hosts
    ProxyCommand none
    ConnectTimeout 10

Host warp-simbi-prd-db-00
    ...
```

authama writes `UserKnownHostsFile ~/.ssh/known_hosts` into every block it manages, so every host it pins keys for
shares one known_hosts store; apart from that line these are the blocks as written by hand, byte for byte.

## From the command line

```bash
# The Cloudflare Access hosts, with a second name each. --update makes add create-or-update.
authama ssh host add simtabi simbi-prd-web-00 -H ssh-web-00.simtabi.com -u ploi --alias simbi-web-cf \
  --proxy cloudflared --section 'simbi prod: Cloudflare Access (works anywhere; browser login once per host)' --update
authama ssh host add simtabi simbi-prd-db-00 -H ssh-db-00.simtabi.com -u ploi --alias simbi-db-cf \
  --proxy cloudflared --update

# Their WARP twins: same user and key, the private address, a direct connection.
authama ssh host variant set simtabi simbi-prd-web-00 warp -H 10.116.0.3 --proxy none --connect-timeout 10 \
  --section 'simbi prod: private IPs over Cloudflare WARP (WARP must be connected)'
authama ssh host variant set simtabi simbi-prd-db-00 warp -H 10.116.0.5 --proxy none --connect-timeout 10
```

`ssh host add` and `ssh host edit` take:

| Flag | What it does |
| --- | --- |
| `--alias NAME` | another name on the `Host` line (repeatable); `edit --remove-alias NAME` takes one off |
| `--proxy cloudflared` | `ProxyCommand <cloudflared> access ssh --hostname %h` (Cloudflare Access) |
| `--proxy none` | `ProxyCommand none`: connect directly, even under a broader block that sets a proxy |
| `--proxy off` | remove the proxy (no `ProxyCommand` line) |
| `--proxy-command '<command>'` | a `ProxyCommand` of your own; each argument is quoted for the shell, and `;`, `&`, `|`, `<`, `>`, `$`, backticks and parentheses are refused. `''` removes it |
| `--proxy-jump HOST` | `ProxyJump` (`''` removes it) |
| `--connect-timeout N` | `ConnectTimeout` in seconds (`0` removes it) |
| `--option KEY=VALUE` | any other ssh_config option OpenSSH knows (repeatable); `KEY=` or `edit --unset-option KEY` removes it |
| `--section TEXT` | a heading comment above the block; hosts after it with none sit under it |
| `--comment TEXT` | a comment inside the block, under its `Host` line |
| `--update` (add) | change the host if the profile already has it, instead of refusing |

`--option` checks the keyword against OpenSSH's list (a typo gets a suggestion, `ServerAliveInterval` for
`serveraliveinterval` is written in OpenSSH's spelling) and refuses the options authama writes itself (`HostName`,
`User`, `IdentityFile`, ...) and those that run a command, load code or open a block (`LocalCommand`,
`RemoteCommand`, `Match`, `Include`, ...). A proxy is set with `--proxy` or `--proxy-command`, never `--option
ProxyCommand=...`.

## Route variants

`ssh host variant set <profile> <alias> <name>` adds a block named `<name>-<alias>` that reaches the same server
another way. It uses the host's user and key - rotating the key or changing the user moves both routes - and sets
only what differs: `--hostname`, `--port`, the proxy, the options and the comments. It inherits the host's proxy and
options unless it sets its own; a variant with its own proxy drops an inherited `ProxyJump`. Variants render after
the profile's hosts, each name's together under its `--section`. `ssh host variant remove <profile> <alias> <name>`
takes one away; deleting the host removes its variants.

## Hand edits

authama remembers what it last wrote to each managed file. When a line in one has changed since,
`authama ssh config render` lists it and stops instead of overwriting it:

```
authama: the managed SSH config was edited by hand, and rendering would discard it:
  profiles/orgs/config: warp-simbi-prd-web-00: sets ConnectTimeout 15 (authama wrote 10)
keep them with `authama ssh config keep-edits` (it adds them to the manifest), or discard them with `authama ssh config render --force` (the file is backed up first)
```

- `authama ssh config keep-edits --dry-run` shows what it would take in; without `--dry-run` (or with `--yes`) it
  updates the manifest and renders again. A block for a managed host updates that host; a block named
  `<name>-<alias>` with the user and key of host `<alias>` becomes its variant `<name>`; any other block becomes a new
  host of the profile whose banner it sits under, using the key its `IdentityFile` names in that profile's directory.
  A block it cannot take in whole is left as it is, with the reason.
- `authama ssh config render --force` writes over them, after copying the edited files to
  `<state home>/config-hand-edits-<time>/`.
- `authama ssh config check` lists them as `HAND-EDIT` lines.

Hand-written blocks outside authama's managed files and blocks are still kept as they always were;
`authama ssh config adopt` handles the ones in `~/.ssh/config` that duplicate a managed host.

## In the web UI

The host page's **Edit** panel has a **Routes** section with the same fields, filled in with the host's current
values: names, proxy (or a command), jump host, connect timeout, other options one `KEY=VALUE` a line, the section
heading and the comment. A name or option taken out of the box is removed (`--remove-alias`, `--unset-option`).
**Route variants** on the host page lists the host's variants; each has an edit panel filled in with its values (an
option taken out is unset; proxy **off** goes back to the host's) and a remove button, and **Add or change one** sets
a variant. The **Add a host** panel has the same Routes section and **Update it if the profile has it already**
(`--update`); adding a host the profile has offers **Preview the update** instead. A bad option is refused with the
closest keyword ssh knows.

**SSH config** lists hand edits in a **Hand edits** card (the `HAND-EDIT` lines of `ssh config check`), with **Keep
hand edits** (`ssh config keep-edits`: its preview lists each block it takes in and the manifest change) and
**Discard hand edits and render** (`ssh config render --force`: a destructive change, confirmed, behind the owner
PIN when one is set; the result names the backup directory). A render refused over hand edits shows the list and both
choices. Every one is previewed first. **Import** takes in blocks with more names, `ProxyCommand` (`none`, the
Cloudflare Access command or one of your own), other options and their section and comment lines, so a block
written by hand round-trips.

## Exports

- `ssh_config`, `windows-terminal` and `iterm2` go through ssh, so they carry every name, proxy and variant.
- Termius and the other clients get one row per host and one per variant, labelled by the alias; extra names are not
  rows. A host behind `cloudflared` or a `--proxy-command` is left out (they do not run a ProxyCommand) and reported;
  a host or variant with `ProxyCommand none` is a direct connection and is exported, so the WARP twins go in and the
  Access hosts stay out. `ConnectTimeout` and other options are not carried; `ProxyJump` is reported as a Host Chain
  to set by hand.

Reference: [`proxy`](../configuration.md#proxy), [host routes](../configuration.md#host-routes-aliases-section-comment-variants).

---

[← Docs index](../../README.md#documentation)
