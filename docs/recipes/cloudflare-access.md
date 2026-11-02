# Reach a server through Cloudflare Access

Put a host behind Cloudflare Access and `ssh <alias>` signs in through the browser, then reaches it over a tunnel
the server opened itself, so port 22 never has to be open to the internet.

The Cloudflare side (Zero Trust organisation, identity provider, one tunnel per server with an SSH route, and an
Access policy) is set up once in the Cloudflare dashboard. This recipe is the laptop side.

```bash
brew install cloudflared
authama ssh host add acme app-web -H ssh.example.com -u admin --proxy cloudflared
authama ssh reconcile
ssh app-web
```

The first `ssh` opens a browser to sign in; after that the Access session lasts as long as the policy allows. To
move an existing host behind Access, change its hostname to the Access hostname and add the proxy:

```bash
authama ssh host edit acme app-web -H ssh.example.com --proxy cloudflared
```

`authama ssh host edit ... --proxy none` takes it off again. `authama doctor` reports a proxied host when `cloudflared`
is missing.

Reference: [`proxy`](../configuration.md#proxy).

---

[← Docs index](../../README.md#documentation)
