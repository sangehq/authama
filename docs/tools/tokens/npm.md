# npm target

Writes the registry auth lines in `.npmrc`.

## Config

```json
{
  "npm": {
    "registry.npmjs.org": { "env": "NPM_TOKEN" },
    "npm.pkg.github.com": { "env": "GH_TOKEN", "scope": "@your-org" }
  }
}
```

- `env` - the variable holding the registry token.
- `scope` (optional) - binds `@your-org` to this registry with a
  `@your-org:registry=https://host/` line.
- `always_auth` (optional) - adds `:always-auth=true`.

## Output

```
@your-org:registry=https://npm.pkg.github.com/
//npm.pkg.github.com/:_authToken=ghp_xxx
//registry.npmjs.org/:_authToken=npm_xxx
```

## Location

`$NPM_CONFIG_USERCONFIG` if set, else `~/.npmrc`; or `--project DIR` for
`DIR/.npmrc`, which overrides the user file for commands run in that directory.

An npm token containing whitespace, and a scope that does not look like
`@org`, are refused.

## Merge

Your other settings (`save-exact`, proxies, registries authama does not manage)
are kept; only the `:_authToken` (and `@scope:registry`) lines for the
configured hosts are added or replaced.

## CI

```sh
NPM_TOKEN=$TOKEN authama token apply --target npm --project .
npm ci
```

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md)
