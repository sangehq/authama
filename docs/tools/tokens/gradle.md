# Gradle target

Writes repository credentials in `gradle.properties`, which a build reads with
`credentials(PasswordCredentials)`.

## Config

```json
{
  "gradle": {
    "acme": { "user_env": "ACME_MAVEN_USER", "pass_env": "ACME_MAVEN_TOKEN" }
  }
}
```

The host is the repository's name in the build:

```kotlin
repositories {
    maven {
        name = "acme"
        url = uri("https://maven.example.com/releases")
        credentials(PasswordCredentials::class)
    }
}
```

## Output

```properties
acmeUsername=<user>
acmePassword=<password>
```

## Location

`$GRADLE_USER_HOME/gradle.properties`, else `~/.gradle/gradle.properties`. Written `0600`. Dev mode
hides `GRADLE_USER_HOME`.

## Merge

By property: every other property is kept; each configured repository's two properties are added
or replaced, escaped as a properties file needs.

<!-- nav-footer -->

---

[← Docs index](../../../README.md#documentation) · [`token`](../tokens.md) · [Composer](composer.md) · [npm](npm.md) · [Docker](docker.md) · [git](git.md) · [netrc](netrc.md) · [PyPI](pypi.md) · [Cargo](cargo.md) · [RubyGems](gem.md) · [Gradle](gradle.md)
