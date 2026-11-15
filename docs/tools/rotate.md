# rotate / rollback - zero-downtime key rotation

```sh
authama ssh rotate <key> [--allow-unverified] [--passphrase] [--yes]
authama ssh rotate [<key>|<profile>|<alias> ...] [--category <name>] [--all] [--dry-run]
authama ssh rollback <key> [--yes]
```

Rotation never leaves a `...-ed25519.new` sibling: the active key keeps its
canonical name throughout, and the outgoing key is archived under the **identical
filename** - so swap-back is a plain move and there's never a name collision.

## The flow (invariant 7)

1. **Stage** a replacement in the profile's `.staging/` (never left behind).
2. **Deploy** the staged public key to every target (the current key is still
   active and authorized - nothing breaks here).
3. **Verify** login with the staged key on each target (`ssh ... true`; `ssh -T`
   for GitHub).
4. **Commit - only after all verify:**
   - purge any existing predecessor in `/old/` (enforces **≤1 old per key**); a rotation stops
     before this point when that predecessor is still authorised on a host it was never removed
     from (as its retirement recorded), naming the hosts, and `--evict-old` replaces it anyway,
   - move the current `<name>(.pub)` → `old/<name>(.pub)` (same filename),
   - promote the staged key to the canonical name,
   - **revoke** the old public key from each target,
   - reset the inventory (new fingerprint, fresh `created`/`expires_on`) + audit.
5. **On any failure before commit:** the staged key is discarded and the active
   key and its files are untouched - true zero-downtime.

`~/.ssh` is snapshotted before the rotation starts (the mutation guard), so the
whole operation is reversible with `authama ssh snapshots restore` as well.

## Verification & manual targets

Each target must verify before commit. Automated providers (generic SSH, GitHub
with a token) verify by logging in / `ssh -T`.

A **hosting panel** target (`ploi`, `forge`, `cpanel`) takes the key by hand.
At a terminal, `rotate` stops there, shows the staged public key and where it
goes, and waits:

```text
app-web: this target takes the key by hand. paste acme_app-web-ed25519.pub at https://ploi.io/servers

ssh-ed25519 AAAAC3Nza... acme/app-web 2026-10-20

  Pasted it? authama will now log in with the new key to prove it [y/N]
```

After `y` it logs in **with the staged key itself**; only a working login
counts as verified, and only then does the new key replace the old one. If the
login fails, or you answer `n`, the staged key is discarded and the old key
stays active. Once committed, the report names where the **old** key still has
to be removed from the panel: until you do, it still logs in.

Without a terminal (a script, CI), such a target cannot verify, so rotation
aborts. `--allow-unverified` commits anyway, before anything proves the key
works, so SSH to that server fails until the key is pasted or you
`authama ssh rollback`. Prefer running it at a terminal.

A git host on the manual path (`gitea`, `bitbucket` and the other `vcs`
web-panel entries) is never proven by logging in, because it refuses a shell
even for a valid key; it still needs `--allow-unverified`.

## Rotating a batch

`rotate` takes any number of keys, profiles or host aliases, every key in a
category (`--category orgs`), or every key (`--all`):

```sh
authama ssh rotate --all --dry-run          # list what would rotate
authama ssh rotate --category clients
authama ssh rotate acme alice/alice_github-ed25519
```

A batch asks once, writes one encrypted backup and one snapshot, then rotates
each key on its own with the flow above. A key that fails (an unreachable host,
a target that will not verify) is left exactly as it was, and the rest still
rotate; the command exits non-zero and names the keys it left. The inventory is
saved after every committed key, so an interrupted batch keeps what it
finished. A key no host uses is skipped: there is nothing to deploy it to.

A batch, and every dry run, ends with a **summary** of each key it covered:

```text
$ authama ssh rotate --all --check

Result   3 would fail · 3 would pause · 4 would rotate   of 10 keys · 4 owners

                       KEY                               VIA         RESULT        REASON
rotate
├─ simtabi                                                                 2 would fail · 3 would pause
│  ├─ simbi · prd                                                                          2 would fail
│  │  ├─ × web-00      simtabi_simbi-prd-web-00-ed25519  ssh         would fail    never deployed [2]
│  │  └─ × db-00       simtabi_simbi-prd-db-00-ed25519   ssh         would fail    never deployed [2]
│  └─ oribi                                                                               3 would pause
│     ├─ ! web         simtabi_oribi-web-ed25519         ploi        would pause   awaiting paste [1]
│     └─ ! db-maria    simtabi_oribi-db-maria-ed25519    ploi        would pause   awaiting paste [1]
│
└─ imanimanyara                                                           1 would fail · 1 would rotate
   ├─ × docker-tunnel  docker-tunnel                     ssh         would fail    host unreachable [3]
   └─ ✓ github         imanimanyara_github-ed25519       github api  would rotate

─────────────────────────────────────────────────────────────────────────────────────────────────────
Notes
  [1] ploi takes the new key by hand: the rotation will stop and ask you to paste it
  [2] answers, but the current key does not log in: it was never deployed here, so there is nothing to
      rotate yet; deploy it first (the key's page, Deploy)
  [3] incus:22 unreachable - check your network connection
  A dry run changes nothing: every key is as it was.
─────────────────────────────────────────────────────────────────────────────────────────────────────
Needs you
  × 1  check network  [3]   imanimanyara: docker-tunnel
  × 2  deploy first   [2]   simtabi/simbi·prd: web-00, db-00
  ! 3  paste by hand  [1]   simtabi/oribi: web, db-maria
─────────────────────────────────────────────────────────────────────────────────────────────────────
3 would fail · 3 would pause · 4 would rotate  ·  12.4s     re-run failed: authama ssh rotate --failed --check
```

