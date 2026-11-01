# `tui` — interactive terminal UI

A numbered stdin menu over the same services the CLI verbs use, so nothing is
reimplemented and the two cannot drift.

```sh
authama          # on a terminal, this opens the menu
authama tui      # the same thing, explicitly
```

Running the binary with no arguments opens the menu when standard input is a
terminal. With any argument it is a plain CLI, and without a terminal — a pipe, a
script, a CI job — it prints help instead. That fallback matters: a menu reading
from a pipe would take whatever is on it as answers.

## What it is

A plain numbered menu, not a full-screen application. Type a number, press enter.
There are no arrow keys, no alternate screen, and no colour — the binary has one
third-party dependency (`cobra`) and no presentation library, which is what makes
every command's output pipe-safe by default. See *Why is the TUI a plain menu?*
in [Architecture](../architecture.md).

## The banner

It opens with a header describing the session:

```
  authama v3.0.1  darwin/arm64   built 2026-08-06 from d7ea28f

  home      ~/.config/ssh-manager   (providers: shipped default)
  ssh       ~/.ssh
  manifest  5 profiles, 7 hosts, 7 keys
  agent     running, 2 keys loaded
! rendered  drifted from the manifest          authama ssh reconcile
! pins      6 of 7 pinned                      authama ssh knownhosts pin
! perms     1 path with the wrong mode         authama doctor --fix
```

The two paths are the point of it. Both the config home and `~/.ssh` are
environment-overridable, so without them a session pointed at a sandbox and one
pointed at your real tree look identical — and every entry under *Change
`~/.ssh`* rewrites whichever one it is.

A row marked `!` is a problem, and carries the command that resolves it in the
last column, so the report and the remedy are never in two different places. A
first run has no manifest and says so, naming `authama init`.

The banner is redrawn after any action that changes `~/.ssh`, so watching
`drifted` become `in sync` is the confirmation that the action landed.

## The menu

Entries are grouped by consequence, not by frequency — everything above *Change
`~/.ssh`* is safe to pick by accident:

```
  Inspect
   1  Browse profiles & hosts         ssh list
   2  Show the rendered config        ssh config show
   3  Expiry status                   ssh expiry
   4  Audit deployments & expiry      ssh audit

  Change ~/.ssh   (each one asks before it writes)
   5  Reconcile - apply the manifest  ssh reconcile
   6  Pin host keys into known_hosts  ssh knownhosts init --all
   7  Deploy a key to its target      ssh deploy
   8  Rotate a key                    ssh rotate

  Recover
   9  Snapshots - list and restore    ssh snapshots

  Auth tokens   (each one asks before it writes)
  10  List installed tokens           token inventory
  11  Check the token files           token checkup
  12  Copy a token to the clipboard   token clip
  13  Store a token                   token store
  14  Sync tokens from the config     token sync
  15  Forget a token                  token forget

   q  Quit
```

The right-hand column is the CLI verb that does the same thing, and it is also
an accepted answer — typing what is on screen is never a dead end. So `5`,
`ssh reconcile`, `reconcile` and the full label all select the same entry, as
do `10`, `token inventory` and `inventory`.

Entry 6 runs `knownhosts init --all`: a trust-on-first-use scan of every manifest
host. Earlier releases listed it as `knownhosts pin`, which is a different command
(it asks about each key and needs a host or `--all`); that answer still selects
the entry.

## Auth tokens

The last group is the token side, the menu `auth-token-manager --ui` used to
be. *List* masks every secret; *Check* writes nothing. *Store* asks for the
tool, the server and (where the tool needs one) a user name, then reads the
token without echoing it. *Sync* writes every target in the token config, and
*Forget* picks an installed entry from a list and asks before removing it. To
edit an entry, store it again: a store replaces the entry for that server. Each
applies the same checks as its `authama token` command, and in dev mode every
file is the sandbox's.

## Answering it

A number, a command name, or `q`. Anything else re-prompts rather than selecting:
an out-of-range number, a misspelled verb, a stray keystroke. A menu that treated
an accidental key as a choice would run whichever action happened to be first,
and one that exited on a typo would cost you the session for a slip. A bare Enter
redraws the menu without comment; a closed input ends the session.

Every entry under *Change `~/.ssh`*, and snapshot restore, asks before it writes,
and each applies the same guards as its CLI command: it takes the advisory lock
before it writes, and `~/.ssh` is snapshotted before any change. The lock is
released when the change ends, so an open menu never holds up another authama
command between changes, and no question waits for your answer while holding it.
Reconcile shows its dry run as the question, then takes the lock and works the
dry run out again; if the manifest moved while you were deciding, it stops and
changes nothing. It pins host keys afterwards unless `SSH_MANAGER_AUTO_PIN=0`. Deploy is refused in dev mode. Rotate writes an
encrypted key backup first, as `authama ssh rotate` does, so it needs
`SSH_MANAGER_AGE_RECIPIENT`; the TUI has no `--no-key-backup`, so without a
recipient it stops and says so.

It needs an interactive terminal. In a script or a pipe it prints a hint and
exits; use the CLI verbs there.

## Design note

Interaction goes through a small `prompter` seam: production reads the command's
own input stream, tests inject a scripted fake. So the whole navigation loop —
including that destructive actions require confirmation — is exercised without a
TTY.

> The production prompter read `os.Stdin` directly until v2.1, which meant it
> ignored a redirected input and could only ever be driven by a real terminal.
> Every test injected the fake, so the code a user actually meets was the one
> piece of the TUI never exercised. It reads `c.InOrStdin()` now, like every
> other prompt in the tool.

---

[← Docs index](../../README.md#documentation)
