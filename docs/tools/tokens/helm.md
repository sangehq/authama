# Helm targets

Two targets, because Helm keeps two kinds of credential in two files:

| Target | For | File |
|---|---|---|
| `helm` | a classic chart repository (`helm repo add`) | `repositories.yaml` |
| `helm-oci` | an OCI registry (`helm registry login`, `oci://` charts) | `registry/config.json` |

A token for an OCI registry belongs in `registry/config.json`; `repositories.yaml` lists only named
classic repositories.

## Config

```json
{
  "helm": {
    "https://charts.example.com/stable": { "user_env": "ACME_CHARTS_USER", "pass_env": "ACME_CHARTS_TOKEN" }
  },
  "helm-oci": {
    "ghcr.io": { "user_env": "GITHUB_USER", "pass_env": "GH_PACKAGES_TOKEN" }
  }
}
```

For `helm` the host is the repository's URL. The entry `helm repo add` made for it gets the user name
and password; when there is none, one is added under a name made from the URL
(`charts.example.com-stable`). For `helm-oci` the host is the registry's host, never `oci://` or a
path.

## Output

```yaml
repositories:
- name: acme
  url: https://charts.example.com/stable
  username: "user"
  password: "token"
```

```json
{ "auths": { "ghcr.io": { "auth": "<base64 of user:token>" } } }
```

## Location

Helm's config home is `$HELM_CONFIG_HOME`, else `~/.config/helm` (Linux, or `$XDG_CONFIG_HOME/helm`),
`~/Library/Preferences/helm` (macOS) or `%APPDATA%\helm` (Windows); `helm env` prints it.

- `helm`: `$HELM_REPOSITORY_CONFIG`, else `<config home>/repositories.yaml`.
- `helm-oci`: `$HELM_REGISTRY_CONFIG`, else `<config home>/registry/config.json`.

Both are written `0600`. Dev mode hides all three variables.

## Merge

`helm`: by URL; only `username` and `password` change on the matching entry, every other repository
and field is kept. Deleting the credential leaves the repository itself in place.

`helm-oci`: by host, in Docker's format; every other key is kept. A `credsStore` or `credHelpers`
named in the file takes precedence over what is written here, as it does for Docker, so on a machine
where one is set the entry is written but not used.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
