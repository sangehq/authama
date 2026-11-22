# ssh structure - keep keys in the expected layout

```sh
authama ssh structure list [--all] [--json]          # what sits outside the layout, and the move for each
authama ssh structure check [--notify] [--quiet]     # scan for changes, refresh the list (the background job runs this)
authama ssh structure approve <id>... [--profile P] [--name N] [--dry-run] [-y]
authama ssh structure approve --plan FILE|- [--dry-run] [-y]
authama ssh structure dismiss <id>... [--undo]
authama ssh structure install [-y] | uninstall       # the optional background check
```

authama expects every key it manages in a profile's directory under the key root:
`~/.ssh/<key root>/<profile>/<key>`, or `~/.ssh/<key root>/<category>/<profile>/<key>` for a profile in a
category ([choose where keys live](../recipes/choose-where-keys-live.md),
[categories](../recipes/group-profiles-into-categories.md)). The structure watcher finds what is not there and
**suggests** a move for each. It never moves anything by itself: each move waits for you to approve it, here or on
the web UI's structure page.

## What it finds

| Kind | What | The suggested move |
|---|---|---|
| `loose` | A key pair directly in `~/.ssh` (`id_ed25519_work` and its `.pub`) | Into a profile's directory, declared in that profile. The profile is guessed from a profile name appearing in the key's file name or comment (`id_ed25519_work`, `me@work`); with no single match, approve it with `--profile`. |
| `untracked` | A key in a profile's directory that no profile declares | Declared where it is: nothing moves. |
| `orphan` | A key in a key-root directory no profile owns (a profile deleted by hand, a typo) | Into the profile you name with `--profile`. |
| `misplaced` | A key a profile declares, in another directory than the manifest resolves (a profile moved into a category by hand) | To where the manifest puts it. |
| `config-home` | A key pair in the config home, which holds settings and never keys | Into a profile's directory under `~/.ssh`. |
| `copy` | A leftover copy of a key authama manages (`vm adopt-key` without `--move`) | Nothing: a note. Remove it yourself once ssh through the managed copy works. |

Left alone, and not listed: keys you [reserved](vm.md) (`key reserve`), keys [adopted](../recipes/onboard-an-existing-ssh-dir.md)
where they are (`key adopt` promises never to move them), symlinks, and anything that is not a key pair. Detection
reads public keys and file names only, never private key material.

Each suggestion has a short id, stable while the file stays where it is, so a dismissal sticks until the file moves.

## Approving a move

```sh
authama ssh structure list
# 69fc6ad9  loose       ~/.ssh/id_ed25519_work
#           a key loose in ~/.ssh: move it into a profile's directory so authama renders, rotates and audits it
#           approve: authama ssh structure approve 69fc6ad9 -> ~/.ssh/profiles/work/id_ed25519_work
#           named in ~/.ssh/config (repointed on approval)
authama ssh structure approve 69fc6ad9 --dry-run     # the preview only
authama ssh structure approve 69fc6ad9               # the preview, then a confirmation
```

The preview runs on a throwaway copy of the home, the same as every web change: it lists each key file removed
and created (a private key's content is never copied into the preview, only its path), and the diffs of the
manifest, the inventory and `~/.ssh/config`. After you confirm, for each move authama:

1. takes the usual snapshot of `~/.ssh`;
2. copies the files it is about to move, owner-only, to `.state/moved/<UTC time>/` in the state home;
3. moves the private key, its `.pub` and any `-cert.pub` into the profile's directory (made `0700`; the key `0600`,
   the `.pub` `0644`), refusing a symlink or a destination that already exists;
4. declares the key in the profile and records it in the inventory, dated from the file;
5. repoints every `IdentityFile` and `CertificateFile` line in your hand-written ssh config (`~/.ssh/config`,
   `~/.ssh/config.d/*`, `~/.ssh/*.conf`) that named the old path, and re-renders the managed config;
6. writes a `structure.move` line to the audit log and the change to the [history](history.md).

