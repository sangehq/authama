# PyPI target

Writes sections in `~/.pypirc`, read by `twine` and other PyPI uploaders, and keeps `[distutils]`'s
`index-servers` in step with them.

## Config

```json
{
  "pypi": {
    "upload.pypi.org": { "user_env": "PYPI_USER", "pass_env": "PYPI_TOKEN" },
    "test.pypi.org":   { "user_env": "TESTPYPI_USER", "pass_env": "TESTPYPI_TOKEN" }
  }
}
```

For a PyPI API token the user name is `__token__` and the token is the password.

## Output

```ini
[distutils]
index-servers =
    pypi
    testpypi

[pypi]
username = __token__
password = <token>

[testpypi]
repository = https://test.pypi.org/legacy/
username = __token__
password = <token>
```

A host other than `upload.pypi.org` or `test.pypi.org` gets a section of its own name, with
`repository = https://<host>/`.

## Location

`~/.pypirc`, written `0600`.

## Installing packages

`.pypirc` is for uploading. `pip` and `uv` read credentials for a private index from `~/.netrc`, so
use the [netrc](netrc.md) target for the index host (`machine pkgs.example.com`).

## Merge

By section: every other section and setting is kept; each configured host's section is added or
replaced. A user name or password containing whitespace is refused.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md)
