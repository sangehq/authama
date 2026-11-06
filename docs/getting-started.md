# Getting started

A first run end to end: from an empty machine to a deployed key, and what each
step is actually doing, then the auth tokens your package managers and git need.
SSH commands live under `authama ssh`; tokens under `authama token`.

## The mental model

Profiles model **identity**, not technology. `work`, `personal`, `acme`,
`development` are profiles; "github" is not. The same hostname can appear in two
profiles and get a different key in each — that is the point, and it is what lets
two accounts on one forge coexist without either being able to authenticate as
the other.

The manifest is the single source of truth. `~/.ssh` is **generated output**:
`reconcile` rebuilds it, and anything the tool did not write is left alone.

## First run

```sh
authama init                  # create ~/.config/authama (dirs, perms, seed files)
```

`init` seeds an **empty** manifest. Add profiles and hosts before `reconcile`
mints anything — three ways, whichever suits:

```sh
authama ssh profile add work
authama ssh host add work gh -H github.com -u git --provider github   # or edit manifest.json
authama ssh host apply work acme-prd-web-00 203.0.113.10 deploy web   # add-or-update from a compact spec
```

`host apply` takes `<alias> <host>[:port] <user> [role]` and adds the host if
it is new or updates it if not, so the same line can be re-run. Server aliases
follow `<code>-<env>-<role>-<nn>` (`acme-prd-web-00`); see
[Naming servers](configuration.md#naming-servers). Many at once:

```sh
authama ssh host apply work \
  --host "acme-prd-web-00 203.0.113.10 deploy web" \
  --host "acme-prd-db-00  203.0.113.11 deploy db" --reconcile
authama ssh host apply work -f hosts.txt     # one spec per line, # comments, - for stdin
authama ssh import ~/.ssh/config                        # or onboard an existing setup
```

Then build the tree:

```sh
authama ssh reconcile --dry-run   # preview: what would be minted, what would be written
authama ssh reconcile             # build ~/.ssh from the manifest
authama ssh config check          # confirm the file matches the manifest (exit != 0 on drift)
```

At this point every declared key exists at `~/.ssh/profiles/<profile>/<key>`
(`profiles/<category>/<profile>/<key>` for a profile with a
[category](configuration.md#categories)), `~/.ssh/config` holds a managed block
with one `Host` entry per host, and reachable hosts have been pinned into
`~/.ssh/known_hosts`. A profile with `key_store: 1password` has only the `.pub`
on disk; its private key lives in a 1Password vault
([Key stores](configuration.md#key-stores)).

## Getting a key onto its target

```sh
authama ssh deploy work_gh-ed25519    # ssh-copy-id, gh, glab, a REST API, or manual
authama ssh validate                  # every keypair parses and the halves match
authama doctor                    # deps, perms, agent, pins, drift
```

`deploy` picks its method from the host's provider. Where there is no API — a
web panel, an unknown forge — it prints the page to paste the key into rather
than pretending to have done it.

## Living with it

```sh
authama ssh list --type vcs         # filterable view across profiles
authama ssh show gh                 # manifest, key files, rendered config and pins, reconciled
authama ssh expiry                  # per-key rotation age
authama ssh rotate work_gh-ed25519  # zero-downtime staged rotation
authama ssh notify install          # scheduled reminders before keys come due
```

**Every verb that changes `~/.ssh` asks first.** `reconcile` shows its dry run as
the question, so you approve what will happen rather than a verb name. `--yes`/`-y`
answers in advance, and the deletes take `--revoke` to pull the key off its
targets as well.

The prompt only appears when there is a terminal to ask at. In a script, a cron
job or a CI step there is nobody to answer, so commands proceed — declining on
their own would turn every existing script into a silent no-op, which is not
safer than running, only harder to diagnose. Pass `--yes` there anyway: it says
so explicitly instead of relying on the absence of a terminal.

## Auth tokens

The second feature writes the credential files Composer, npm, Docker, git and
curl read. Its config names environment variables, never the secrets themselves:

```sh
authama token init                  # a starter ~/.config/authama/tokens.json
$EDITOR ~/.config/authama/tokens.json
export GH_TOKEN="$(op read op://Private/GitHub/token)"   # or put GH_TOKEN="op://Private/GitHub/token" in ~/.config/authama/.env, or set it in CI
authama token status                # which variables are still missing
authama token apply --dry-run       # what would be written, secrets masked
authama token apply                 # write auth.json, .npmrc, ... (0600, merged)
authama token list                  # what is installed now
```

One token without a config: `authama token set --target git --server github.com
--username you` reads it from a hidden prompt. `authama token audit` (and
`authama doctor`) confirm every file is owner-only. Try any of it first with
`--dev-root ./sandbox`, which keeps every file in the sandbox. Coming from
auth-token-manager, run `authama token import`; see the
[migration guide](migration.md) and [`token`](tools/tokens.md).

## Passphrases

Off by default, always a conscious choice. `--passphrase` prompts without echo
and confirms; leaving it off mints the key without one, and nothing prompts. The passphrase reaches
`ssh-keygen` through the askpass protocol, never through the command line, so it
does not appear in the process list.

## Safety: what is always on

Every mutating command passes through one guard that takes an advisory lock,
sweeps crash residue, and snapshots `~/.ssh` before changing anything. Combined
with atomic writes, the tree is always left internally consistent and any run is
reversible.

There are four recovery layers, and they are not interchangeable:

| Layer | Recovers | When |
|---|---|---|
| `snapshots restore` | the config tree | an edit went wrong; local, last 10 kept |
| `rollback <key>` | one key's predecessor | a rotation deployed badly |
| `restore <bundle>` | the keys themselves | a new machine, or the old one is gone |
| `recover <key>` | access to a server | you are locked out entirely |

**Snapshots deliberately carry no private keys.** One is written before every
mutating command and several are kept, so a private key in one would be a rolling
plaintext archive of your keys. The encrypted `bundle` is the path that recovers
key material, and `recover` is the break-glass escape hatch that needs no working
SSH at all.

---

[← Docs index](../README.md#documentation)
