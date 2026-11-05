# RubyGems target

Writes API keys in `~/.gem/credentials`, read by `gem push`.

## Config

```json
{
  "gem": {
    "rubygems.org":                 { "env": "RUBYGEMS_API_KEY" },
    "rubygems.pkg.github.com/acme": { "env": "GH_PACKAGES_TOKEN" }
  }
}
```

## Output

```yaml
---
:rubygems_api_key: <key>
"https://rubygems.pkg.github.com/acme": <key>
```

## Location

`~/.gem/credentials`, written `0600`: `gem` refuses the file unless it is owner-only.

## Merge

By key: every other key is kept; each configured host's key is added or replaced. The file is the
flat map `gem` writes; nested YAML is not read.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
