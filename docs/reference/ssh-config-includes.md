# SSH configs in any folder layout

authama works with any `~/.ssh` layout, not only the managed
`profiles/<category>/<profile>/` tree it writes. It reads your ssh config the
way OpenSSH does, follows every `Include`, and discovers keys in nested
folders. It never rewrites a file it does not manage: those files are reported
so you can update them by hand.

## Include directives

`authama ssh config tree` (and the Include tree on the SSH config page) walks
`~/.ssh/config` and every file it Includes:

- Relative paths are resolved against `~/.ssh`, as ssh does for a user config
  (not against the including file's directory).
- `~` and `~user` are expanded. Globs (`*`, `?`, character classes) are
  expanded in lexical order. A `**` path segment matches every directory
  beneath the prefix, including the prefix itself, so `config.d/**/*.conf`
  works.
- Includes nest, including those inside a `Host` or `Match` block, up to
  OpenSSH's limit of 16 levels. A cycle or a missing file is a warning, not
  an error; the rest of the tree is still shown.
- Each Host is listed with the file and line it was written on, so a hand
  edit, `keep-edits`, or a check targets the right file.

`--json` prints the same tree for scripts. `ssh config check` repeats Include
warnings (they do not fail the check by themselves). A Host in a file
authama does not manage that redefines a managed alias is reported as a
`COLLISION`; `ssh config adopt` still only edits `~/.ssh/config`.

## Keys in nested folders

Structure classification walks all of `~/.ssh`, not only the top of the
directory and the key root. It skips `known_hosts*`, `authorized_keys*`,
agent directories, ControlPath directories (`authama-cm`), sockets, and
config files. Symlinks are followed without looping. A public key is read
to pair it; a private key is never read.

When keys already sit together in a folder of yours (two or more pairs, or a
config alongside them), authama keeps that grouping and only mentions the
managed `profiles/<category>/<profile>/` taxonomy as an optional alternative,
with evidence (how many keys, which IdentityFile lines name them). A lone
key in an otherwise empty folder is suggested as a move into a profile, the
same as a key loose at the top of `~/.ssh`.

When a key moves, IdentityFile and CertificateFile lines are rewritten only
in files authama manages (`~/.ssh/config` and files that open with the
managed header). A reference in any other Included file is listed as a
manual follow-up.

## The watcher

`authama serve` watches every directory that contributes an Included file or
a key, including trees outside `~/.ssh` (for example `~/.orbstack/ssh` when
`~/.ssh/config` Includes it). A new Include is picked up after the debounce
without a restart. The 30-second poll is still the fallback.

## See also

- [CLI and web parity](cli-web-parity.md)
- [SSH folder structure](../tools/structure.md)
- [Onboard an existing ssh dir](../recipes/onboard-an-existing-ssh-dir.md)
