# Security Policy

Where this file is silent, the [Simtabi security policy](https://github.com/simtabi/.github/blob/HEAD/SECURITY.md) applies.

## Reporting a vulnerability

The preferred channel is GitHub private vulnerability reporting: open a private report at <https://github.com/simtabi/ssh-manager/security/advisories/new>. The report stays attached to the repository, with a draft advisory and a CVE request path. Email **security@simtabi.com** if you do not use GitHub.

Do **not** open a public issue for a security report.

Include, where possible: a description of the issue, affected version(s),
reproduction steps, and any suggested remediation. We aim to acknowledge reports
within a few business days and will keep you updated on remediation progress.

## Scope notes for this project

`ssh-manager` (the `authama` binary, also installed as `sshmgr`) manages SSH
private keys and config, and the auth-token files package managers and git
read. A few invariants are security-load-bearing - please flag any regression
in these:

- **Secrets never touch git.** Private keys live in `~/.ssh`; the per-user home
  (the OS-standard config dir, e.g. `~/.config/ssh-manager`) holds `.env`,
  `age-identity.txt`, `*.age` bundles, `log/audit.log`, and `snapshots/`. Both live
  outside any repository; in the project checkout the equivalent paths are also
  gitignored, and a `gitleaks` CI job scanning the full history is the safety
  net.
- **Key passphrases.** When you generate passphrase-protected keys, the
  passphrase is passed to `ssh-keygen` on its command line (it has no stdin
  channel for generation), so it is briefly visible to other local users via
  `ps`/`/proc`. Avoid passphrase generation on shared multi-user hosts.
- **Provider tokens at rest.** A provider `token_env` (e.g. `GH_TOKEN`) in the
  home `.env` is plaintext at mode `0600`. To keep it out of plaintext, set the
  value to `cmd:<command>` and the token is fetched at use-time from any secret
  manager (it never touches disk):
  `GH_TOKEN=cmd:op read op://Private/GitHub/token`, or
  `cmd:keyring get ssh-manager gh_token`, or `cmd:age -d -i <id> token.age`.
- **Permissions are enforced.** Directories `700`, private keys + `config` `600`,
  `.pub` + `known_hosts` `644` - set on create and re-asserted by `doctor`.
- **Per-profile isolation.** Everything for an identity lives under
  `~/.ssh/profiles/<profile>/` - keys, config, and its **own** `known_hosts`. With
  `IdentitiesOnly yes` + per-profile `UserKnownHostsFile`, neither a key nor a
  trusted host key ever bleeds across identities (even on a shared host).
- **The encrypted bundle never contains `.env`** (it holds the age recipient
  refs that unlock the bundle).
- **Token files are owner-only and written atomically.** `auth.json`, `.npmrc`,
  Docker's `config.json`, `.git-credentials` and `.netrc` are written to a temp
  file at `0600` and renamed into place; `doctor` fails on a looser mode and
  `doctor --fix` tightens it.
- **The token config holds no secrets.** It names environment variables; the
  values are read at run time and written only to the tools' own files.
- **Secrets are not printed or recorded.** `token inventory` masks them unless
  `--reveal` is confirmed, `token clip` copies without printing, `token sync`
  does not echo `COMPOSER_AUTH`, and the audit log and history record the target
  and host, never the value. The web UI never receives or shows a token.
- **No credential injection.** `.npmrc`, `.netrc` and the git credential store
  have no quoting, so a newline or whitespace in a value could add a second
  credential. Such values, and malformed hosts, are refused before any write.
- **Docker credential stores are respected.** A registry Docker keeps in a
  credential helper is logged in with `docker login --password-stdin` (the
  token on stdin, never argv) or refused; a non-interactive run with no setting
  refuses.
- **Dev mode cannot reach real credentials.** Under `--dev-root` every token file
  maps into the sandbox, the tools' path variables are ignored, and `docker
  login` is refused.

## Supported versions

This project is pre-1.0; security fixes land on `main` and the latest release.
