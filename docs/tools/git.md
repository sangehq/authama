# `git` — switch checkouts between profile SSH, plain SSH and HTTPS

Two verbs: `git status` shows how each checkout's GitHub `origin` authenticates, and `git use` points it at
one of three modes after proving the new URL works.

| Mode | Origin URL | Authenticates as |
|---|---|---|
| `profile` (default) | `git@<profile's alias>:owner/repo.git`, e.g. `git@github-alice:acme/app.git` | that profile's GitHub key |
| `ssh` | `git@github.com:owner/repo.git` | the key bound to the rendered `Host github.com` (`defaults.git.profile`), written into that profile's category config when it has one |
| `https` | `https://github.com/owner/repo.git` | your git credential helper (for example `gh-app git-credential`) |

## Choosing the mode

`git use` takes the mode from, in order: the first argument when it names a mode, `--mode`,
`$AUTHAMA_GIT_MODE`, `defaults.git.mode` in the manifest, then `profile`.

## Choosing the profile

The repository owner picks the profile: `defaults.git.orgs` (owner → profile, case-insensitive), then
`defaults.git.profile`, then the only profile with a `github.com` host. With several candidates and no
mapping it stops and asks for `--profile` instead of guessing, because the wrong guess pushes as the wrong
account. `--profile` overrides the mapping for one run.

```jsonc
"defaults": {
  "git": {
    "mode": "profile",
    "profile": "alice",
    "orgs": {"acme": "alice", "acme-labs": "alice", "clientco": "example-user"}
  }
}
```

## `authama ssh git status [PATH...]`

```text
$ authama ssh git status ~/code/acme/*
.../app       https    acme/app      profile=alice (by owner)
.../api       profile  acme/api      profile=alice
.../notes     -  no origin
```

`--json` prints the same as one document, for scripts and other tools:

```json
{
  "version": 1,
  "default_mode": "profile",
  "checkouts": [
    {"path": ".../app", "state": "managed", "url": "https://github.com/acme/app.git",
     "mode": "https", "owner": "acme", "repo": "app", "profile": "alice", "profile_from": "owner"},
    {"path": ".../notes", "state": "no-origin"}
  ]
}
```

| Field | Meaning |
|---|---|
| `state` | `managed` (a GitHub origin `git use` can switch), `unmanaged` (another host; `url` says which) or `no-origin` |
| `mode` | `profile`, `ssh` or `https` |
| `profile` | the profile whose key the checkout uses; absent when nothing names one |
| `profile_from` | `remote` (the origin's `github-<profile>` alias names it) or `owner` (`defaults.git.orgs`, then `defaults.git.profile`) |
| `default_mode` | `defaults.git.mode`, or `profile` when unset |

Fields are only ever added. A change that removes or renames one bumps `version` and is noted in the
CHANGELOG; `TestGitStatusJSONKeepsItsFields` fails before such a change can land unannounced.

## `authama ssh git use [MODE] [PATH...]`

```sh
authama ssh git use profile ~/code/*/ --dry-run   # show every rewrite, change nothing
authama ssh git use profile ~/code/*/ --yes       # rewrite, each URL checked first
authama ssh git use https .                        # back to HTTPS in this checkout
AUTHAMA_GIT_MODE=ssh authama ssh git use .          # one-off override of the default
```

What it does per checkout:

1. Reads `origin`. No origin, or an origin that is not GitHub (GitLab, another host's alias), is skipped
   and said so.
2. Builds the URL for the mode. `ssh` mode needs `defaults.git.profile`; `profile` mode needs the
   profile to have a `github.com` host.
3. Runs `git ls-remote` against the new URL, with prompts off and SSH in batch mode. Only if that works
   is `origin` rewritten; otherwise the checkout is left as it was and named in the error. A 1Password
   key whose app is locked fails here, before anything changes.

`--dry-run` stops after step 2. The command exits non-zero if any checkout was left unchanged by an error.

`--json` prints one document instead of lines, with or without `--dry-run`, so a tool such as sange can
compare its view of the same checkouts with authama's:

```json
{
  "version": 1,
  "mode": "profile",
  "dry_run": true,
  "checkouts": [
    {"path": ".../app", "from": "https://github.com/acme/app.git", "to": "git@github-alice:acme/app.git",
     "applied": false, "skipped": "", "error": ""},
    {"path": ".../notes", "from": "", "to": "", "applied": false,
     "skipped": "no origin (not a git checkout, or no remote named origin)", "error": ""}
  ]
}
```

| Field | Meaning |
|---|---|
| `from`, `to` | the origin before, and the URL for the mode; `to` is empty when the checkout was skipped first |
| `applied` | origin was rewritten (never on a dry run) |
| `skipped` | why nothing changed, such as `already profile`; empty when it changed or would |
| `error` | why the checkout was left as it was, such as a failed `ls-remote`; the command then exits non-zero |

## `authama ssh git resolve OWNER/REPO`

Says how a GitHub repository would be reached, with no checkout and without changing anything. The
profile is `--profile`, else the owner's (`defaults.git.orgs`, then `defaults.git.profile`); the mode is
`--mode`, else the same order as `use`.

```text
$ authama ssh git resolve acme/app
profile:      alice
mode:         profile
url:          git@github-alice:acme/app.git
host alias:   github-alice
key:          alice/alice_github-alice-ed25519  SHA256:...
```

`--json` gives the shape sange's companion contract asks for, so its names are camelCase:

```json
{"version": 1, "profile": "alice", "mode": "profile", "url": "git@github-alice:acme/app.git",
 "hostAlias": "github-alice", "keyFingerprint": "SHA256:..."}
```

| Field | Meaning |
|---|---|
| `hostAlias` | the SSH alias the URL connects through: the profile's `github.com` host in `profile` mode, `github.com` in `ssh` mode, empty in `https` mode |
| `keyFingerprint` | that host's key, as the inventory recorded it; empty in `https` mode and for a key not made yet |

Both `use --json` and `resolve --json` keep the promise `status --json` makes: fields are only ever
added, and the same test guards them.

Glob `packages/*`, not `*/`, where parked copies share names with live ones.

---

[← Docs index](../../README.md#documentation)
