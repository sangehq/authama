# Back up, and restore onto a new machine

The only path that recovers **the same keys**. Snapshots do not: they carry no
private key material by design.

Create an age identity and keep it somewhere other than the machine you are
backing up:

```sh
age-keygen -o ~/age-identity.txt        # note the "public key:" line
authama ssh bundle -r age1... -o ~/Backups
```

The bundle holds the private keys, the manifest, the inventory and
`providers.json`. It **never** holds `.env` — that is where provider API tokens
live, and a backup you might copy to another machine is the wrong place for them.
The archive is encrypted before it reaches the disk; there is no plaintext
staging copy in `$TMPDIR`.

On the new machine:

```sh
authama init
authama ssh restore ~/Backups/authama-*.age -i ~/age-identity.txt
authama ssh validate                          # every pair parses and matches
authama doctor
```

Keys kept in 1Password (`key_store: 1password`) are not in the bundle: only
their `.pub` is on disk, and the private key is already backed up by 1Password.
On the new machine, sign in to 1Password and run `authama ssh key regen-pub --all`
(or `authama ssh reconcile`) to bring their public halves back from the vault.

`restore` overlays: a key already on disk that the bundle does not contain — one
minted since the backup — survives. It replaces only what it has a copy of.

> Set `AUTHAMA_AGE_RECIPIENT` and `AUTHAMA_AGE_IDENTITY_FILE` to skip the
> flags. The recipient is also what lets `keygen --force` write a backup before
> destroying a key, instead of refusing.

Reference: [`bundle`](../tools/bundle.md).

---

[← Docs index](../../README.md#documentation)
