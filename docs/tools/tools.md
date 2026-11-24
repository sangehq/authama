# ssh tool - keys used by tools

```sh
authama ssh tool list [--json]
authama ssh tool show <profile>/<key>
authama ssh tool status [--json]
```

A tool key lives in the shared folder `~/.ssh/profiles/tools/<tool>/`, apart from the host keys in each profile's
folder. It stays declared under the profile that owns it (a personal, org or client profile), so every tool key is
still linked to its owner, and the Tools page and the folder diagram group them that way. Built-in slugs include
`git`, `docker`, `ploi`, `forge` and `ansible`. Moving a key there is [`ssh structure classify`](structure.md) and
approve; this command lists what the manifest already declares.

```text
~/.ssh/profiles/
├── personal/imanimanyara/   host keys of the imanimanyara profile
└── tools/                   no config: used directly by tools
    ├── docker/docker-tunnel     owned by imanimanyara
    └── git/git-signing          owned by imanimanyara
```

`tools` is reserved under the key root: no category, and no uncategorised profile, may take the name. Tool keys
share the folder, so two profiles cannot declare the same `tools/<tool>/<name>`; give one a distinct name
(`acme-git-signing`).

**Moving from the older layout.** Before 2026-11-24 a tool key lived in its profile's own `tools/<tool>/`
folder. [Structure](structure.md) finds a key still there and offers a `misplaced` move to the shared folder;
approving it moves both halves (backed up first), repoints `IdentityFile` lines, keeps the key's inventory
record and deployments at the new path, and hands a git signing key to sange to retarget `user.signingkey`.

Git signing keys belong in `tools/git/`. After a move, [sange](https://github.com/sangehq/sange) updates
`user.signingkey` (`sange profile signing retarget`). Authama does not write `~/.gitconfig`.

## In the web UI

The **Tools** page (`/tools`) lists each declared tool key, its path, and whether the file is present. Use
[Structure](structure.md#in-the-web-ui) to classify and approve a move, including by dragging a key onto a
profile's `tools/<tool>/` folder.
