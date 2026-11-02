# `vm` — local VMs as SSH hosts

A VM's address comes from its hypervisor and changes, so a VM host does not carry one. Its block renders
`ProxyCommand "<authama>" vm proxy <alias>`, and each connection asks the hypervisor where the VM is now.

| Hypervisor | CLI | Address from | Key install | Keys owned by |
|---|---|---|---|---|
| Parallels Desktop | `prlctl` | `list --all --full --json` (`ip_configured`) | `prlctl exec` | authama |
| VMPal | `vmpal` | `list --json` (`localAddress`) | `vmpal exec` | authama |
| VMware Fusion / Workstation | `vmrun` | `getGuestIPAddress -wait` | over SSH once (guest commands need guest credentials) | authama |
| UTM | `utmctl` | `ip-address` (first IPv4) | `utmctl exec` | authama |
| Lima | `limactl` | `list --json` (forwarded `sshLocalPort` on loopback) | `limactl shell` | authama |
| OrbStack | `orb` | its own ssh config (`ssh <machine>@orb`) | not needed | OrbStack |
| Podman machine | `podman` | `machine list --format json` (forwarded port) | not needed | Podman |

OrbStack and Podman run their own SSH access and keys: `vm list` shows their machines, and they are
refused as hosts so authama never takes their keys over. VMware's VM name is the path of its `.vmx`.

## Choosing your hypervisors

authama queries only the hypervisors you say you use. Name them in `authama.json`:

```json
{ "vm": { "hypervisors": "parallels,vmpal" } }
```

or in `.env` (double-quote a list: commas and spaces are special characters there):

```sh
AUTHAMA_VM_HYPERVISORS="parallels,vmpal"
```

Unset, every built-in driver and every custom one is enabled. `vm list`, `vm sync`, `doctor` and the web
UI's VMs page skip the others; a VM host on a hypervisor that is not enabled is reported, never changed.
A name that matches no driver is a warning in `doctor` and `vm list`.

### Adding a hypervisor authama has no driver for

The `hypervisors` section of `authama.json` declares a **command driver**: argv arrays (no shell), in which
`{vm}` is replaced with the VM's name. Enable it by naming it in `vm.hypervisors`.

```json
{
  "vm": { "hypervisors": "parallels,vmpal,myvirt" },
  "hypervisors": {
    "myvirt": {
      "description": "my own VM tool",
      "list":    ["myvirt", "list", "--json"],
      "address": ["myvirt", "ip", "{vm}"],
      "start":   ["myvirt", "start", "{vm}"],
      "exec":    ["myvirt", "exec", "{vm}", "--"]
    }
  }
}
```

| Field | Required | Prints / does |
|---|---|---|
| `list` | yes | the VMs: a JSON array of `{"name": "...", "state": "..."}` (an optional boolean `running` wins over `state`), or one `name<TAB>state` per line. `running`, `started` and `up` count as running |
| `address` | yes | where the VM's SSH server answers: `host` or `host:port` |
| `start` | no | starts the VM; without it, `ssh` to a stopped VM fails instead of starting it |
| `exec` | no | runs a command in the guest (the guest argv is appended), so `vm sync` can re-install a missing key; without it the key goes in over SSH |

A custom name may not shadow a built-in one. `authama settings validate` checks the section.

### Declaring devboxes

The `devboxes` section names the VM hosts authama should keep, by alias. `vm sync` adds a declared devbox
that is not a host yet and, when its profile has no key named `key` but `~/.ssh/<key>` exists, copies
that pair into the profile (the originals stay). A declared devbox is never pruned.

```json
{
  "devboxes": {
    "devbox": { "profile": "devbox", "hypervisor": "parallels", "vm": "Ubuntu 24.04",
                "user": "dev", "key": "devbox-key", "docker_context": true }
  }
}
```

`profile`, `hypervisor`, `vm` and `user` are required; `docker_context` defaults to `true`.

A devbox **without `vm`** is *planned*: it keeps its key for a VM you have not made yet. Only `profile`
and `key` are required (a `hypervisor` and `user` are checked if you give them). `doctor` reports the key
under "reserved keys" instead of calling it loose, and `vm sync` adds no host for it - it notes
"devbox docker has no vm yet". Give it its `vm` (and `hypervisor`, `user`) later and the next sync adds
the host and adopts the key.

```json
{
  "devboxes": {
    "docker": { "profile": "devbox", "key": "docker-tunnel",
                "description": "Docker devbox VM, kept for later" }
  }
}
```

