# Maven target

Writes repository credentials into `settings.xml`, which Maven reads for any repository or mirror
whose `<id>` matches a `<server>`.

## Config

```json
{
  "maven": {
    "acme-releases": { "user_env": "ACME_MAVEN_USER", "pass_env": "ACME_MAVEN_TOKEN" }
  }
}
```

The host is the server id, the same `<id>` the build gives the repository:

```xml
<repository>
  <id>acme-releases</id>
  <url>https://maven.example.com/releases</url>
</repository>
```

## Output

```xml
<servers>
  <server>
    <id>acme-releases</id>
    <username>user</username>
    <password>token</password>
  </server>
</servers>
```

A `{` or `}` in a value is written `\{` and `\}`, because Maven reads a braced value as an
encrypted one. A password Maven already holds encrypted (`{...}`, with `settings-security.xml`) is
read and kept as it is.

## Location

`~/.m2/settings.xml` (`%USERPROFILE%\.m2\settings.xml` on Windows). Written `0600`, as Maven's
encryption guide recommends. Maven 4's project `.mvn/settings.xml` ignores server credentials, so it
is never the target.

## Merge

By server: only the `<server>` element with that id is replaced, by splicing the text around it, so
every other element, comment and blank line stays byte for byte. A file with no `<servers>` gets one
before `</settings>`; a missing file becomes a minimal `settings.xml`. A file that is not a settings
document is left alone.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md) · [Maven](maven.md) · [Yarn Berry](yarn.md) · [Helm](helm.md) · [GitHub CLI](gh.md)
