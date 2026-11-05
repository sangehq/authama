# Composer target

Writes Composer's `auth.json`, or the `COMPOSER_AUTH` export with `token apply --exports`.

## The two Composer facts that bite

1. **Composer cannot read environment variables inside `auth.json`** - there is
   no `${VAR}` interpolation, so authama writes the resolved values.
2. **A present `auth.json` overrides `COMPOSER_AUTH`.** A stale file silently
   shadows the token you set in CI. `authama token audit` and `authama doctor`
   point it out.

Composer reads auth from four places, later wins: `COMPOSER_AUTH` → global
`auth.json` → project `auth.json` → inline in `composer.json`.

## Mechanisms

| Mechanism | Shape |
|---|---|
| `github-oauth` | `{"github.com": "TOKEN"}` |
| `gitlab-oauth` | `{"host": "TOKEN"}` |
| `gitlab-token` | `{"host": "TOKEN"}` |
| `bitbucket-oauth` | `{"host": {"consumer-key": "…", "consumer-secret": "…"}}` |
| `http-basic` | `{"host": {"username": "…", "password": "…"}}` |
| `bearer` | `{"host": "TOKEN"}` |
| `custom-headers` | `{"host": ["API-TOKEN: …"]}` |
| `forgejo-token` | `{"host": "TOKEN"}` |

Name a `service` instead of a mechanism; `authama token providers` lists them
(GitHub, GitHub Enterprise, GitLab, Bitbucket, Gitea, Forgejo, Codeberg, Azure
DevOps, sourcehut, Private Packagist, Satis, Repman, Packeton, Artifactory,
Cloudsmith, Nexus, Gemfury, unlock.sh, Magento, Laravel Nova and Spark, and two
generic ones). An explicit `type` still wins.

## Self-hosted GitLab / GitHub Enterprise

Add the host to `gitlab-domains` / `github-domains` in your `composer.json`
`config`, or Composer will not apply the token.

## Two identities on one host

`github-oauth` is one token per host. Keep the default account global and give
the second a project-scoped file:

```sh
GH_TOKEN="$(op read op://Acme/GitHub/token)" \
  authama token apply --target composer --project ~/code/acme-app
```

## CI

```sh
eval "$(GH_TOKEN=$TOKEN authama token apply --target composer --exports)"
composer install --no-interaction
```

Do not also leave an `auth.json` in CI: it overrides `COMPOSER_AUTH`.
Unlike auth-token-manager, a write never echoes `COMPOSER_AUTH` with the
secret in it; the hint after a write names the variable only (`--no-env-hint`
drops it).

## Merge

Writes merge into an existing `auth.json`: hosts and sections authama does not
manage are kept; only the configured hosts are added or replaced. A file that is
not a JSON object is refused, not replaced - it may be a half-finished edit that
still holds credentials. (auth-token-manager replaced it with a warning.)

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
