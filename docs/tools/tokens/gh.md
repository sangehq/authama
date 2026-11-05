# GitHub CLI target

Writes an account's token into the GitHub CLI's `hosts.yml`, for a machine where gh's token is not in
the system keyring.

## Config

```json
{
  "gh": {
    "github.com": { "user_env": "GITHUB_USER", "pass_env": "GH_TOKEN" }
  }
}
```

The host is `github.com` or a GitHub Enterprise Server host. The user name is the account's login,
which gh needs: it keeps a token per account.

## Output

```yaml
github.com:
  users:
    alice:
      oauth_token: "token"
  oauth_token: "token"
  user: "alice"
  git_protocol: "https"
```

The token is written where gh reads it when it was not put in the keyring (`gh auth login
--insecure-storage`): under the account, and as the host's own token with `user`, so the account is
the active one and `gh auth switch` keeps it. A new host gets `git_protocol: https`; an existing
choice is kept.

`GH_TOKEN` and `GITHUB_TOKEN` (or `GH_ENTERPRISE_TOKEN` and `GITHUB_ENTERPRISE_TOKEN` for an
Enterprise host) take precedence over the file, so in CI exporting the variable is often all gh needs.

## Location

`$GH_CONFIG_DIR/hosts.yml`, else `$XDG_CONFIG_HOME/gh/hosts.yml`, else `%APPDATA%\GitHub CLI\hosts.yml`
on Windows, else `~/.config/gh/hosts.yml` (macOS included). Written `0600`. Dev mode hides
`GH_CONFIG_DIR`.

## Merge

By host and account: the account's token, the host's `oauth_token` and `user` are set; other
accounts on the host, other hosts and every other key are kept. Deleting it removes that account's
token and the host's, and leaves the rest.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
