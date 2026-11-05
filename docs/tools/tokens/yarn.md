# Yarn Berry target

Writes a registry token into Yarn 2+'s home `.yarnrc.yml`, which every project on the machine
reads after its own `.yarnrc.yml`.

## Config

```json
{
  "yarn": {
    "npm.pkg.github.com": { "env": "GH_PACKAGES_TOKEN" }
  }
}
```

The host is the registry, with or without a path; it is written under `npmRegistries` as
`"//npm.pkg.github.com"`, the spelling Yarn falls back to for any scheme. An entry already there as
`"https://npm.pkg.github.com"` is updated in place rather than duplicated.

## Output

```yaml
npmRegistries:
  "//npm.pkg.github.com":
    npmAuthToken: "token"
    npmAlwaysAuth: true
```

Bind a scope to the registry yourself, under `npmScopes`, with `npmRegistryServer`; the scope then
uses this token.

## Location

`~/.yarnrc.yml`. Written `0600`. Yarn 1 (classic) reads `~/.npmrc`; use the [`npm`](npm.md) target for it.

## Merge

By registry: `npmAuthToken` and `npmAlwaysAuth` are set on that registry; its other keys, the other
registries, the scopes and the rest of the file are kept, comments included.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
