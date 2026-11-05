# Repair and factory reset

`authama repair` finds what is wrong with the authama home, `~/.ssh` and the token files, and fixes
what can be fixed without guessing. `authama reset` puts authama back to how it was before it was set
up. Neither deletes anything.

## Repair

```sh
authama repair                 # list what is wrong, ask, then fix
authama repair --check         # list only; exits non-zero when anything is wrong (CI)
authama repair --area ssh --yes
```

| Area | Fixed automatically | Reported, for you to fix |
|---|---|---|
| `tool` | a missing config, state, log or `.state` directory (made owner-only); a history database that fails SQLite's integrity check (renamed to `history.db.corrupt-<time>`, kept; a new one is filled from `audit.log`) | a manifest or inventory that does not load; an [owner seal](owner.md) that no longer matches |
| `ssh` | files a crashed command left in `~/.ssh`; modes ssh or authama refuse; keys with a private half and no `.pub` (written from the private key); a `~/.ssh/config` that no longer matches the manifest (rendered again, keeping what is outside authama's block) | |
| `tokens` | a credential file others can read (set to `600`) | a `tokens.json` that does not parse |

`~/.ssh` is snapshotted before any fix, and each fix is reported as it is made. A fix that fails
does not stop the others; the command exits non-zero and names it.

The web UI's **Repair** page, under System, lists the same findings and makes the fixes as a
change: previewed first, with each fix listed, then confirmed.

## Factory reset

```sh
authama reset                  # shows what it will move, then asks you to type "reset"
authama reset --confirm reset  # a script
```

1. A home [bound to its owner](owner.md) is unbound first: the PIN and the sealing key in the OS
   store go. A home renamed back later is then simply unbound, never bound to a key that is gone.
2. The config home, and the state home when it is separate, are renamed to `<name>.reset-<time>`.
3. `~/.ssh` is not touched: your keys, `config` and `known_hosts` stay.

Renaming the folders back undoes it. `authama init` starts again. With a PIN set, the command asks
for it, and the web UI's factory reset (on the Repair page) asks for it on the confirmation.

---

[← Docs index](../../README.md#documentation)
