# `doctor` — check, and repair, the managed tree

Checks over the environment, the config home and `~/.ssh`, then the
auth-token files, with an exit code that is a verdict rather than a success flag.

```sh
authama doctor           # report
authama doctor --fix     # report, and repair what it can
authama doctor --json    # the same report, for a script
authama doctor --strict  # any warning fails the run (CI)
```

## What it checks

| Check | Reports |
|---|---|
| `os` / `runtime` | the platform, and that the binary needs no interpreter |
| `hard deps` | OpenSSH is present: `ssh`, `ssh-keygen`, `ssh-add`, `ssh-keyscan` |
| `optional` | which of `age`, `sops`, `gitleaks`, `gh`, `glab`, `age-plugin-yubikey` are missing |
| `home` / `state` / `ssh` | the resolved config home, the state home (when it is separate) and `~/.ssh`, so an override is visible |
| `providers` | whether the catalog is the shipped default or a user file |
| `agent` | whether `ssh-agent` is running and how many keys it holds |
| `1Password` | when a profile uses `key_store: 1password`: that `op` is on PATH, the agent socket exists, and the agent serves the profile's vault (`agent.toml`) |
| `proxy` | every `ProxyCommand` authama rendered names a binary that still exists: the authama binary for VM hosts (an upgrade that moved it fails the verdict, and `authama ssh config render` rewrites the path) and `cloudflared` for Cloudflare Access hosts (install it, then re-render) |
| `MISSING tool` | a command the manifest relies on is not on PATH: the hypervisor CLI behind a VM host (its address comes from it at connect time), and `git` once `defaults.git` is set (fails the verdict) |
| `clipboard` | no clipboard tool was found, so `termius keys` needs `--print` (advice; never fails the verdict) |
| `known_hosts` | whether the trust store exists, and which hosts are unpinned |
| `config drift` | whether the rendered block still matches the manifest |
| `perms` | every managed path whose mode is not what it should be |
| alias collisions | the same Host alias in more than one profile, so ssh applies only the first (warning) |
| shadowed hosts | a block above the managed block matches a managed alias, so the manifest's values do not apply (fails the verdict) |
| raw-option overlaps | a `raw_options` key that duplicates a field authama already writes for that host (informational) |
| dangling keys | declared-but-missing, half a pair, on disk but untracked, unwired |
| duplicate keys | the same key material under two profiles |
| stale predecessors | archived keys older than `AUTHAMA_OLD_KEY_MAX_AGE_DAYS` |
| auth tokens | after the SSH report, under `=== auth tokens ===`: each installed credential file (Composer's `auth.json`, `.npmrc`, Docker's `config.json`, `.git-credentials`, `.netrc`) and whether it is owner-only. A file others can read is a warning, and fails the verdict under `--strict`. The same check, plus `.gitignore` coverage for a project, is `authama token audit` |

## Exit codes

**0 when the tree is clean, 1 when it is not.** That is a verdict, not an error —
`doctor` succeeded either way. Everything else in the tool uses 1 for failure, so
a script that only wants to know "did it run" should check for output rather than
the code.

A token file with a loose mode is a **warning** by default, so a machine whose
`~/.npmrc` was group-readable under sshmgr 3.x still passes; the report says
`doctor --fix` (or a `chmod 600`) tightens it. With `--strict` it fails the run,
which is what CI wants. `token audit` always fails on it.

Without `--strict`, a key that is declared but has never been minted is a warning
and does not fail the run: it is an ordinary state on a machine that has not
reconciled yet. With `--strict` every warning - dangling keys, loose token files,
alias collisions, unpinned hosts and the rest - fails the run with a non-zero
exit, and the summary never says the tree is clean while warnings are listed.
That is what makes it usable as a CI gate.

## `--fix`

Repairs permissions, and only permissions. It re-asserts the canonical mode on
every managed path in `~/.ssh`, on the config-home secrets and on the installed
token files (0600) — the same
enumeration the check walks, so the two cannot disagree about what is managed.

It does not mint keys, render config or pin hosts. Those are `reconcile` and
`knownhosts pin`, and they are separate because they change more than metadata.

## `--json`

The scripting surface. Empty collections serialize as `[]` and `{}`, never
`null`, so a consumer never has to distinguish "none" from "absent":

```sh
authama doctor --json | jq -e '.perm_issues | length == 0'
```

Keys: `ok`, `home`, `ssh_dir`, `preflight_ok`, `providers_source`, `agent`,
`known_hosts`, `config_in_sync`, `perm_issues`, `orphan_keys`, `duplicate_keys`,
`unpinned_hosts`, `alias_collisions`, `stale_proxy`, `old_keys`, `stale_old_keys`,
`dangling_keys`, `stranded_legacy_home`, `shadowed_hosts`, `raw_option_overlaps`, `needed_tools`, `clipboard` (`null` when a
clipboard tool was found).

`tokens` is `{"ok": bool, "findings": [{"level", "target", "text"}]}`. Its `ok` is
separate from the top-level `ok`, which stays the SSH verdict; the exit code
fails if either is false:

```sh
authama doctor --json | jq -e '.ok and .tokens.ok'
```

Diagnostics go to stderr, so the document on stdout is always parseable.

---

[← Docs index](../../README.md#documentation)
