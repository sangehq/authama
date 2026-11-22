# Sign commits with each profile's SSH key

Work commits made as your work address and signed with your work key; personal ones as yourself, with
your personal key. One manifest block per profile does both, for every checkout under its directories.

```sh
# the keys the profiles already use for GitHub sign too
authama ssh git sign set work     --email ada@acme.example --name "Ada Lovelace" --dir ~/code/acme
authama ssh git sign set personal --email ada@example.com  --name "Ada Lovelace" --dir ~/code/personal
authama ssh git sign status        # each profile's key, and whether the config is applied
```

`set` shows the diff of what it writes (a fragment per profile, one `allowed_signers`, and an `includeIf` block
in `~/.gitconfig`) and asks first. From then on:

```sh
cd ~/code/acme/app
git commit -m "Fix the thing"            # made as ada@acme.example, signed with the work key
authama ssh git sign verify              # G = good signature from an allowed signer
git log --show-signature -1              # git's own view, through the same allowed_signers
```

A checkout outside those directories joins a profile on its own:

```sh
authama ssh git sign use personal ~/src/dotfiles
```

And a fresh clone gets the right remote and identity in one step:

```sh
authama ssh git clone acme/service ~/code/acme/service
```

For GitHub to show **Verified**, add each profile's public key to its GitHub account a second time, as a
*signing* key (Settings → SSH and GPG keys → New SSH key → Key type: Signing Key):

```sh
authama ssh show work            # the key's path; or copy the .pub from the key's page in the web UI
```

To stop signing for a profile but keep its identity: `authama ssh git sign set work --no-sign`. To take it away
altogether: `authama ssh git sign set work --clear`; the includes and the fragment go with it, and the rest of
your `~/.gitconfig` is untouched.

Remotes per profile (`git@github-work:acme/app.git`) are covered in
[Switch checkouts to profile SSH](switch-git-remote-modes.md) and
[Two GitHub accounts on one machine](two-github-accounts.md). Full reference: [tools/git.md](../tools/git.md).

---

[← Docs index](../../README.md#documentation)