The same can be recorded without `authama.json`: `authama ssh key reserve docker-tunnel --for devbox`
(see [Keeping a key for later](#keeping-a-key-for-later)).

## `authama ssh vm list`

```text
parallels  not installed
vmpal      Ubuntu 24.04                   running  host devbox/devbox
vmware     not installed
lima       default                        Running
```

## `authama ssh vm add <profile> <alias> --hypervisor H --vm NAME -u USER`

Adds the host (HostName is the alias, since ProxyCommand ignores it) and re-renders the config. Mint
its key with `authama ssh reconcile`, then install the public key in the guest once (with the hypervisor's
exec, or over SSH with a password).

```sh
authama ssh vm add devbox devbox --hypervisor parallels --vm "Ubuntu 24.04" -u dev
authama ssh reconcile
ssh devbox
```

In the manifest:

```jsonc
{"alias": "devbox", "hostname": "devbox", "user": "dev", "key_name": "devbox_docker-ed25519",
 "vm": {"hypervisor": "parallels", "name": "Ubuntu 24.04"}}
```

A `port` other than 22 on the host wins over the hypervisor's (Lima and Podman forward their own).

## What `vm proxy` does (ssh runs it)

1. Finds the host's VM and hypervisor.
2. Asks for the current address. A stopped VM is started, and authama waits up to three minutes for an
   address, saying so on stderr.
3. Connects and relays stdin/stdout, so ssh talks to the guest as if directly.

The rendered path is always `authama`, even when `sshmgr` (the deprecated alias) renders the
config, and prefers the `authama` on your `PATH` (a package manager's stable link) over the
versioned file behind it. If an upgrade still moves it, `doctor` reports `vm proxy: … does not exist`
and `authama ssh config render` rewrites it.

## `authama ssh vm docker-context <alias>`

Creates (or updates) a Docker context `ssh://<alias>`, so `docker --context devbox ps` runs against the
VM's daemon through the same ProxyCommand. OrbStack and Podman provide their own contexts.

## `authama ssh vm sync`

Keeps every VM host matching its VM. For each host it asks the hypervisor what became of the VM:

| VM is | sync does |
|---|---|
| running | pins the guest's host key if nothing is pinned; re-installs the host's public key in the guest when the guest refuses it (with the hypervisor's exec); creates or refreshes Docker context `ssh://<alias>` |
| gone | backs up the manifest, removes the host entry (never its key files), and removes the Docker context authama made for it (exported first) |
| suspended, paused or stopped | reported and left alone: sync never starts, stops or deletes a VM |

It also adds declared devboxes and removes Docker contexts authama made (they carry the description
`managed by authama`) whose host is gone; a context you made yourself, or the current one, is never
removed. A VM counts as gone only when its hypervisor listed its VMs successfully and the list is not
empty, so a hypervisor that is not running or not installed never causes a removal. Running sync twice
changes nothing the second time.

```sh
authama ssh vm sync --dry-run   # the plan, nothing changed
authama ssh vm sync             # the plan, then a confirmation
authama ssh vm sync --yes       # no confirmation (implied with no terminal)
```

| Flag | Meaning |
|---|---|
| `-n`, `--dry-run` | show the plan and change nothing |
| `-y`, `--yes` | apply without confirming |
| `-q`, `--quiet` | print only what changed and errors |
| `--no-probe` | ask the hypervisors only; do not connect to running guests |
| `--repin` | replace a pin when the guest's host key changed (a rebuilt VM); otherwise reported (`vm.repin_changed`) |
| `--install-schedule` / `--remove-schedule` | run `sync --quiet --yes` daily at 09:30 with launchd (macOS), systemd or cron (Linux), or Task Scheduler (Windows) |

Settings: `vm.prune_gone` (`AUTHAMA_VM_PRUNE_GONE`, default `true`; off, a gone VM is only reported) and
`vm.repin_changed` (`AUTHAMA_VM_REPIN`, default `false`).

Backups: before any change, sync snapshots `~/.ssh` and copies the manifest to
`<state home>/backups/vmsync/`; an exported Docker context goes there too. Private keys are never read,
moved or deleted. `authama doctor --fix` runs the same sync.

## `authama ssh vm adopt-key [<key> [<profile> [<alias>]]]`

Turns a key you already use for a VM, sitting loose in `~/.ssh`, into a VM host's managed key. It copies
the pair into the profile (verified identical; the originals stay unless `--move`), adds the VM host,
records the change and re-renders `~/.ssh/config`. At a terminal, anything missing is asked for with a
picker: the key from the loose keys in `~/.ssh`, the hypervisor from the enabled ones, the VM from its
listing, and the guest user.

```sh
authama ssh vm adopt-key                       # fully guided
authama ssh vm adopt-key devbox-key devbox devbox \
  --hypervisor parallels --vm "Ubuntu 24.04" --user dev --docker-context --dry-run
authama ssh vm sync                            # then pin, install the key in the guest, Docker context
```

OrbStack and Podman are refused (they own their keys); so is an alias already in use or an invalid user.
`doctor` points at this command for a loose key whose name or comment suggests a devbox (words such as
`devbox`, `docker`, `tunnel`, `vm`, or a hypervisor's name). The picker labels a reserved key
("reserved for devbox"), and adopting a reserved key releases its reservation in the same change.

## Keeping a key for later

A key you keep on purpose for a VM that does not exist yet - say `~/.ssh/docker-tunnel`, kept in case you
run Docker in a VM again - need not be a `doctor` warning. Reserve it:

```sh
authama ssh key reserve docker-tunnel --for devbox --note "Docker devbox VM, kept for later" --dry-run
authama ssh key reserve docker-tunnel --for devbox --note "Docker devbox VM, kept for later"
authama ssh key list          # lists it under "reserved"
authama ssh show docker-tunnel
authama ssh key unreserve docker-tunnel   # loose again
```

The argument is the file's name, `~/.ssh/<name>` or its `.pub`; only a key pair directly in `~/.ssh` that
authama does not manage can be reserved. The reservation is a `reserved_keys` entry in `manifest.json`
(see [the manifest reference](../configuration.md#reserved_keys)) with the public key's fingerprint and
the day; the key file is never moved, changed or read beyond its `.pub`. `doctor` then prints it under
"reserved keys (kept on purpose, not dangling)" and `--strict` passes; it warns only if the file is gone,
was replaced by a different key, or has since been adopted. When the VM exists,
`authama ssh vm adopt-key docker-tunnel ...` (or a `devboxes` entry and `vm sync`) adopts it and
releases the reservation.

## The web UI

`authama serve` has a **VMs** page (`/vms`): the VM hosts and their state, the pending sync plan with a
Preview/Apply of `ssh vm sync` (optionally `--repin`), the hypervisors with their CLI and VMs, the
declared devboxes (a planned one shows "no VM yet"), the reserved keys, an adopt-key form, and buttons to install or remove the daily sync. Every change shows
the exact CLI command and a preview first, as on the other pages.

---

[← Docs index](../../README.md#documentation)
