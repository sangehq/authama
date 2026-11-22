# deploy - install a public key on its target(s)

```sh
authama ssh deploy <key> [target]
```

- `<key>` is a key name (e.g. `work_hpc-ed25519`). The key must already be minted
  (`authama ssh reconcile` or `authama ssh keygen`).
- `[target]` is an optional host alias. Omitted → deploy to **every** host that
  uses the key (one for `per_service`, all for a `shared` profile key).

Deploy installs the key's **public** half and records the deployment in
`inventory.json` (keyed by fingerprint) + the audit log. It's idempotent - one
entry per target.

## Providers (Strategy, pluggable)

A host names a provider (`"provider": "github"`); none → generic SSH. Resolution
order:

1. **Named adapter** (richer: API/CLI add + list + revoke + auto-record)
2. **Generic SSH** → `ssh-copy-id` to any reachable server
3. **Web-panel / manual** → open the keys page, paste, record a manual deployment

| Provider | category | deploy | needs |
|---|---|---|---|
| `generic-ssh` (default) | server | `ssh-copy-id -i <pub> [-p port] user@host` | reachable host |
| `github` | vcs | `gh ssh-key add` | `gh` + a token (`token_env`, default `GH_TOKEN`) |
| `gitlab` | vcs | `glab ssh-key add` | `glab` + `GLAB_TOKEN` |
| `ploi` / unknown | panel/generic | manual (prints the panel URL) | - |

When a VCS provider has no token/CLI it **degrades to the manual path** - the
deployment is recorded as `needs-redeploy` (unverified) and you finish it in the
web UI. Two accounts on one provider stay separate via per-host `token_env`
(`GH_TOKEN` vs `GH_TOKEN_ACME`).

## Key status: not-deployed, needs-redeploy, deployed

A deployment is `verified` when the tool confirmed it (ssh-copy-id / API success);
manual steps are `unverified` until `audit` compares the key against the
server's `authorized_keys` / the account's key list.

`list`, `audit`, `export` and the web UI show one status per key:

| Status | Meaning |
|---|---|
| `not-deployed` | no deployment is recorded: a freshly minted or regenerated key |
| `needs-redeploy` | deployments are recorded, but none is verified |
| `deployed` | at least one deployment is verified |
| `no-key` | (hosts only) the host's key is not in the inventory |

## What a destination may hold

`deploy`, `verify` and `remove` run `ssh` and `ssh-copy-id` with the destination after `--`, so a
host can never be read as an option. Through the route (the default, below), the destination is the
host's alias, held to the same rule. On the direct route it is the host's `user@hostname`:

- The user may hold dots, dashes, `@`, `\` and `$` (`first.last`, `DOMAIN\user`, `svc$`), but not
  a leading `-`.
- The hostname may be a name, an IPv4 address, a bare or bracketed IPv6 address, or hold `_`. It may
  not start with `-` or hold `$` or `\`.
- Neither part may hold whitespace, quotes or any of `` `;&|<>(){} ``.

A host that breaks the rule is refused with a message naming it. Fix it with
`authama ssh host edit <profile> <alias> --hostname … --user …`.

## How deploy reaches a server

By default, `deploy`, `rotate`, `rollback` and `key revoke` reach a server as `ssh <alias>` does:

- authama writes the managed config, flattened with every path absolute, to `route.conf` in the
  state home. The file is owner-only and holds what `ssh config export` does, never a key.
- It runs `ssh -F <route.conf> -- <alias>`, so the host's `ProxyJump` (and the jump host's own
  block), `ProxyCommand`, `--proxy cloudflared` or VM route applies. So do its `HostName`, `User`
  and `Port`.
- A host with such a route is not probed on its hostname first, since nothing listens there; ssh
  says whether it can get through.
- Deploy uses plain `ssh` for a routed host, because an `ssh-copy-id` older than OpenSSH 9 has no
  `-F`. It does what `ssh-copy-id` does: creates `~/.ssh` owner-only and appends the key once.

The direct route is the old behaviour: `ssh -- user@hostname`, with `-p` for another port, read
against your own `~/.ssh/config`. Ask for it with:

- `--direct` on `deploy`, `rotate`, `rollback` or `key revoke`, for one run;
- *Connect directly* on the job's review in the web UI;
- `ssh.deploy_route` set to `direct` in `authama.json`, or `AUTHAMA_DEPLOY_ROUTE="direct"`, for
  every run.

A host only a person can reach (a console, a panel) takes its key by hand:
`authama ssh recover <key>` prints a snippet to paste on it.

## Examples

```sh
authama ssh deploy work_hpc-ed25519            # ssh-copy-id to hpc.example.edu:443
authama ssh deploy personal_github-ed25519     # gh ssh-key add (or manual if no token)
authama ssh deploy shareddemo_all-ed25519      # fans out to every host in the profile
authama ssh audit                              # see what's deployed where
```

---

[← Docs index](../../README.md#documentation)
