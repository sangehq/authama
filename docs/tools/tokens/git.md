# git target

Writes `~/.git-credentials`, the file git's `store` credential helper reads.

```sh
git config --global credential.helper store
```

## Config

```json
{
  "git": {
    "github.com":    { "user_env": "GH_USER", "pass_env": "GH_TOKEN" },
    "git.acme.corp": { "user_env": "ACME_USER", "pass_env": "ACME_TOKEN" }
  }
}
```

## Output

```
https://octocat:changeme@github.com
```

The user name and password are URL-encoded, so values with `@`, `:` or `/`
work. A host containing whitespace, a control character or `@` is refused
(put the user name in `--username`), as is a value containing a newline, which
would plant a second credential line.

## Location

`~/.git-credentials`, or `$XDG_CONFIG_HOME/git/credentials` when that exists
and `~/.git-credentials` does not - the order git searches. On Windows it is
under `%USERPROFILE%`.

## Merge

By host: lines for other hosts are kept; each configured host's line is added or
replaced.

## Git over SSH instead

For GitHub and GitLab remotes over SSH, with one key per identity, see
[`authama ssh git`](../git.md) and the
[two GitHub accounts recipe](../../recipes/two-github-accounts.md).

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
