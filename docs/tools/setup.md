# setup export / import - move a setup between machines

```sh
authama setup export [--out FILE]                                  # the setup document: no secret in it
authama setup export --encrypt --out FILE [--passphrase-file F]    # everything: keys and token values too
authama setup import FILE [--dry-run] [--on-conflict keep|replace|fail] [--passphrase-file F] [-y]
```

Two forms, for two jobs:

| | Setup document | Full export (`--encrypt`) |
|---|---|---|
| File | JSON | an [age](https://age-encryption.org) file, passphrase-encrypted |
| Manifest (profiles, hosts, defaults, reserved keys) | yes | yes |
| `providers.json` | yes | yes |
| Settings (`authama.json`) | yes, without the `paths` section | yes, without the `paths` section |
| Token config (`tokens.json`, names of variables) | yes | yes |
| Names of the token variables, profiles and keys | yes, for the reader | - |
| Inventory (key records) | no | yes |
| Private keys under the key root | no | yes |
| Token values (from the environment and `.env`) | no | yes |
| Owner PIN | not needed | asked for, when one is set, to export and to import |
| Safe to commit / email | yes | only with the passphrase kept apart |

The `paths` section of `authama.json` (state home, identity file, token config path) names places on one machine,
so neither form carries it and an import never writes it.

## The setup document

```sh
authama setup export --out setup.json     # or to stdout
```

```json
{
  "format": "authama-setup",
  "version": 1,
  "exported_at": "2026-11-22T12:00:00Z",
  "authama": "v0.1.0",
  "secrets": false,
  "manifest": { "version": 1, "defaults": { … }, "profiles": { … } },
  "providers": { … },
  "tokens": { "composer": { "github.com": { "type": "github-oauth", "env": "GH_TOKEN" } } },
  "settings": { "backup": { "snapshot_retain": 10 } },
  "env_names": ["GH_TOKEN"],
  "profiles": ["work", "personal"],
  "keys": ["work/work_github-ed25519", "personal/personal_github-ed25519"]
}
```

A token config holding anything but variable names is not exported (the same check `token` and the web UI's
editor make), and an import refuses one too, whatever `secrets` says. On the new machine
`authama ssh reconcile` makes the keys the manifest declares, and `authama token set` (or `.env`) supplies the
values; or carry them in the full export.

## The full export

```sh
authama setup export --encrypt --out ~/Desktop/setup.age
```

It asks for the owner PIN when one is set, then for a passphrase (twice; at least 12 characters), and writes
the file owner-only, refusing to replace one that exists. `--passphrase-file` reads the passphrase from an
owner-only file instead, for scripts. The variables whose values it carries are the ones the token config and
the hosts' `token_env` name, read from the environment after `.env` is loaded; a variable with no value is named
in the output and left out.

Inside is the backup bundle's archive ([bundle](bundle.md)): the private keys under the key root (never a
staging folder or a rendered config), `manifest.json`, `inventory.json` and `providers.json`, plus
`config/tokens.json`, `config/authama.json` and `secrets/tokens.env`. The token values are put into the archive
from memory and the archive is streamed into the cipher: no plaintext copy of a key or a value is written
anywhere.

### Why age with a passphrase, and no new dependency

