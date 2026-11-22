# Tidy loose keys into their profiles

Years of `ssh-keygen` leave `~/.ssh` full of `id_rsa`, `id_ed25519_work`, `github_old` and a key or two dropped
somewhere odd. This recipe moves each one into the profile it belongs to - with a preview, a backup and a history
entry - and keeps watching for new ones. Nothing moves until you approve it.

## 1. See what is out of place

```sh
authama ssh structure list
```

Each line is a suggestion with an id, the file, and what approving it would do. A key whose name or comment names a
profile (`id_ed25519_work`, `me@work`) already has its profile guessed; the others say `--profile <profile>`.
Keys you [reserved](../tools/vm.md) or adopted in place are never listed.

## 2. Preview, then approve

```sh
authama ssh structure approve 69fc6ad9 --dry-run              # what would change, nothing written
authama ssh structure approve 69fc6ad9                         # the guessed profile
authama ssh structure approve 0e16 --profile personal --name legacy   # a profile and a new name
```

The preview lists the key files that move, and the manifest, inventory and `~/.ssh/config` diffs. Any
`IdentityFile ~/.ssh/id_ed25519_work` line in your own config is repointed to the new path in the same change, so
`ssh` keeps working.

Many at once, as a plan:

```sh
cat > moves.txt <<'PLAN'
key ~/.ssh/id_ed25519_work profile work
key ~/.ssh/id_rsa profile personal name legacy
PLAN
authama ssh structure approve --plan moves.txt
```

A key you mean to leave where it is: `authama ssh structure dismiss <id>`.

### Or in the browser

`authama serve`, then **SSH → Structure**: drag each stray key onto its profile (or pick the profile from its menu),
drag profiles between categories, then **Preview the moves**. The preview is the same one the terminal shows.

## 3. Keep watching

`authama serve` refreshes the list whenever a file under `~/.ssh` or the config home changes. To be told without
the web UI open:

```sh
authama ssh structure install      # checks every 15 minutes, notifies about new suggestions
```

## Undo

Each approval snapshots `~/.ssh` first and copies each moved key to `.state/moved/<time>/` in the state home;
`authama history` lists the change. See [ssh structure](../tools/structure.md).
