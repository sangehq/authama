# checklist - where each key and token is, and what is left to revoke it

```sh
authama checklist [--open] [--json]
authama checklist show <item> [--json]
authama checklist tick <item> <step>
authama checklist untick <item> <step>
authama checklist revoking <token item> [--stop]
```

Every SSH key and every auth token has a checklist. It lists every place the item
is used and every step to rotate or revoke it completely, so nothing is forgotten
on a server, in ssh-agent, in a backup or in a CI secret.

An item is named by the id `authama checklist` prints (`key:SHA256:…`,
`token:npm/registry.npmjs.org`), or a key by its `profile/key`.

## A key's checklist

It follows the key's life.

| The key is | The checklist lists |
|---|---|
| In use | Its key pair on disk (owner-only), or its 1Password vault; the hosts that use it; each target it is installed on (verified, not verified, or pasted by hand); when it is due for rotation; copies outside authama. |
| Revoked (`ssh key revoke`) | Each target it was removed from, and each it could not be; hosts still pointing at it; the private key on this machine; ssh-agent; encrypted bundles that hold it; old snapshots that hold private keys; accounts it was added to by hand; other copies. |
| Retired by a rotation | The same, from what the rotation recorded in `retired.json`: the targets it removed the old key from, the ones it could not, and where the by-hand ones are managed. The private key stays in `old/` for `ssh rollback` until the next rotation. |

## A token's checklist

While in use it lists each tool's credential file that holds the token, Docker's
credential store, the variables `tokens.json` reads the secret from, and the
`tokens.json` entry. `authama checklist revoking <item>` switches it to the
revocation steps: revoke it where it was issued (with a link for npm, GitHub,
GitLab, PyPI, RubyGems, crates.io and Docker Hub), remove it from each file
(`authama token delete`), log Docker out, clear its variables, and remove it from
CI secrets and other machines. `--stop` switches it back; ticks are kept.

## Checked, and ticked by hand

authama works out each step it can see every time the checklist is read: a file on
disk, a removal it recorded, a key ssh-agent holds (`ssh-add -l`), a bundle's
contents. A step it cannot see (a token's issuer, Termius, a CI secret, a
web-panel target) is marked "by hand": tick it with `authama checklist tick` or
the button on the web page. Ticks are kept in `.state/checklists.json` in the state
directory, owner-only.

Every step that is not done names the command that does it, or the page where it
is done.

## In the web UI

- **Security > Checklists** lists every key and token with its progress, filtered by
  steps left, keys or tokens.
- Each key's and token's row menu has **Checklist**, which opens it in the side panel.
- A key's page shows a summary card with a link to its checklist.

## Choosing several keys to rotate, revoke or delete

`ssh rotate`, `ssh key revoke` and `ssh key delete` each take several keys, profiles
or host aliases, `--category <name>` (every profile filed under it) or `--all`:

```sh
authama ssh key revoke work personal/personal_gh-ed25519 --yes
authama ssh key revoke --category clients --yes
authama ssh key delete work --purge --yes   # each key in work no host uses
```

Each key is done in turn; one that fails is named and the rest go on. On the
**Keys** page (and in the Actions menu), **Rotate keys**, **Revoke keys** and
**Delete keys** open the job's review page, where its keys are chosen: a picker
with every key, whole categories, whole profiles and single keys, beside the job's
options and its Start button. A key the action cannot act on (rotate: no host uses
it; revoke: never minted; delete: a host uses it) is shown, unticked, with the
reason. Changing the keys or an option reviews the job again, and the page has an
address of its own (`/jobs/review?action=rotate-keys&sel=work`), so it can be
reloaded or bookmarked; an older `/keys#rotate-keys` link opens it too.

---

[← Docs index](../../README.md#documentation)
