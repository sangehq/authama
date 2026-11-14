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

Relative `Include`s resolve against the folder, as ssh resolves them against `~/.ssh`. From another
machine's copy, an `IdentityFile ~/.ssh/<key>` is looked for in that copy, so its keys are adopted
here. A key that already
lives under `profiles/<name>/` is referenced where it is; one outside the managed
tree is **copied** in, never moved, and the copy is made owner-only whatever mode
the original had.

> Nothing is regenerated. `import` never mints over an existing key, so the
> identities your servers already trust keep working.

`import` does not edit the config it read. Its summary names the hand-written `Host` blocks it
copied, which stay where they are: ssh reads them before the managed block, so `authama doctor`
reports them as shadowed until you remove them from that file. It also lists, as written, each
`Match` block and each `Host` block of wildcards only (`Host *.internal`): authama manages plain
`Host` blocks, so those are not imported, and they stay in your config and still apply.

Review before reconciling — the derived profile names are a guess:

```sh
authama ssh list
authama ssh profile edit imported --key-scope per_service
```

In the web UI, *Import an ssh config* on the Profiles page offers this machine's `~/.ssh/config`
and the config home's `ssh/config`, and previews the import first. The preview lists every key it
would copy in, without copying one. Another machine's copy is imported from the terminal.

A second `import` replaces the manifest, so it is safe to re-run after editing
the source config. It will not overwrite keys it adopted the first time.

---

[← Docs index](../../README.md#documentation)
