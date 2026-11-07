# Keep your keys in a directory of your choosing

Move the profile directories from `~/.ssh/profiles/` to another directory under `~/.ssh`, such as
`~/.ssh/keys/`, with one command.

```sh
authama ssh key root          # where they are now: ~/.ssh/profiles/
authama ssh key root keys     # move them to ~/.ssh/keys/
```

The move snapshots `~/.ssh`, renames the whole tree in one step (keys, `.pub` files, `old/` archives,
category config files), re-points the inventory and renders `~/.ssh/config` again, so every
`IdentityFile` and `Include` names the new place:

```
Match all
Include ~/.ssh/keys/clients/config

Host gh
    IdentityFile ~/.ssh/keys/work/work_gh-ed25519
```

Keys you adopted where they already were (`authama ssh key adopt`) stay put. To go back, run
`authama ssh key root profiles`.

In the web UI, *Move the key root* on the Keys page does the same. You type the new name twice, and
the preview lists every key the rename would move before anything does.

If you set `defaults.key_root` by editing the manifest instead, the keys stay behind in
`~/.ssh/profiles/`. `authama doctor` reports them and names the two commands that bring them over.

The rules for the name are in [Where the keys live](../configuration.md#key-root).

---

[← Docs index](../../README.md#documentation)