The file is the [age v1 format](https://age-encryption.org/v1) with its scrypt recipient: exactly what
`age -p` writes, so `age -d setup.age | tar tz` lists it on any machine, and authama reads files `age -p` made.
age is a published, reviewed format, and authama's backup bundles already use it. What age does not offer is a
passphrase from anywhere but a terminal (it reads one only from a TTY), so authama does not shell out for this;
it implements the scrypt recipient itself (`src/internal/util/agepass`) with `golang.org/x/crypto`, already a
dependency - scrypt (N=2^18, r=8, p=1), HKDF-SHA-256 and ChaCha20-Poly1305 - which are the primitives the format
specifies. The construction is age's: a random file key wrapped under the scrypt key, a header MAC, and the
payload in 64 KiB authenticated chunks whose last one is marked, so a wrong passphrase, a changed header, a
flipped bit, a truncated or extended file are each refused before anything is written. The tests cover those;
interoperability with the `age` binary was checked in both directions.

## Import

```sh
authama setup import setup.json --dry-run          # what it would add, keep and replace
authama setup import setup.json                    # the same, then asks
authama setup import setup.age --on-conflict replace
```

Import tells the two forms apart by their first bytes. It shows, before writing anything:

- every item it would **add** (new here), **replace**, or leave because it is **kept** (differs; this home's
  wins) - profiles one by one, reserved keys one by one, `defaults`, the providers and token config as whole
  files, and settings one key at a time;
- the diff of every config file it changes, `~/.ssh/config` included (the same preview the web UI makes, run on
  a throwaway copy of the home);
- for a full export, each key file by its path (never its content), and each token variable by name (never its
  value).

`--on-conflict` settles what differs:

- `keep` (the default) adds what is new and leaves what differs as it is here;
- `replace` takes the import's. Each key file it replaces, and `.env` before a value in it changes, is copied
  first into the state home's `replaced/<time>/`, owner-only; config files are in the snapshot taken before every
  change;
- `fail` writes nothing at all if anything differs, and names what.

A manifest that sets SSH options that could redirect or weaken connections (a `ControlPath` or `IdentityAgent`
outside `~/.ssh`, `ForwardAgent` or `StrictHostKeyChecking no` for every host) is refused, as `ssh restore`
refuses one, unless you pass `--allow-risky-options` for a file you made yourself: neither form proves who made it.
The same goes for settings that declare custom hypervisor commands (authama runs them whenever it lists VMs) and for
a `providers.json` that would send a token to a host this build's catalog does not; the refusal names each command
and host. A full export's `.env` brings only the variables the setup reads tokens from; any other name, and any that
changes how programs run (`GIT_*`, `SSH_*`, `LD_*`, `DYLD_*`, `AUTHAMA_*`, `PATH`, proxies), is listed as not
imported. A token value that runs a command when it is read (`cmd:<command>`, or an `op://` reference read with
`op read`) is refused the same way, naming the variable and the command: your own export carries yours, so pass
`--allow-risky-options` for it at a terminal.
So are settings that, chosen by someone else, would work against you: a `backup.age_recipient` (your key backups
would be encrypted to it), `history.enabled` off, `vm.repin_changed` on, and turning on `api`, `mcp` or `webhooks`
(including `allow_private` and `include_details`). A value that matches the default is not flagged.
A signature does: `setup export --out FILE --sign-key KEY` writes `FILE.sig`, and `setup import` checks it,
refusing a file changed after it was signed (`--require-signature` and `--signers` as for `ssh restore`; see
[signed bundles](../reference/bundle-signatures.md)).

Keys land 0600 in folders 0700, `.env` stays 0600, and the config files are written owner-only. Token values are
merged into `.env` double-quoted, every other line of it kept as it was. The import is recorded in the history
(`authama history --command "setup import"`) with a `setup.import` event, and the snapshot taken first restores
the config with `authama ssh snapshots restore`. `ssh restore` given a full export points here, and
`setup import` given a backup bundle (encrypted to a key, not a passphrase) points to `ssh restore`.

## In the web UI

**History → Move setup** (`/setup`, `g m`) downloads the setup document, lists what it holds, and imports a
pasted one through the usual preview and confirm, with the same three conflict choices. The import is recorded with
the web as its source. A document with risky options is refused there.

**Export / Import** (the header button beside **New**, `/transfer`, `g f`) does the rest:

- **Encrypted export** downloads the full export under a passphrase typed twice, behind the owner PIN. It goes
  straight to the download, and nothing of it is shown.
- **Import** takes an uploaded setup document, an [export zip](export.md#export-pack) (for its `setup.json`), or a
  full export with its passphrase. It lists every key file and token value it would bring by name and state only,
  then goes through the same preview and confirm. The upload and its passphrase wait in the session's secret stash
  for 15 minutes and are used once. They are never written into a page.

`setup import` at a terminal takes an export zip too. A full export with risky options is refused in the browser; import it at a
terminal with `--allow-risky-options` if you set them yourself.

Recipe: [Move to a new machine](../recipes/move-to-a-new-machine.md).
