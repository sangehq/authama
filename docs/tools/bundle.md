# bundle / restore - encrypted backup & migration

```sh
authama ssh bundle [--recipient age1...] [--output DIR] [--keep N]
authama ssh restore <bundle.age> [--identity age-identity.txt]
```

For moving to a new machine with a passphrase instead of an age key - token values included - see
[setup](setup.md).

Uses **age** (X25519 + ChaCha20-Poly1305) - modern, single-binary, far simpler
than GPG. Install it first: `brew install age` (macOS) / `apt install age` (Linux).

## What's in a bundle

`bundle` tars and age-encrypts:

- the **private keys** under `~/.ssh/profiles/**` (excluding `.staging/`); a
  profile with `key_store: 1password` has none on disk, so only its `.pub` is
  included and its keys stay in 1Password
- `manifest.json`, `inventory.json`, `providers.json`

It does not carry the files authama renders from the manifest: a category's
`profiles/<category>/config` and any legacy `profiles/<profile>/known_hosts`.
`restore` rebuilds them from the restored manifest, so a bundle cannot plant a
config that `~/.ssh/config` Includes. An older bundle that carries them still
restores; those members are skipped and named in the summary.

...into a single `authama-<stamp>.age`, alongside two sidecars:

- `*.age.sha256` - checksum (verified before any restore)
- `*.age.contents` - plaintext list of filenames

Without `--output`, bundles are written to the state home's `dist/` (e.g.
`~/.local/state/authama/dist/`), owner-only (0700). Pass `--output DIR` to write
elsewhere (e.g. an external drive).

**`.env` is deliberately excluded** - it holds the age recipient/identity refs
that unlock the bundle, so bundling it would be circular. Back `.env` up
separately in your password manager.

## Recipient (where the decryption identity lives)

Set the recipient via `--recipient` or `$AUTHAMA_AGE_RECIPIENT`. On restore, the
decryption identity comes from `--identity` or `$AUTHAMA_AGE_IDENTITY_FILE` (a path
to an age identity file). Where you keep that identity is up to you:

1. **Password manager** - store the age identity in 1Password/Bitwarden and write
   it to a file (e.g. `op read ... > id.txt`) before pointing `--identity` at it.
2. **Hardware key** - `age-plugin-yubikey`, so decryption needs the physical
   device + touch.

age has **no revocation** - rotate a bundle by re-encrypting to a new recipient
and destroying the old.

## restore = the same keys (contrast with reconcile)

| | source | brings back |
|---|---|---|
| **`restore`** | the encrypted bundle | the **same** keys (same fingerprint) - true recovery |
| **`reconcile`** | the manifest only | rebuilds structure + **mints new** keys for any missing |

`restore` snapshots `~/.ssh` first, decrypts, lays the keys back down, re-asserts
perms (700/600/644), and re-renders the config from the restored manifest so `~/.ssh`
is immediately usable. A snapshot never holds private keys, so before it writes
anything, `restore` also keeps every file it is about to replace with different
content (a key rotated since the bundle was made, the manifest, the inventory) in
the state home under `replaced/<stamp>/`, as `ssh/<path>` and `config/<file>`,
owner-only. The summary lists them. Nothing that a restore overwrites is lost. Then `authama ssh load <profile>` to add
keys to the agent.

A bundle does not prove who made it: anyone holding your age recipient can make one that decrypts
with your identity. So `restore` decrypts and reads the whole bundle before it writes anything, and
refuses one whose manifest sets an SSH option that could redirect or weaken your connections, naming
each option, where it is and why:

| option | refused when |
|---|---|
| `ControlPath` | it is outside `~/.ssh` (or `%d/.ssh`) and `$XDG_RUNTIME_DIR`, or holds `..`: another local user could host the shared connection |
| `IdentityAgent` | it is not `none`, `SSH_AUTH_SOCK`, the 1Password agent, or a socket under those directories: another agent would sign as you |
| `ForwardAgent` | it is anything but `no` in `global_options`, for every host |
| `StrictHostKeyChecking` | it is `no` or `off` in `global_options`, for every host |

If you set them yourself, `authama ssh restore <bundle> --allow-risky-options` restores it: the
question before it writes lists the options, and the summary says which it laid down. The web UI has
no such switch; it shows the refusal and the command. A manifest you edit by hand is not checked:
these options are yours to set.

```sh
authama settings backup-key                # once: makes an age key and sets the recipient (or set
                                           # AUTHAMA_AGE_RECIPIENT="age1..." to a key you have)
authama ssh bundle -o ~/Backups            # writes ~/Backups/authama-<stamp>.age (+ sidecars)
# ... new machine ...
authama ssh restore ~/Backups/authama-<stamp>.age -i age-identity.txt
```

---

[← Docs index](../../README.md#documentation)
