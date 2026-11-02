# Use a local VM as a devbox

Reach a Parallels, VMPal, VMware, UTM or Lima VM by name, whatever IP it booted with, and point Docker at it.

```sh
authama ssh vm list                                   # find the VM's name
authama ssh profile add devbox --category personal
authama ssh vm add devbox devbox --hypervisor vmpal --vm "Ubuntu 24.04" -u dev
authama ssh reconcile                                 # mint devbox_devbox-ed25519
```

Install the public key in the guest once, for example through the hypervisor:

```sh
vmpal exec "Ubuntu 24.04" -- sh -c 'mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys' \
  < ~/.ssh/profiles/personal/devbox/devbox_devbox-ed25519.pub
```

Then connect, and give Docker a context:

```sh
ssh devbox                      # starts the VM if it is stopped
authama ssh vm docker-context devbox
docker --context devbox ps
```

OrbStack and Podman keep their own SSH and keys; use their contexts instead.

## Already have a key for the VM?

If a key for the devbox already sits loose in `~/.ssh` (say `~/.ssh/devbox-tunnel`), adopt it instead of
minting a new one. Tell authama which hypervisors you use first, then let it guide you:

```sh
# .env (or "vm": {"hypervisors": "parallels,vmpal"} in authama.json)
AUTHAMA_VM_HYPERVISORS="parallels,vmpal"

authama ssh vm adopt-key                        # pick the key, hypervisor, VM and guest user
# or, non-interactively, previewed first:
authama ssh vm adopt-key devbox-tunnel devbox devbox --hypervisor parallels \
  --vm "Ubuntu 24.04" --user dev --docker-context --dry-run
```

The pair is copied into the profile; the originals stay where they are unless `--move`.

No VM yet, but want to keep the key? Reserve it, and `doctor` stops calling it loose:

```sh
authama ssh key reserve devbox-tunnel --for devbox --note "for the Ubuntu VM, later"
```

Adopting it later (above) releases the reservation. A `devboxes` entry in `authama.json` with a `key`
and no `vm` does the same (see [tools/vm.md](../tools/vm.md#declaring-devboxes)).

## Keep it up to date

```sh
authama ssh vm sync --dry-run       # what would change
authama ssh vm sync                 # pin, re-install the key if the guest lost it, refresh the Docker context
authama ssh vm sync --install-schedule   # do it daily (sync --quiet --yes)
```

If the VM is rebuilt, sync puts the key back in the new guest (`--repin` accepts its new host key). If
the VM is deleted, sync backs up the manifest and removes the host entry and its Docker context; the
key files stay. A suspended VM is left alone. `authama doctor --fix` runs the same sync. To have sync
create the host for you on a new machine, declare it in `authama.json`'s `devboxes` section
(see [`vm`](../tools/vm.md#declaring-devboxes)).

Reference: [`vm`](../tools/vm.md).

---

[← Docs index](../../README.md#documentation)
