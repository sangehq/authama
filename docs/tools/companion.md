# `companion` — the handshake for a companion tool

`authama companion info` tells a tool that works beside authama, such as sange, which build it is
talking to, which versions of the companion contract it answers, what it can do, and where its homes are.
It reads only.

## `authama companion info`

```text
$ authama companion info
authama 0.1.4
contract:      1
capabilities:  git.status.json, git.use.json, git.resolve.json, api.read, events.cloudevents, webhooks.standard, mcp.stdio
config home:   ~/.config/authama
state home:    ~/.local/state/authama
```

`--json` prints the handshake sange's companion contract (v1) defines, under its names:

```json
{
  "name": "authama",
  "version": "0.1.4",
  "contractVersions": ["1"],
  "capabilities": ["git.status.json", "git.use.json", "git.resolve.json", "api.read",
                   "events.cloudevents", "webhooks.standard", "mcp.stdio"],
  "configHome": "/home/alice/.config/authama",
  "stateHome": "/home/alice/.local/state/authama"
}
```

| Capability | What it is |
|---|---|
| `git.status.json` | [`ssh git status --json`](git.md#authama-ssh-git-status-path) |
| `git.use.json` | [`ssh git use --json`](git.md#authama-ssh-git-use-mode-path), with `--dry-run` |
| `git.resolve.json` | [`ssh git resolve OWNER/REPO --json`](git.md#authama-ssh-git-resolve-ownerrepo) |
| `api.read` | the [read API](api.md) on `serve`, when `api.enabled` is on |
| `events.cloudevents` | the [event feed](api.md#the-event-feed), as CloudEvents 1.0 |
| `webhooks.standard` | [webhooks](webhooks.md) signed as Standard Webhooks, when `webhooks.enabled` is on |
| `mcp.stdio` | the [MCP server](mcp.md), when `mcp.enabled` is on |

A capability is listed when this build has it, whether or not a setting turns it on: the API, webhooks
and MCP are off by default, and say so when called. The homes are the ones this run resolves, so
`--config-dir` and `AUTHAMA_CONFIG_DIR` move them as they move everything else.

Fields are only ever added. A contract version is added when authama implements it, and is dropped
only by a release whose CHANGELOG says so.

---

[← Docs index](../../README.md#documentation)
