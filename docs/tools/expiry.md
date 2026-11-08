# expiry & notifications - "always warn before a key is due"

A raw SSH keypair **does not self-expire** (only OpenSSH *certificates* carry a
hard validity window). So this is a **policy reminder** authama computes and
surfaces - nothing breaks on the date; it's the cue to rotate.

## How it's computed

From the inventory, per key: `expires_on = created + rotate_after_days`,
`days_remaining`, and a `state`:

- `ok` - outside the warn window
- `due_soon` - within the largest `warn_before_days` threshold (default 30)
- `overdue` - past `expires_on`

The lifetime is the key's own `rotate_after_days` or the default's; set either with
`authama ssh key lifetime` (see [key lifecycle](key-lifecycle.md#lifetime)), which counts the
expiry again. `rotate` resets `created`/`expires_on`, clearing the warning. Archived `/old/`
keys are excluded. To clear several at once, rotate them as a batch: a category
(`authama ssh rotate --category clients`) or everything (`authama ssh rotate --all`); see
[Rotating a batch](rotate.md#rotating-a-batch).

## Surfaces (layered - warned whether or not you open the tool)

1. **Inline banner** - every command runs a cheap, **debounced** check first
   (cached for `expiry_check.debounce_hours`, default 24) and prints a `⚠` line to
   stderr for any due/overdue key. Never breaks scripting (stderr, best-effort).
2. **`authama ssh expiry`** - the full table (key, profile, `expires_on`, days, state).
   Also folded into `authama ssh audit`.
3. **Scheduled notifier** - `authama ssh notify install` registers a launchd (macOS) /
   cron (Linux) job running `authama ssh audit --notify`. It emits a **desktop
   notification** and appends to the audit log. Cadence is **weekly** normally,
   **daily** once any key is inside its warn window, with a last-notified debounce.
   `authama ssh notify test` fires a test notification.

All of this is config-driven from `defaults.warn_before_days` and
`defaults.expiry_check` in the manifest (`AUTHAMA_SNAPSHOT_RETAIN` and the cadence
caches live in the config-dir, gitignored).

```sh
authama ssh expiry                 # full table
authama ssh audit --notify         # report + cadence-gated desktop alert (what the job runs)
authama ssh notify install         # set up the scheduled notifier
authama ssh notify test            # confirm desktop notifications work
```

## How a notification is shown

Notifications go through [go-notifykit](https://github.com/simtabi/go-notifykit),
so on macOS a banner says **authama** with authama's logo mark rather than
"Script Editor". The first notification builds a small helper app in
`~/.local/state/authama/notifykit/`, and macOS asks once whether to allow it; it
is listed under System Settings › Notifications as `authama`. Linux uses
`notify-send` with the same icon, Windows a PowerShell toast.

| Switch | Effect |
|---|---|
| `AUTHAMA_NOTIFICATIONS=off` | no desktop notifications from authama (as before) |
| `AUTHAMA_NOTIFY_DESKTOP=0` / `NOTIFYKIT_DESKTOP=0` | the same, through go-notifykit's own switch (the second covers every program using it) |
| `AUTHAMA_NOTIFY_BACKEND=osascript` | force a backend: `helper`, `terminal-notifier`, `osascript`, `notify-send`, `powershell` |

Nothing is shown under `go test` or when `CI` is set.

```sh
authama ssh notify doctor                                # is it on, which backend, which socket
authama ssh notify send -title Backup -body "42 files"   # show one now
authama ssh notify ping -title "Deploy finished"         # show one through the running web UI
```

While `authama serve` runs, other programs and scripts can ping it: it listens on
a socket only you can open, in its state directory, and each message needs the
token it writes there on every start.

---

[← Docs index](../../README.md#documentation)
