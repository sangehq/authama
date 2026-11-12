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
| `authama webhook add <name> <url> [--event <prefix>]...` | adds an endpoint and prints its `whsec_` secret once; `--event` sends only types starting with a prefix (`operation.finished`, `file.`) |
| `authama webhook list [--json]` | endpoints and how their delivery stands; never a secret |
| `authama webhook remove <name>` | forgets an endpoint and its secret |
| `authama webhook disable <name>`, `enable <name>` | pauses an endpoint, or resumes it from where it stopped |
| `authama webhook test <name>` | sends one `webhook.test` event now; moves nothing |
| `authama webhook deliver` | sends every endpoint what is due, without `serve` |

Endpoints and their secrets are kept in `webhooks.json` in the state home (mode 0600).

## What is sent

Each event is a `POST` with a JSON body and three headers:

```http
POST /authama HTTP/1.1
Content-Type: application/json
webhook-id: op_0f9c2e1a7b3d4c5e
webhook-timestamp: 1763000000
webhook-signature: v1,K5oZfzN95Z9UVu1EsfQmfVNQhnkZ2pj9o9NDN/H/pI4=

{"type":"operation.finished","timestamp":"2026-11-12T10:00:05Z","data":{"op_id":"0f9c2e1a7b3d4c5e","command":"ssh rotate", ...}}
```

The types and their data are the [event feed's](api.md#the-event-feed). The signature is `v1,` and the
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
- **https to public addresses only.** An endpoint on this machine or a private network (loopback,
  10/8, 172.16/12, 192.168/16, 100.64/10, link-local such as `169.254.169.254`) is refused, so a
  webhook cannot be aimed at a service behind your firewall. The address is checked when it is
  dialled, after its name resolves, so DNS cannot rebind around it. `webhooks.allow_private` lifts
  this, and also allows plain `http` to this machine, for a receiver you run locally.
- **No redirects** are followed, and no proxy from the environment is used.
- **A user name or password in the address** is refused: the signature authenticates each request.
- Each endpoint has its own secret; a receiver only needs that one.

---

[← Docs index](../../README.md#documentation)
