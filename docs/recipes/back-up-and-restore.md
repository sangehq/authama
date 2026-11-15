# Back up, and restore onto a new machine

The only path that recovers **the same keys**. Snapshots do not: they carry no
private key material by design.

Make an age key for backups, and keep a copy of its identity somewhere other than
the machine you are backing up:

```sh
authama settings backup-key             # makes the key, sets backup.age_recipient and paths.age_identity_file
authama ssh bundle -o ~/Backups
```

`backup-key` runs `age-keygen` for you and keeps the identity owner-only in the config home
(`backup/age-identity.txt`, or where `paths.age_identity_file` already points). Without a
copy of that file elsewhere, no backup can be decrypted. To use an age key you already have,
set its public key instead: `authama settings set backup.age_recipient age1...`.

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
minted since the backup — survives. It replaces only what it has a copy of, and
keeps each file it replaces with different content (a key rotated since the backup,
say) in the state home under `replaced/<stamp>/` first; the summary says where.

> Set `AUTHAMA_AGE_RECIPIENT` and `AUTHAMA_AGE_IDENTITY_FILE` to skip the
> flags. The recipient is also what lets `keygen --force` write a backup before
> destroying a key, instead of refusing.

Reference: [`bundle`](../tools/bundle.md).

---

[← Docs index](../../README.md#documentation)
