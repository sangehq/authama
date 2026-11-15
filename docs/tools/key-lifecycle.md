# Key lifecycle

Six things happen to an SSH key after it is made: it lives for a set time, it is deployed, it is replaced, it can be rolled back, it can be regenerated, and it can be revoked.

## At a glance

| Step | Command | What it does |
|---|---|---|
| Set its lifetime | `authama ssh key lifetime <profile/key> <days\|preset>` | how long it lives before rotation is due; the expiry is counted again |
| Deploy | `authama ssh deploy <profile/key>` | installs the public half on every host that uses it, and records each |
| Rotate (recycle) | `authama ssh rotate <profile/key>` | mints a successor, deploys and verifies it everywhere, then retires the old key to `old/` |
| Roll back | `authama ssh rollback <profile/key>` | puts the `old/` predecessor back, and keeps the rotated-in key so it can be undone |
| Regenerate | `authama ssh keygen <profile/key> --force` | a new pair now, without the staged deploy; the old pair moves to `old/` |
| Revoke | `authama ssh key revoke <profile/key>` | removes the key from every target and never deploys it again |

The web UI does each of these from a key's page and its row in the Keys list, except regenerating,
which writes an encrypted backup of `~/.ssh` first and stays in the terminal (the page offers to
copy the command). Deploy, rotate, roll back and revoke reach real servers, so they run as jobs and
are refused in dev mode.

A broken or missing `.pub` is rebuilt from its private key (or its 1Password item) with
`authama ssh key regen-pub`, which never changes the private key. The web UI does it from a key's
page, a row's menu, or *Rebuild public keys* on the Keys page for a profile or every key, previewed
like any change.

## Lifetime

A key's lifetime is its `rotate_after_days`. Set on the key, it wins; unset, the key inherits
`defaults.rotate_after_days` (365). The expiry date is `created + lifetime`, and
[`expiry`](expiry.md) warns from `warn_before_days` ahead (30 days by default).

```sh
authama ssh key lifetime work/work_gh-ed25519 quarterly   # 90 days
authama ssh key lifetime work/work_gh-ed25519 120         # any number from 1 to 1825
authama ssh key lifetime work/work_gh-ed25519 --clear     # inherit the default again
authama ssh key lifetime --default annual                 # every key without its own
```

| Preset | Days | When it is asked for |
|---|---|---|
| `annual` | 365 | the usual floor for SSH user keys, and authama's default |
| `semiannual` | 180 | many internal security policies |
| `quarterly` | 90 | stricter regimes, and keys on shared or exposed machines |
| `monthly` | 30 | high-turnover access, short engagements |

Five years (1825 days) is the most it accepts: no guidance gives a key used for authentication
longer. A raw SSH keypair does not expire by itself; the lifetime is a policy reminder, and the
rotation is what ends the key.

## Revoke

Revoking withdraws a key you no longer trust - a lost laptop, a leaked file, someone leaving -
without deleting it:

```sh
authama ssh key revoke work/work_gh-ed25519 --reason "laptop lost"
```

1. Every target the key is deployed to is asked, through its host's provider, to remove it: GitHub
   and GitLab through `gh` and `glab`, a cloud provider through its API, a server by editing its
   `authorized_keys`. These are the same calls rotation makes for the key it replaces.
2. A target that cannot be reached, whose provider cannot remove keys (a web panel such as
   Codeberg's), or whose host is no longer declared, is listed as **not removed**, and the command
   exits non-zero so a script notices. Remove the key there by hand.
3. The deployments it was removed from leave its inventory record; the ones it was not stay, so the
   record still says where it is.
4. Its fingerprint is written to `revoked.json` beside the inventory, with the date and reason. From
   then on `deploy` and `rollback` refuse that key.

The key's declaration, hosts and files stay, so nothing that names it breaks. Mint its successor
next: `authama ssh rotate <profile/key>` stages it, or `authama ssh keygen <profile/key> --force`
replaces it at once. The successor has a fingerprint of its own and is not affected by the
revocation.

`key delete`, `host delete` and `profile delete` take `--revoke` too, which revokes the key from the
targets of what is being deleted first, and reports the same way. Before this release that flag was
accepted and did nothing.

## When each event happened

authama records the time, to the second, at which each thing happened to a key, in
`key-events.json` beside the inventory: `created` (minted, by keygen, reconcile or a rotation),
`added` (entered the inventory, with its origin: `generated`, `adopted` or `imported`), `deployed`
(the latest deploy that reached a target), `rotated` (on the successor, naming the key it
replaced), `restored` (brought back by a rollback), `retired` (replaced by a rotation, or rolled
back), `revoked` and `acknowledged` (`authama ssh key ack`, below). The time is
written by the same save as the inventory record, and only times and origins are kept: nothing
about the key's material.

The file is new; nothing older than it needs migrating. A key recorded before it existed falls
back to the days the inventory already holds (`created`, each deployment's date) and to
`revoked.json` and `retired.json`, so its ages are shown by the day. Recording is always on; only
the label and pills built from it are configurable.

## Recency: new keys and ages

`authama ssh key list` shows how long ago each key was created, added or rotated in (`AGE`) and one
word for where it stands (`STATUS`):

| Status | When |
|---|---|
| `revoked`, `retired` | the key was revoked, or replaced by a rotation (and not rolled back) |
| `overdue`, `due soon` | its [expiry](expiry.md) says so; expiry always wins over age |
| `new` | one of `keys.new_events` happened within `keys.new_window_days` |
| `ageing` | it is past half its lifetime |
| `current` | otherwise |

```sh
authama ssh key list                 # AGE and STATUS columns
authama ssh key list --new           # only the keys labelled new
authama ssh key list --json          # each key's recency: events, ages, is_new, new_because, pill
authama ssh key ack work/work_gh-ed25519   # seen it: clear the label
authama ssh key ack --all
```

A key is **new** for 7 days after it is created, added, rotated in or deployed. Both are settings:
`keys.new_window_days` (`0` turns the label off) and `keys.new_events`, which may also name
`restored` (see [configuration](../configuration.md)). A deploy re-arms the label, and
`--json`'s `new_because` names the event that set it. `authama ssh key ack` clears it at once and
records an `acknowledged` time; the next of those events makes the key new again.

Each event also carries its own age against the key's constraints: `fresh` inside the new window,
`old` past the key's lifetime, `ageing` past half of it, `current` otherwise. A revocation, a
retirement and an acknowledgement end or note something, so they carry only their time. The table
and `--json` never carry colour or terminal escapes.

In the web UI the Keys list has a **Status** column with the pill and the key's age, and a key's
page shows its status and a **Lifecycle** card: every event with how long ago it was (a `<time>`
carrying the exact time) and its constraint. A new key's card offers **Mark as seen**, which is `key ack` as a
previewed change. Each pill says its word as well as taking the badge colour (blue for new and
fresh, green for current, amber for ageing and due soon, red for old and overdue, grey for revoked
and retired), so nothing depends on colour.

## History and events

Every step is an operation in [`history`](history.md), with a `key.lifetime` or `key.revoke` event,
and its file changes show in [`events`](events.md).

---

[← Docs index](../../README.md#documentation)
