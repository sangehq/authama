# netrc target

Writes `machine` entries in `~/.netrc` (`~/_netrc` on Windows), read by
`curl`, many HTTP client libraries, Heroku and others.

## Config

```json
{
  "netrc": {
    "api.heroku.com": { "user_env": "HEROKU_USER", "pass_env": "HEROKU_API_KEY" }
  }
}
```

## Output

```
machine api.heroku.com login me@example.com password <api-key>
```

## Location

`$NETRC` if set, else `~/.netrc` (`~/_netrc` on Windows). Written `0600`
(a user-only ACL on Windows): many tools refuse a group- or world-readable
netrc, so the mode is a correctness requirement as well as hygiene.

## Merge

By machine: entries for other hosts are kept; each configured host's entry is
added or replaced. Merge expects one entry per line (what authama and most
clients write) and does not keep `macdef` macros. A login or password containing
whitespace or a control character is refused: netrc has no quoting, so a space
starts a new keyword and could plant a second credential.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md)