Everything in a plan is checked before anything is touched: an unknown profile, a name the profile already
declares, or two moves to one place refuse the whole plan.

### Plans

`--plan` approves move lines directly (one per line; `#` comments allowed). The web structure page builds the same
lines from its tree.

```text
key ~/.ssh/id_ed25519_work profile work            # move a key into a profile, under its own name
key ~/.ssh/id_rsa profile personal name legacy     # ... or a new one
key config/stray_key profile work                  # a key from the config home
misplaced work/deploy                              # a declared key back where the manifest puts it
host work/gh profile personal                      # a host into another profile, with its own key if no other host uses it
key work/deploy profile ops                        # a declared key into another profile, with the hosts that use it
profile work category clients                      # a profile into a category: its directory moves too
profile work category -                            # ... and out of one
```

## In the web UI

**SSH → Structure** (`/structure`, `g b` in the palette) draws the folder as a tree: the key root, each category, each
profile with its directory, the keys it declares (with a badge for one kept where it is, or not on disk yet) and its
hosts with the key each uses, then **Outside the layout** - every suggestion, with its kind, why, and the config
files that name it.

- **Choose a move** from an item's menu: a profile's moves it into another category or out of one; a stray key's
  moves it into a profile (the guessed one is marked *suggested*); a misplaced key's moves it back.
- **Or drag it**: a profile onto a category, a stray key, a declared key or a host onto a profile. A drop only chooses the same menu entry, so
  the keyboard and the mouse reach the same moves, and the page works without the script. Items that can move are
  marked as draggable; a place an item cannot go does not accept it.
- **Moves** lists what is chosen so far, and takes extra [plan lines](#plans) - a new category, a key under a new
  name. **Preview the moves** opens the usual preview: each key file that moves (its path, never its content), and
  the manifest, inventory and `~/.ssh/config` diffs. Nothing is written until you confirm, and the change is in
  History like any other.
- **Dismiss** hides a suggestion; **Show the dismissed suggestions** lists them with **Show again**.

### Moving hosts and keys between profiles

A host moved to another profile takes its own key along when no other host uses it. Otherwise it takes the key its
new profile gives it, which `ssh reconcile` mints and you deploy; the preview says which. A declared key moves only
with every host of its profile that uses it, so a plan moving the key alone is refused while a host still uses it.
A moved key keeps its inventory record and deployments, because it is the same key. Its key specs follow it, and
`~/.ssh/config` lines naming it are repointed.

Refused, before anything moves: a key kept where it is (`ssh key adopt`), a shared key, a target profile with shared
key scope or one that already has that key name, and a 1Password profile on either side. A read-only server shows
the tree and the suggestions, without the menus.

## The watcher

- **Inside `authama serve`.** The server already scans `~/.ssh`, the config home and token files every 30 seconds
  for the [events](events.md) log. When a scan sees a change under `~/.ssh` or the config home, it refreshes the
  suggestions (kept in `.state/structure.json`), which the structure page shows.
- **In the background.** `ssh structure install` registers a job that runs
  `authama ssh structure check --notify --quiet` every 15 minutes: a launchd agent (`StartInterval`) on macOS, a
  systemd user timer (`OnUnitActiveSec=15min`, or a cron line where there is no systemd) on Linux, a Scheduled Task
  (`/SC MINUTE /MO 15`) on Windows, under the label `authama.structure`. Each run records file events, refreshes the
  suggestions and posts one desktop notification naming the new ones (each is named once; set
  `AUTHAMA_NOTIFICATIONS=false` to silence it). `ssh structure uninstall` removes it. Both refuse in
  [dev mode](../recipes/dev-mode.md), since a scheduled job outlives the sandbox.

## Undoing a move

Every approval takes a snapshot of `~/.ssh` first (`authama ssh snapshots list`) and keeps a copy of each moved key
in `.state/moved/`. The manifest and inventory changes are in the history. Moving a key back by hand and deleting its
declaration (`ssh key delete <profile/key>`) returns it to `loose`.
