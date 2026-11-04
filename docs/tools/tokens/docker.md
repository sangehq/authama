# Docker target

Writes the `auths` section of Docker's `config.json`: `base64(user:password)`,
exactly what `docker login` writes when no credential store is configured.

## Config

```json
{
  "docker": {
    "ghcr.io":             { "user_env": "GH_USER", "pass_env": "GH_TOKEN" },
    "registry.gitlab.com": { "user_env": "GITLAB_USER", "pass_env": "GITLAB_TOKEN" }
  }
}
```

## Location

`$DOCKER_CONFIG/config.json` if set, else `~/.docker/config.json`
(`%USERPROFILE%\.docker\config.json` on Windows).

## Credential stores

Docker reads a registry from a credential store (`credsStore`, such as
`desktop` or `osxkeychain`) or a per-registry helper (`credHelpers`) before it
looks at inline `auths`. An inline entry for such a registry does nothing while
looking as if it worked - auth-token-manager wrote it anyway. authama asks what
to do instead:

| Mode | What happens |
|---|---|
| `login` | Runs `docker login <host> --username <user> --password-stdin`, so the credential lands in the store Docker reads. |
| `refuse` | Skips the registry with a warning naming the store. |

The mode comes from `--creds-store-mode`, else `_settings.docker_creds_store`
in the token config, else a prompt at a terminal. Without a terminal and with no
mode set, the registry is refused. `login` is refused under `--dev-root`,
because it would reach the real store. The web UI's `token apply` uses the
config setting and otherwise refuses.

## Merge

Writes merge into `config.json`: `credsStore`, `credHelpers`, `proxies` and
`auths` for other registries are kept; only the configured registries are added
or replaced.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md)
