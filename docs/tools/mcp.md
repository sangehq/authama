# MCP server

`authama mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio, so an
AI client can read your profiles, keys, hosts, history and events. Its tools read only, and it is off
by default.

## Turn it on

```sh
authama settings set mcp.enabled true
```

Then register it with your client as a stdio server whose command is `authama mcp`. For Claude Code:

```sh
claude mcp add authama -- authama mcp
```

For a client configured by file, the entry is the command and no arguments:

```json
{ "mcpServers": { "authama": { "command": "authama", "args": ["mcp"] } } }
```

## Tools

Every tool is marked read-only (`readOnlyHint`), and returns JSON as text and as structured content.

| Tool | What it returns |
|---|---|
| `list_profiles` | every profile |
| `list_keys` | every key, or a `profile`'s |
| `get_key` | one key by its `ref` (`profile/name`), and the hosts that use it |
| `list_hosts` | every host, or a `profile`'s |
| `list_tokens` | which auth tokens are set and where; never a value |
| `list_history` | recorded changes, newest first; `limit`, `since`, `command`, `host`, `profile`, `key` |
| `get_operation` | one change by its `op_id` (or its first 8 characters) |
| `list_events` | the [event feed](api.md#the-event-feed); pass the returned `next` as `after` to read on |
| `doctor` | doctor's report |

## Security

- **Off by default.** `authama mcp` refuses to start until `mcp.enabled` is on.
- **stdio only.** It opens no port, so only the client that started it can talk to it; the protocol
  recommends stdio for a local server for this reason.
- **Reads only.** No tool changes the home, mints, deploys, rotates or revokes. A change still goes
  through the command line or the web UI, where you confirm it.
- **No secrets.** The tools read the same records as the [read API](api.md) and the export: no key
  material, and no token's value.
- stdout carries nothing but protocol messages; anything else goes to stderr.

The protocol revisions it speaks are `2025-11-25`, `2025-06-18` and `2025-03-26`: a client asking for
one of them gets it, and any other gets the newest.

---

[← Docs index](../../README.md#documentation)
