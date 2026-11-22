# Move to a new machine

Everything - profiles, hosts, keys and token values - from the old laptop to the new one, in one encrypted file.

On the old machine:

```sh
authama setup export --encrypt --out ~/Desktop/setup.age
# owner PIN (if set), then a passphrase of at least 12 characters, twice
```

Copy `setup.age` across (AirDrop, a USB stick, a shared drive: it is encrypted). Give the passphrase another
way - say it, or keep it in your password manager.

On the new machine, after [installing authama](../installation.md):

```sh
authama setup import ~/Downloads/setup.age --dry-run   # every key file, config change and token name it would write
authama setup import ~/Downloads/setup.age             # the same, then asks
ssh -T git@github.com                                  # the same keys, so nothing to redeploy
```

The import adds what the new machine lacks and leaves anything already there that differs (`--on-conflict keep`);
use `--on-conflict replace` to take the old machine's versions (each replaced key file and `.env` is kept in the
state home's `replaced/` folder first), or `--on-conflict fail` to stop if anything differs.

## Only the setup, no secrets

To share a setup - with a teammate, or into a dotfiles repository - export the document instead:

```sh
authama setup export --out setup.json        # profiles, hosts, settings, token variable names; no key, no value
authama setup import setup.json --dry-run    # on the other machine
authama ssh reconcile                        # make the keys it declares, fresh
```

The web UI's **History → Move setup** page downloads and imports the document too. **Export / Import**, the
header button beside **New**, does the encrypted export in the browser as well. It downloads the age file behind
your PIN, and on the new machine imports it from an upload with its passphrase, listing the keys and token values it
would lay down by name only before you confirm.

See [setup](../tools/setup.md) for what each form holds and how the encrypted file is made.
