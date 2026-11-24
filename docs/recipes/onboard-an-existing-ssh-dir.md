# Onboard an existing `~/.ssh`

Bring a hand-built setup under management without regenerating anything.

```sh
authama init                      # create the home (seeds an EMPTY manifest)
authama ssh import ~/.ssh/config      # read the config into the manifest
authama ssh diff                      # what would change, before anything does
authama ssh reconcile
```

`import` parses your config — including `Include` directives — and derives a
profile per `IdentityFile` path, falling back to `imported`.

It reads any layout, flat or nested. Give it a file or a folder:

```sh
authama ssh import ~/.ssh                    # its config, and whatever that Includes
authama ssh import ~/.config --dry-run       # a config home with ssh/config
authama ssh import ~/old-laptop/.ssh --yes   # another machine's copy
authama ssh import ~/ssh-snippets            # no top-level config: every config and *.conf, nested
```

| Given | It reads |
|---|---|
| a file | that file |
| a folder with `config` | that file; its `Include`s pull in `config.d/`, `profiles/<name>/config` and the rest |
| a folder with `ssh/config` | that file, for a config home such as `~/.config` |
| any other folder | every file named `config` or ending `.conf`, up to four levels down, in path order |

Relative `Include`s resolve against the folder, as ssh resolves them against `~/.ssh`. Quoted paths
with spaces, `~`, globs (including `**`), and nested Includes are followed the same way; a directory
`Include` is skipped (use a glob such as `config.d/*.conf`). See
[SSH configs in any folder layout](../reference/ssh-config-includes.md). From another
machine's copy, an `IdentityFile ~/.ssh/<key>` is looked for in that copy, so its keys are adopted
here. A key that already
lives under `profiles/<name>/` is referenced where it is; one outside the managed
tree is **copied** in, never moved, and the copy is made owner-only whatever mode
the original had.

> Nothing is regenerated. `import` never mints over an existing key, so the
> identities your servers already trust keep working.

Importing this machine's own config **adopts** each plain `Host` block it imports: the block leaves
`~/.ssh/config` (or the file under `~/.ssh` it was Included from) and authama's managed block is
rendered in the place of the first one, in the same write. ssh reads one definition of every host,
so `authama doctor` is clean after `reconcile`. Every file it edits is copied first to
`<state>/.state/import-backup-<time>/ssh/`, and the run is snapshotted, so `authama ssh snapshots`
can undo it. `--dry-run` says what it would move and writes nothing.

| In your config | What import does |
|---|---|
| `Host web` whose every line the manifest reproduces | imported, block moved out |
| `Host jump` with a `ProxyCommand`, a second `IdentityFile`, its own `UserKnownHostsFile`, or an `Include` | **not imported**, block left as it is, listed under "kept in your config" with the line |
| `Host web *.lab` (a literal and a wildcard on one line) | not imported, left as it is |
| `Match ...`, `Host *.internal` | left as they are, listed under "not imported"; they still apply |
| a block in a file outside `~/.ssh` (e.g. `~/.orbstack/ssh/config`) | not imported; that tool owns it |

The managed block goes where your first adopted host was, so what you wrote after your hosts (a
`Host *` of defaults) is still read after them. For the options authama's own `Host *` sets
(`ServerAliveInterval`, `AddKeysToAgent`, `IdentitiesOnly`, `HashKnownHosts`, ...), import copies
your values into `defaults.global_options`, so they still apply. Importing another machine's copy
edits nothing.

### A home onboarded before import adopted

An `import` from before this change left your blocks beside the managed ones, and `doctor` reports
each as a config collision. Move the duplicates out:

```sh
authama ssh config adopt --dry-run   # which blocks the manifest already reproduces, as a diff
authama ssh config adopt             # snapshot, back up, remove only those
```

A block that differs from the manifest is kept and listed with the line that differs: fix the
manifest (`authama ssh host edit`) or the block, then run it again. A second run changes nothing.

Review before reconciling — the derived profile names are a guess:

```sh
authama ssh list
authama ssh profile edit imported --key-scope per_service
```

In the web UI, *Import an ssh config* on the Profiles page offers this machine's `~/.ssh/config`
and the config home's `ssh/config`, and previews the import first. The preview lists every key it
would copy in, without copying one. Another machine's copy is imported from the terminal.

A second `import` replaces the manifest, so it is safe to re-run after editing
the source config. It will not overwrite keys it adopted the first time, and the hosts already in
the managed block are read from it, so a re-run moves nothing.

---

[← Docs index](../../README.md#documentation)
