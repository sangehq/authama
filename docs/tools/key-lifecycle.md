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

## History and events

Every step is an operation in [`history`](history.md), with a `key.lifetime` or `key.revoke` event,
and its file changes show in [`events`](events.md).

---

[← Docs index](../../README.md#documentation)
