# Read API

`authama serve` answers programs at `/api/v1/` with JSON when `api.enabled` is on: profiles, keys,
hosts, which tokens are set, the history, the event feed and doctor. It reads only, and it is off by
default.

## Turn it on

```sh
authama settings set api.enabled true
authama api token create ci          # prints the token once
authama serve
```

`serve` prints the API's address when it is on. Call it with the token as a bearer token:

```sh
curl -H "Authorization: Bearer $AUTHAMA_TOKEN" http://127.0.0.1:7822/api/v1/keys
```

## API tokens

| Command | What it does |
|---|---|
| `authama api token create <name> [--expires <days>]` | makes a token and prints it once; 90 days by default, `--expires 0` for never |
| `authama api token list [--json]` | the tokens' names, scopes, dates and last use; never a token |
| `authama api token revoke <name>` | the token stops working at once |

A token is `ama_` and 32 random bytes. authama keeps only its SHA-256, in `api-tokens.json` in the
state home (mode 0600), so the file holds nothing a caller could use. Every token has the `read` scope,
the only one there is. Making and revoking a token are recorded in the [history](history.md).

## Routes

Every route is `GET` (or `HEAD`) and answers JSON.

| Route | What it returns |
|---|---|
| `/api/v1` | the app, its version and these routes |
| `/api/v1/profiles` | every profile, as the [export](export.md) dataset has it |
| `/api/v1/keys` | every key: type, fingerprint, created, expiry, store, whether it is on disk |
| `/api/v1/keys/{profile}/{key}` | one key, and the hosts that use it |
| `/api/v1/hosts` | every host with its SSH settings and status |
| `/api/v1/tokens` | which auth tokens are set and where; never a secret |
| `/api/v1/history` | operations, newest first; filters `since`, `until`, `command`, `source`, `host`, `profile`, `key`, `limit` (up to 500) |
| `/api/v1/history/{op}` | one operation by its id or its first 8 characters |
| `/api/v1/events` | a page of the [event feed](#the-event-feed) after `after`; `limit` (up to 500) and `type` (comma-separated prefixes) |
| `/api/v1/events/stream` | the feed as server-sent events, from `after`, `Last-Event-ID` or now |
| `/api/v1/doctor` | doctor's report: `ok`, the text, and the details |

An error is `{"error": {"code": "...", "message": "..."}}` with a matching status.

## The event feed

One ordered stream of what happened, read from the history database and the [file events](events.md)
log, so it holds changes made from the command line and the TUI as well as the web:

| Type | When | Data |
|---|---|---|
| `operation.finished` | a change finished, from any source | `op_id`, `command`, `args`, `source`, `result`, `error`, `subjects` |
| `file.added`, `file.changed`, `file.removed`, `file.mode` | a watched file changed | `kind`, `area`, `path`, `changed_at` |

Each event has an `id`, a `type`, a `timestamp` and its `data`. `/events` returns `next`: pass it as
`after` to read on, and each event is read once. A file event is ordered by when authama noticed it, so
a change made while nothing was watching still comes after your cursor. The stream's SSE `id` is the
same cursor, so a client that reconnects with `Last-Event-ID` resumes where it stopped.

## Security

- **Off by default**, and `/api/v1/` answers 404 until `api.enabled` is on.
- **Reads only.** Any method but `GET` and `HEAD` is refused with 405.
- **API tokens only.** The browser's session cookie is never read on `/api/v1/`, so a page in a browser
  cannot use your signed-in session to call it, and the web UI's pages never accept an API token. The
  sign-in link's token is not an API token either.
- **Programs, not pages.** A request carrying an `Origin` header (which browsers send) is refused, even
  with a good token.
- **The same host check** as the web UI: only `127.0.0.1`, `localhost`, `::1` and any `--allow-host`,
  which stops DNS rebinding.
- **No secret leaves.** The tokens route says only whether a token is set; key routes carry
  fingerprints and paths, never key material.
- Remote callers go through `--allow-remote` and a TLS proxy, as the web UI does; the API serves plain
  HTTP on loopback.

---

[← Docs index](../../README.md#documentation)
