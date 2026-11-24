# ssh service - reachability services

```sh
authama ssh service list [--json]
authama ssh service show <name>
authama ssh service health [name] [--json]   # also: ssh service status
authama ssh service add <name> --kind <kind> [--binary B] [--label L]
authama ssh service edit <name> [--kind] [--binary] [--label]
authama ssh service remove <name> [-y]
authama ssh host add|edit … --service <name> [--remove-service <name>]
authama ssh host variant set … --service <name>
```

A service is how a host is reached: Cloudflare Access (`cloudflare-access`), WARP (`cloudflare-warp`), Tailscale,
a bastion, Teleport, AWS SSM, 1Password's agent, Secretive, or a VPN. Services attach to hosts and route variants
(`services[]` in the manifest), not to key folders. Named catalog entries live in `defaults.services`; a host may
also name a built-in kind directly.

`ssh service health` is read-only. It looks up a binary on `PATH` (`cloudflared --version`, `warp-cli status`,
`tailscale status`, and so on). It never starts a tunnel, changes WARP, or opens a browser.

## In the web UI

The **Services** page (`/services`) shows health, catalog entries, and which hosts attach each service. Adding or
removing a catalog entry goes through the change preview. Attach a service to a host with `ssh host edit --service`.
