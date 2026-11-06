# `tui` — removed

The terminal menu was removed. The web UI is the interactive surface, and every menu entry was
already a CLI command.

## What to run instead

`authama serve`, or a bare `authama` on a terminal, opens the [web UI](serve.md). The `tui` command
still answers, but it only prints that pointer and exits with an error, so an old script or the
`atm-ui` alias from an earlier install says where to go. A fresh `token shell-aliases --install`
points `atm-ui` at `authama serve`.

## Where each menu entry went

| Menu entry | Command | Web UI |
|---|---|---|
| Browse profiles & hosts | `ssh list` | Profiles, Hosts |
| Show the rendered config | `ssh config show` | SSH config |
| Expiry status | `ssh expiry` | Keys, Overview (keys due for rotation) |
| Audit deployments & expiry | `ssh audit` | Doctor, Audit |
| Reconcile — apply the manifest | `ssh reconcile` | Overview quick actions (previewed) |
| Pin host keys into known_hosts | `ssh knownhosts init --all` | Hosts → Pin host keys; a host's page pins one host |
| Deploy a key to its target | `ssh deploy` | Keys (a job) |
| Rotate a key | `ssh rotate` | Keys (a job) |
| Snapshots — list and restore | `ssh snapshots` | Snapshots |
| List installed tokens | `token list` | Auth tokens |
| Token setup status | `token status` | Auth tokens |
| Audit the token files | `token audit` | Auth tokens |
| Copy a token to the clipboard | `token copy` | CLI only |
| Add or replace a token | `token set` | Auth tokens (`n t`) |
| Apply the token config | `token apply` | Auth tokens (previewed) |
| Delete a token | `token delete` | Auth tokens (previewed) |

The banner the menu opened with is the Overview's first table. Both read `overview.Rows`, so
the problems, and the command that fixes each one, are the same.

## Why it went

The menu was a plain numbered prompt over stdin, with no colour, no arrow keys and no progress
display. That was deliberate, so the binary keeps one third-party dependency. The web UI does the
same jobs with previews, explainers on every control, a command palette and keyboard shortcuts.
Keeping both meant every change had to be built and tested twice. Operations the menu recorded
keep the `tui` source in [`history`](history.md).

---

[← Docs index](../../README.md#documentation)