- **The command** and a **Result** line open it: how many keys went each way, worst first, of
  how many keys and owners.
- **The tree** hangs every key off one trunk under `rotate`, grouped by owner (profile), then by
  project, read from each key's name: a name with an environment word (`prd`, `staging`, `dev`
  and the like) after its first part is grouped as `project · env`, and keys that share a first
  part are grouped under it. Owners, groups and keys are sorted worst first, and each owner and
  group carries its own counts on the right.
- **Each key** says its result in words (`rotated`, `replaced`, `not rotated`; for a dry run
  `would rotate`, `would pause`, `would fail`) with a ✓, ! or ×, the key's full name, how the new
  key reaches its hosts (`ssh`, a provider's API, or the provider that takes it by hand), and a
  short reason.
- **Notes** print each explanation once, numbered in the order the run met them, with a hanging
  indent, and say what became of the keys that did not go through.
- **Needs you** lists what is left to do (check the network, deploy first, paste by hand,
  deploy a forced key next), worst first, with the keys each is for.
- **The last line** repeats the counts and the time taken, and gives the command that runs the
  failed keys again: `authama ssh rotate --failed` runs the keys the last rotation or dry run
  left (add `--check` to test them again, `--force` to replace them).

The output is plain text, so a pipe or a log reads it as printed. The web's job page shows the
same summary, in colour, as the job's Output; every line the job printed is folded beneath it
under **Full log**.

A selector that matches nothing is an error, so a typo never runs as an empty
batch, and `--all` cannot be combined with other selectors.

## Forcing a rotation

A rotation replaces a key only once the new one is deployed and logs in, so a key
it cannot reach is left as it was. Two kinds of key can never rotate in place:
one that was **never deployed** to a host that uses it (there is no old login to
replace), and one whose **host cannot be reached** (a VM that is off, a network
that is down). `rotate --check` names both.

`--force` replaces those keys anyway, on this computer only, as
`authama ssh keygen <profile/key> --force` does for one key:

```sh
authama ssh rotate --all --force --yes
authama ssh rotate work/work_box-ed25519 --force
```

Every key that can rotate in place still does, with the flow above. Each key
that cannot is minted afresh, its current key moved to `old/`, and nothing is
deployed; the output marks it `next` and names the command that deploys it.

> **A forced rotation can lock you out until you deploy.** A host that trusted the
> old key does not trust the new one. Deploy each replaced key next
> (`authama ssh deploy <profile/key>`, or **Deploy** on its page), or roll it back
> with `authama ssh rollback <profile/key>`: the old key is kept under `old/`
> until the next rotation replaces it.

On the web, **Force: replace what cannot rotate in place** is an option on Rotate
keys and on a rotation's review. A forced review is titled *(forced)*, warns of
the lockout, starts from a red button, asks in a red dialog (**Yes, force it**),
and lists the rollback from `old/` as its undo. A dry run is never forced.

Revoking needs no force: `authama ssh key revoke` always records the key as
revoked, and lists each target it could not reach as **not removed**, to remove
by hand (see [key lifecycle](key-lifecycle.md)).

## Shared keys

For a `shared` profile key the deploy/verify/revoke steps **fan out to every host**
in the profile - one keypair, many targets.

## rollback

`rollback <key>` is the symmetric reverse: it moves the single `/old/` predecessor
back to the canonical name, re-deploys it, and revokes the rotated-in key -
available until the next rotation supersedes it.

`doctor` asserts the ≤1-old invariant (counted per key name).

## Keys kept in 1Password

For a profile with `key_store: 1password` the flow is the same, with the vault
standing in for the private key files:

1. **Stage** a new SSH Key item titled `<key>.staged` in the profile's vault
   (a leftover from a crashed rotation is archived first); its public half goes
   to `.staging/`.
2. **Deploy** and **verify** as above. A verify login uses the staged `.pub`
   through the 1Password agent, so nothing private is read.
3. **Commit:** any earlier `<key>.old` item goes to the vault's Archive (one
   predecessor, as on disk), the live item is retitled `<key>.old`, the staged
   item takes `<key>`, and the public keys move as files do. A failed step puts
   back what had moved.

`rollback` retitles `<key>.old` back to `<key>` and archives the rotated-in
item. authama archives items and never deletes them, so every one is still
recoverable from 1Password's Archive.

Rotating a key whose profile has just been switched to `1password` moves it
into the vault: the outgoing key file pair is archived to `old/`, and the new
key exists only as an item. A rollback across that move is refused, because its
predecessor is a key file; set `key_store` back to `file` to roll back.

## A GitHub machine user

A key on a GitHub account is deployed, verified and revoked through that
account's API, with the token named by the host's `token_env`. When both the
API and an ssh login are available, verify also checks that GitHub greets the
key as the account the token belongs to, so a token for the wrong account
fails the rotation instead of committing a key the alias cannot push with. See
[Use a GitHub machine user](../recipes/github-machine-user.md).

---

[← Docs index](../../README.md#documentation)
