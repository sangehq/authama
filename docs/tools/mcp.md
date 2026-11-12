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

## Protocol

The server is built on the official Go SDK,
[`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk), which
handles the framing, JSON-RPC 2.0, initialisation and version negotiation. It answers `initialize`,
`ping`, `tools/list` and `tools/call`; any other request is refused with `-32601`.

The protocol revisions it speaks are `2025-11-25`, `2025-06-18` and `2025-03-26`: a client asking for
one of them gets it, and any other gets the newest.

| Request | Answer |
|---|---|
| a tool that fails, such as `get_key` with no `ref` | a result marked `isError`, with the reason as text, so the model can read it |
| `tools/call` with no tool name | error `-32602`, `tools/call needs a tool name` |
| `tools/call` naming no tool | error `-32602`, `no tool <name>` |
| `tools/call` whose `arguments` is not an object | error `-32602`, `arguments must be an object` |
| a method it does not serve | error `-32601`, `method not found` |
| a line that is not JSON-RPC 2.0 | the session ends; start the server again |

When stdin closes, the server answers every request it has already read before it exits, so a script
can write its requests and close stdin:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"sh","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_profiles"}}' \
  | authama mcp
```

---

[← Docs index](../../README.md#documentation)
