# `git` — remotes over each profile's SSH key, and commits signed with it

`authama ssh git` does two jobs for git:

- **Remotes.** `git status` shows how each checkout's GitHub `origin` authenticates, `git use` points it at
  one of three modes after proving the new URL works, `git resolve` says how a repository would be reached,
  and `git clone` clones through the right profile's alias.
- **Identity and signing.** `git sign` makes the checkouts under a profile's directories commit as that
  profile (its email and name) and sign commits and tags with its SSH key, and checks those signatures.

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
     "applied": false, "skipped": "", "error": "",
     "owner": "acme", "repo": "app", "profile": "alice", "profile_from": "owner"},
    {"path": ".../notes", "from": "", "to": "", "applied": false,
     "skipped": "no origin (not a git checkout, or no remote named origin)", "error": "",
     "owner": "", "repo": "", "profile": "", "profile_from": ""}
  ]
}
```

| Field | Meaning |
|---|---|
| `from`, `to` | the origin before, and the URL for the mode; `to` is empty when the checkout was skipped first |
| `applied` | origin was rewritten (never on a dry run) |
| `skipped` | why nothing changed, such as `already profile`; empty when it changed or would |
| `error` | why the checkout was left as it was, such as a failed `ls-remote`; the command then exits non-zero |
| `owner`, `repo` | the repository the origin points at; empty when the checkout was skipped before its origin was read |
| `profile` | the profile the new URL is built for; empty when none is given or mapped (ssh and https need none) |
| `profile_from` | `flag` (`--profile`) or `owner` (`defaults.git.orgs`, then `defaults.git.profile`); empty with `profile` |

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

## `authama ssh git clone OWNER/REPO [DIR]`

Clones over the URL `resolve` gives (the owner's profile, the default mode; `--profile` and `--mode` override
them), so the checkout starts on the right key. When the profile has a git identity that is applied, the new
checkout also includes its fragment, as `git sign use` does, so the first commit is already made and signed as
that profile. `--dry-run` prints the URL and directory and clones nothing.

```sh
authama ssh git clone acme/app ~/code/acme/app
authama ssh git clone me/dotfiles --profile personal --dry-run
```

## `authama ssh git sign` — commit as a profile, signed with its SSH key

SSH signing needs **git 2.34 or newer**. `ssh git sign status`, `ssh git sign verify` and the
`/git-sign` page name the version found, and how to upgrade, when it is older; nothing is written
for an older git. Git runs inside each checkout (its working directory), never with `-C`. The
allowed signers file reaches `git log` through `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_0`/`GIT_CONFIG_VALUE_0`,
so nothing is added to the checkout's config to verify it.

Each profile can carry a git identity in the manifest:

```jsonc
"profiles": {
  "work": {
    "git": {
      "email": "ada@acme.example",   // user.email, and the signer's principal in allowed_signers
      "name": "Ada Lovelace",         // user.name; optional
      "signing_key": "",              // a key of this profile; empty = its github.com host's key
      "sign": true,                   // sign commits and tags; false writes the identity only
      "dirs": ["~/code/acme"]         // checkouts under these commit as this profile
    },
    ...
  }
}
```

| Verb | Does |
|---|---|
| `sign set <profile> --email E [--name N] [--key K] [--dir D]... [--no-sign] [--clear]` | Writes the profile's `git` block (each `--dir` replaces the list), then applies. `--no-apply` changes the manifest only. |
| `sign apply [--dry-run] [-y]` | Renders every profile's block, showing each change as a diff first. |
| `sign status [PATH...] [--json]` | Each profile's identity and key, whether `apply` is due, and for each checkout the email, signing key and format git resolves there and the file the email comes from. |
| `sign use <profile> [PATH...] [--dry-run] [-y]` | Makes checkouts commit as the profile wherever they are, through their own `include.path`; another profile's fragment is taken out, includes that are not authama's stay. |
| `sign verify [PATH] [-n N] [--strict] [--json]` | Checks the last N commits (default 10) with git's own signature check against authama's `allowed_signers`. Exits non-zero on a bad signature, and with `--strict` on any commit that is not a good signature from an allowed signer. |

What `apply` writes, every time from the manifest:

| File | Holds |
|---|---|
| `<config home>/git/<profile>.gitconfig` (0600) | `user.email`, `user.name`, `user.signingkey` (the key's `.pub`), and when it signs `gpg.format = ssh`, `gpg.ssh.allowedSignersFile`, `commit.gpgsign` and `tag.gpgsign` |
| `<config home>/git/allowed_signers` (0600) | `<email> namespaces="git" <public key>` for every signing profile |
| the global git config | one managed block: an `[includeIf "gitdir:<dir>/"]` per directory, pointing at the profile's fragment |

The global git config is `$GIT_CONFIG_GLOBAL`, else `~/.gitconfig` (or `$XDG_CONFIG_HOME/git/config` when only
that exists); in a sandbox (`--dev-root`) it is the sandbox home's `.gitconfig`. Only the block between the
`# >>> authama: git identities` markers is ever changed; a copy of the file is kept under the state home's
`backups/gitconfig/` before each change, and a symlinked `~/.gitconfig` (a dotfiles checkout) is written through,
so the link stays. A fragment whose profile no longer has a git block is removed, and so is its include.

**Keys.** Signing names the public half; git runs `ssh-keygen -Y sign`, which uses the private key next to it
(or the agent). No private key is read or copied. A profile with `key_store: 1password` signs through
1Password's `op-ssh-sign` with `signingkey = key::<public key>`. A key not made yet leaves signing off and says
so (`authama ssh reconcile` mints it); the identity is still written. Upload the same public key to GitHub as a
**signing key** (Settings → SSH and GPG keys → New SSH key → Key type: Signing Key) for GitHub to show the
commits as Verified.

`gitdir:` is matched against the checkout's real path: give a directory behind a symlink by its resolved path.
Two profiles cannot claim one directory; the manifest refuses it.

## In the web UI

**SSH → Git identities** (`/git-sign`, `g g` in the palette) does the same in the browser:

- Each profile has a card with its email, name, the key that signs, its directories and whether its fragment
  is written. **Set a git identity** (or **Change the identity**) goes through the usual preview: the manifest
  diff, then every line the fragment, `allowed_signers` and the managed block of the global git config gain or
  lose. Nothing is written until you confirm, and the owner PIN is asked as for any change. **Remove the
  identity** is `sign set --clear`.
- **Preview the apply** is `sign apply` when the git files lag behind the manifest.
- **Use it in checkouts elsewhere** is `sign use`: give each checkout's full path (or `~/…`).
- **Check a checkout** reads how a checkout commits and checks its recent commits' signatures (`sign status`
  and `sign verify`); it changes nothing.
- **Remote mode** is `ssh git status` and `ssh git use`: give checkouts' full paths, one a line, to see each
  origin's mode, owner, repository and profile. **Preview the switch** points them at `profile`, `ssh` or `https`
  (or the default), through the owner's profile or one you choose. The preview lists what each origin would go
  from and to and changes nothing. Confirming, with the PIN, proves each new URL with `git ls-remote` before it
  rewrites origin, as the command line does. A checkout that fails is left as it was, and the rest still change.
- **Clone through a profile** works out the URL, profile and mode `ssh git clone` would use and gives the
  command; the clone itself runs at a terminal.
- `allowed_signers` is shown as it is: public keys only.

A read-only server (`serve --read-only`) shows the same status with the commands to run instead of forms.


---

[← Docs index](../../README.md#documentation)
