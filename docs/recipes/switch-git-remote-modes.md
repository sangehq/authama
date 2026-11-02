# Switch checkouts to profile SSH (or back to HTTPS)

Move a tree of checkouts from HTTPS to each owner's own SSH key, safely, and step back if you need to.

Map owners to profiles once, in the manifest:

```jsonc
"defaults": {"git": {"profile": "alice", "orgs": {"acme": "alice", "acme-labs": "alice"}}}
```

Then preview, apply, and check:

```sh
authama ssh git use profile ~/code/acme/* --dry-run
authama ssh git use profile ~/code/acme/* --yes
authama ssh git status ~/code/acme/*
```

Each new URL is checked with `git ls-remote` before `origin` changes, so a checkout whose key is not on
the account stays on its old remote and is named in the error. To go back: `authama ssh git use https …`.

Plain `git@github.com` remotes (`ssh` mode) use the `defaults.git.profile` key through the rendered
`Host github.com`; client repositories belong in `profile` mode, so they never use your key.

Reference: [`git`](../tools/git.md).

---

[← Docs index](../../README.md#documentation)
