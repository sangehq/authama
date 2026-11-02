# Release

Tag-driven, and on hold. A `vX.Y.Z` tag records a version; a binary release is
built from it only when dispatched by hand with a confirmation (below), which
builds a static binary per OS/arch and attaches them, with checksums, to a GitHub
Release. Until binary releases resume, users build from source
([installation](installation.md#install)).

## Two tags per release

Each release is tagged twice on the same commit, and they are not
interchangeable:

| Tag | Read by | Why |
|---|---|---|
| `vX.Y.Z` | GoReleaser, the installers, `git describe` | the release proper, and the only one a human writes |
| `src/vX.Y.Z` | the Go module proxy, and nothing else | Go resolves a subdirectory module through a tag prefixed with that subdirectory, so `go install .../src@vX.Y.Z` finds nothing without it |

They cannot be collapsed into one. OSS GoReleaser rejects `src/vX.Y.Z` outright
("current tag is not semver"), and `monorepo.tag_prefix`, which exists for this,
is a Pro feature. So the release runs off `vX.Y.Z` and the workflow mirrors
`src/vX.Y.Z` onto the same commit before publishing anything - you tag once.
`src/...` does not match the workflow's own `v[0-9]*` trigger, so pushing the
alias starts no second run.

## How a release is built

Each binary is pure Go with no runtime dependency of any kind - there is no
embedded interpreter and no `-tags bundled` build any more, so every target
cross-compiles from one Linux runner (`CGO_ENABLED=0`) instead of needing a
matching native one per OS. The binary version comes from the tag via ldflags
(`internal/version.Version`).

## Cut a release

1. Update `CHANGELOG.md` (move Unreleased into a dated, semver-tagged section).
2. Tag and push: `git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z`.
   Pushing a tag publishes nothing: binary releases are on hold, and authama is
   built from source until the owner decides otherwise.
3. Only when a binary release has been approved for this version, dispatch it
   on the tag: `gh workflow run release.yml --ref vX.Y.Z -f confirm=release-go`.
   The workflow refuses to run without that confirmation or on anything but a
   `v[0-9]*` tag. A tag with a pre-release suffix (`-rc.1`, `-beta`) is
   published as a GitHub pre-release, and the workflow adds the second tag
   itself - see below.
4. The `Release` workflow builds every target and publishes the per-OS/arch
   binaries and archives, the deb/rpm/apk packages, per-archive SBOMs,
   `checksums.txt`, and a signed build-provenance attestation. Verify any
   downloaded artifact with
   `gh attestation verify <file> --repo simtabi/authama`.

> There is no cosign signature. Provenance attestation is the org default and is
> what `release.yml` produces; a `signs:` block would be added against a named
> downstream need, not speculatively.

## First release (one-time setup)

1. Make the repo public and enable Issues.
2. Set the metadata:
   `gh repo edit simtabi/authama --homepage "https://github.com/simtabi/authama#readme"`,
   topics `oss ssh ssh-keys ssh-config key-rotation cli go`.
3. Create the `release` GitHub Environment (`release.yml` runs in it).
4. Green on macOS, Linux and Windows: `make check`, `make e2e`, `make feature-check`.
5. After the tag lands, confirm the published artifact installs:
   `curl -fsSL https://raw.githubusercontent.com/simtabi/authama/main/src/scripts/install.sh | bash -s vX.Y.Z && authama version`.

## Homebrew and Scoop

GoReleaser publishes the cask and the Scoop manifest with the write deploy keys
`HOMEBREW_TAP_DEPLOY_KEY` (on `simtabi/homebrew-tap`) and `SCOOP_BUCKET_DEPLOY_KEY`
(on `simtabi/scoop-bucket`), each a repository secret that reaches its one
repository and nothing else. `TAP_GITHUB_TOKEN` is read only for the Winget pull
request. Every manifest downloads from this repository's release assets, which
are named `authama_*` from 0.1.2 on (the bare `sshmgr_*` binaries are the
deprecated alias).

- **Homebrew:** the tap requires a pull request on `main`, with deploy keys as its
  one bypass actor, so the cask `Casks/authama.rb` is committed to `main` directly.
  A release without `HOMEBREW_TAP_DEPLOY_KEY` skips it. The old `sshmgr` cask is
  gone: the tap's `cask_renames.json` maps `sshmgr` to `authama`, so `brew upgrade`
  moves an existing install over by itself.
- **Scoop:** the bucket requires a pull request on `master` and has no bypass actor,
  so GoReleaser pushes each manifest to its own branch instead: `authama-X.Y.Z`
  for `bucket/authama.json`, and `sshmgr-X.Y.Z` for `bucket/sshmgr.json`, the
  deprecated manifest kept current so existing installs still update and are told
  to switch (Scoop has no rename). The
  bucket's own workflow, `.github/workflows/auto-merge-manifest.yml`, runs on every
  push to an `sshmgr-*` or `authama-*` branch: it opens the pull request with its
  `GITHUB_TOKEN`, waits for the checks, and merges it, deleting the branch. Nothing
  is left to do by hand, and `master` stays protected. It relies on "Allow GitHub
  Actions to create and approve pull requests", enabled for the simtabi
  organization and for the bucket. If a check fails, the pull request stays open
  for a person to look at; merge it with `gh pr merge <number> -R
  simtabi/scoop-bucket --merge --delete-branch` once it is fixed.

The release body and the provenance attestation run even if GoReleaser fails after
publishing, so a late publisher failure no longer leaves a release unattested.

## Winget

Winget is **not a published channel yet**. `src/.goreleaser.yaml` carries a manifest for
`Simtabi.authama`, but it only publishes when the repository variable `SKIP_WINDOWS_PKGS`
is `false`, and that variable is unset, so every release skips it. Turning it on needs
the `simtabi/winget-pkgs` fork of `microsoft/winget-pkgs` (it does not exist; checked
2026-11-02 with `gh api repos/simtabi/winget-pkgs`, which answers 404) and a
`TAP_GITHUB_TOKEN` able to open a pull request there. Until both exist, no doc lists
winget as an install method. Scoop does not depend on it: it has its own switch,
`SKIP_SCOOP`, which is `false`.

## GitHub Actions hygiene

Keep action versions current via Dependabot (weekly); re-pin on every merged
dependency PR.

---

[← Docs index](../README.md#documentation)
