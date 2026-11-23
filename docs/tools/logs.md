# `logs`

`authama logs` shows authama's own log, `authama.log`: one record for each command, web action, job,
watched-file change and sign-in. The web UI shows the same under **System › Logs**.

```sh
authama logs                               # the newest 200 records
authama logs --follow                      # and keep printing new ones until Ctrl-C
authama logs --level warn --since 24h      # warnings and errors from the last day
authama logs --grep signin --json          # sign-ins, as JSON lines
authama logs export --out authama-log.jsonl
authama logs path                          # where the file is
```

## What is recorded

| Area | Records |
|---|---|
| `cli` | every command: its words, the names of the flags given, its arguments (left out for the token, owner, API and webhook commands), how long it took and its error |
| `web` | the server starting; every change posted (info) and every page served (debug) |
| `auth` | `signin.link`, `signin.browser`, `signin.refused`, `browser.reused`, `browser.forgotten`, `signout` |
| `job` | each job's start and finish, with its result |
| `watch` | each file change the server's watcher sees under `~/.ssh`, the config home and the token files |

Times are written in UTC (RFC 3339) and shown in the [display time zone](#times).

## What is never recorded

Nothing secret is written. Before a record reaches the file:

- an attribute named like a secret (`token`, `secret`, `passphrase`, `password`, `pin`, `cookie`,
  `authorization`, `control`, `credential`, `api_key`, `signature`, `private`) is replaced with
  `[redacted]`;
- anything that looks like a secret, wherever it appears, is replaced: a private key block, a
  `?token=` / `challenge=` / `caller=` / `proof=` value, a GitHub, GitLab or Slack token, an age
  secret key and a `Bearer` value;
- a flag named like a secret has its value blanked (`--passphrase [redacted]`), and a failed token,
  owner, API or webhook command has its arguments blanked in the error too;
- the web log has only a request's path, never its query string.

The file is owner-only (0600).

## Where, how much, how long

`authama.log` is in the state home's `log/` directory, beside `audit.log` and `history.db`
(`authama logs path`). It rotates to `authama-<time>.log` when it reaches `log.max_size_mb` (5 MB),
and rotated files are deleted after `log.retention_days` (14 days), checked whenever authama starts.

```sh
authama logs level debug        # log.level: debug, info (default), warn or error
authama logs retention 30       # log.retention_days
```

Both are settings (see [configuration](../configuration.md)), so the Settings page edits them too.
`debug` adds every page the web UI serves.

## Filters

| Flag | Meaning |
|---|---|
| `--level` | the least severe level shown: `debug`, `info`, `warn`, `error` |
| `--since` | from then on: `36h`, `7d`, a date (`2026-11-20`) or RFC 3339 |
| `--grep` | records containing this text, in any case |
| `--limit` | the newest N (200 by default; 0 for all) |
| `--json` | each record as written |
| `--follow`, `-f` | keep printing new records until Ctrl-C |

`logs export` takes the same filters, and `--out` names a new file (never overwritten); without it
the records go to stdout.

The **Logs** page has the same filters and a search box, a **Follow** switch that adds new records
as they are written, and **Download** (the filtered records as JSON lines).

## Times

Every time authama shows, in the terminal and the web UI, is in one zone: `display.timezone`, an
IANA name such as `Europe/Berlin` or `UTC`, and this system's zone when it is not set. Stored times
(the history, events and this log) stay UTC.

```sh
authama settings timezone show           # the zone in use, and the time there now
authama settings timezone set Asia/Tokyo
authama settings timezone set local      # back to this system's zone
authama settings timezone list europe    # search the zones
authama --tz UTC history                 # one command in another zone
```

`$AUTHAMA_TIMEZONE` sets it from the environment. The
Settings page's **Display › Timezone** field searches the zones and shows the time in the one you
type. A running server uses a new zone, or goes back to this system's when the setting is taken
out, from its next page; one started with `--tz` or `$AUTHAMA_TIMEZONE` keeps that zone.
