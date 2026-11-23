# ssh tool - keys used by tools

```sh
authama ssh tool list [--json]
authama ssh tool show <profile>/<key>
authama ssh tool status [--json]
```

A tool key lives under `~/.ssh/profiles/<category>/<profile>/tools/<tool>/`, apart from host keys at the profile
root. Built-in slugs include `git`, `docker`, `ploi`, `forge` and `ansible`. Moving a key there is
[`ssh structure classify`](structure.md) and approve; this command lists what the manifest already declares.

Git signing keys belong in `tools/git/`. After a move, [sange](https://github.com/sangehq/sange) updates
`user.signingkey` (`sange profile signing retarget`). Authama does not write `~/.gitconfig`.

## In the web UI

The **Tools** page (`/tools`) lists each declared tool key, its path, and whether the file is present. Use
[Structure](structure.md#in-the-web-ui) to classify and approve a move, including by dragging a key onto a
profile's `tools/<tool>/` folder.
