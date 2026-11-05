# Cargo target

Writes registry tokens in Cargo's `credentials.toml`, read by `cargo publish`, `cargo login` and
`cargo install` from an authenticated registry.

## Config

```json
{
  "cargo": {
    "crates.io": { "env": "CARGO_REGISTRY_TOKEN" },
    "acme":      { "env": "ACME_CARGO_TOKEN" }
  }
}
```

The host is `crates.io` or the name `.cargo/config.toml` gives an alternate registry.

## Output

```toml
[registry]
token = "<token>"

[registries.acme]
token = "<token>"
```

## Location

`$CARGO_HOME/credentials.toml`, else `~/.cargo/credentials.toml`. Written `0600`. Dev mode hides
`CARGO_HOME`.

## Merge

By table: every other table is kept; each configured registry's table is added or replaced. A
registry name is letters, digits, `-` and `_`.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
