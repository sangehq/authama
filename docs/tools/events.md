# `events`

`authama events` lists every change seen to a file authama is responsible for, newest first: anything
under `~/.ssh`, the config home, and each tool's credential file, whoever changed it.

## What is tracked

| Area | Files |
|---|---|
| `ssh` | everything under `~/.ssh`: config, profile configs, keys, `known_hosts` |
| `config` | the config home: `manifest.json`, `inventory.json`, `authama.json`, `tokens.json`, `.env` (authama's own state, the history and these events, is left out) |
| `auth` | each tool's credential file: Composer's `auth.json`, `~/.npmrc`, Docker's `config.json`, `~/.git-credentials`, `~/.netrc` |

Each scan keeps, per file, its size, mode, modification time and a SHA-256 of its bytes, in an
owner-only state file, and appends what changed to `log/events.jsonl` in the state home:

| Kind | Means |
|---|---|
| `started` | the first scan: tracking began, nothing compared |
| `added` | a file that was not there before |
| `changed` | its contents changed |
| `removed` | a file that is gone |
| `mode` | only its permissions changed |

**A file's contents are never recorded.** An event names the file, what changed and when; the hash
detects a change and cannot be turned back into a key or a token.

## Tamper evidence

Each line of `events.jsonl` carries a `hash`: SHA-256 over the previous line's hash and its own
fields. The state file records the hash of the last line. `authama events --verify` walks the chain
and exits non-zero, naming the first bad line, when a line was edited, removed or inserted after the
fact, or when the log stops short of the recorded last line. The Events page shows the same result.

This is tamper *evidence*: someone who can rewrite the state home can rewrite both files together,
but not one line of the log without the chain showing it. Lines written before the chain existed sit
at the start of the log and are counted, not checked.

## When it scans

- `authama events` scans first, then lists (`--no-scan` lists what is recorded).
- `authama serve`, and a bare `authama` on a terminal, scan every 30 seconds while they run, and when
  the Events page or the Overview opens.

An event's time is the file's modification time for an added or changed file, and the scan's time for
a removal or a mode change, which leave no time behind. The web UI matches each event to the authama
operation that was running then, and links to it in the [history](history.md); anything else shows as
*outside authama*.

## Usage

```sh
authama events                        # scan, then list everything
authama events --since 24h --area auth
authama events --kind mode            # permission changes only
authama events --json                 # for scripts
authama events --csv > events.csv     # for a spreadsheet
authama events --verify               # check the hash chain
```

| Option | Does |
|---|---|
| `--since DURATION` | only events seen in that window, e.g. `24h` |
| `--area ssh\|config\|auth` | only that area |
| `--kind KIND` | only that kind |
| `--limit N` | at most N, newest first |
| `--json` | machine-readable output |
| `--csv` | one CSV row per event, header first |
| `--verify` | check the log's hash chain instead of listing; exit non-zero if broken |
| `--no-scan` | list without scanning first |

---

[← Docs index](../../README.md#documentation)
