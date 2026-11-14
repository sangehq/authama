# Webhooks

`authama webhook` sends each new event to endpoints you add, as it happens: a change finished, or a
watched file changed. Requests are signed as [Standard Webhooks](https://www.standardwebhooks.com)
1.0.0 describes. Webhooks are off by default.

## Turn them on

```sh
authama webhook add ops https://hooks.example.com/authama   # prints its signing secret once
authama settings set webhooks.enabled true
authama serve                                               # sends what is due every 15 seconds
```

An endpoint is sent what happens from when it is added, never the history before it.

| Command | What it does |
|---|---|
| `authama webhook add <name> <url> [--event <prefix>]...` | adds an endpoint and prints its `whsec_` secret once; `--event` sends only types starting with a prefix, whole or without `com.simtabi.authama.` (`operation.finished`, `file.`) |
| `authama webhook list [--json]` | endpoints and how their delivery stands; never a secret |
| `authama webhook remove <name>` | forgets an endpoint and its secret |
| `authama webhook disable <name>`, `enable <name>` | pauses an endpoint, or resumes it from where it stopped |
| `authama webhook test <name>` | sends one `com.simtabi.authama.webhook.tested` event now; moves nothing |
| `authama webhook deliver` | sends every endpoint what is due, without `serve` |

Endpoints and their secrets are kept in `webhooks.json` in the state home (mode 0600).

## What is sent

Each event is a `POST` of the [CloudEvents](api.md#the-event-feed) event itself, in structured mode,
with three Standard Webhooks headers:

```http
POST /authama HTTP/1.1
Content-Type: application/cloudevents+json; charset=utf-8
User-Agent: authama-webhooks/1
webhook-id: op_0f9c2e1a7b3d4c5e
webhook-timestamp: 1763000000
webhook-signature: v1,<base64 HMAC-SHA256>

{"specversion":"1.0","id":"op_0f9c2e1a7b3d4c5e","source":"https://opensource.simtabi.com/products/simtabi/authama/install/7f3c2a9e5b1d4c60",
 "type":"com.simtabi.authama.operation.finished","subject":"operation/0f9c2e1a7b3d4c5e","time":"2026-11-12T10:00:05Z",
 "datacontenttype":"application/json","data":{...},"authamasub":"ops"}
```

`webhook-id` is the event's `id`, the same on every retry. `authamasub` is the endpoint's name,
inside the signed body, so a delivery captured from one endpoint cannot be replayed to another that
checks it. Every attribute is inside the signature: no `ce-*` header is sent. This is sange's wire
format, so one receiver handles both. The signature is `v1,` and the
base64 HMAC-SHA256 of `<webhook-id>.<webhook-timestamp>.<body>`, keyed with the base64-decoded part of
the secret after `whsec_`. Any Standard Webhooks library verifies it. A receiver should:

- verify the signature over the exact bytes received, comparing in constant time;
- refuse a `webhook-timestamp` more than a few minutes from now;
- use `webhook-id` to drop a delivery it has already handled.

## Delivery

- Events go to each endpoint in order. A 2xx answer moves the endpoint on to the next event.
- Any other answer, a redirect, or no answer within 10 seconds is a failure. The same event is tried
  again after 5 seconds, then 5 minutes, 30 minutes, 2 hours, 5 hours, 10 hours, and then every 24
  hours.
- `410 Gone`, or 10 failures in a row, turns the endpoint off. `webhook list` says why;
  `webhook enable` resumes it from the event that failed.

## Security

- **Off by default.** Nothing is sent until `webhooks.enabled` is on, or you run `webhook deliver`.
- **https to public addresses only.** An endpoint on this machine or a private network is refused,
  so a webhook cannot be aimed at a service behind your firewall: loopback, 10/8, 172.16/12,
  192.168/16, 100.64/10, link-local such as `169.254.169.254`, IPv6 unique-local and site-local, and
  every other IANA special-purpose range (0.0.0.0/8, 192.0.0.0/24, the documentation and
  benchmarking ranges, multicast, 240/4 and broadcast). An IPv6 address that carries an IPv4 one
  (NAT64 `64:ff9b::/96`, 6to4 `2002::/16`, IPv4-compatible `::a.b.c.d`) is judged by the IPv4 address
  inside it. A host written as a bare number, in hex or octal, or shortened (`2130706433`,
  `0x7f.1`, `127.1`) is refused; write four dotted decimals or a name. The address is checked again
  when it is dialled, after its name resolves, so DNS cannot rebind around it. `webhooks.allow_private` lifts
  this, and also allows plain `http` to this machine, for a receiver you run locally.
- **No redirects** are followed, and no proxy from the environment is used.
- **One delivery at a time.** The server's loop and `authama webhook deliver` take turns, so an event
  is sent once, and a webhook removed or disabled while an event is in flight stays removed or
  disabled.
- **A user name or password in the address** is refused: the signature authenticates each request.
- Each endpoint has its own secret; a receiver only needs that one.

---

[← Docs index](../../README.md#documentation)
