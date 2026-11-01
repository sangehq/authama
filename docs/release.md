# Release

Tag-driven. A `v*` tag builds a static binary per OS/arch and attaches them,
with checksums, to a GitHub Release. Install the binary, or build from source
with `go install github.com/simtabi/authama/src/v3/cmd/sshmgr@latest`.

## Two tags per release

Each release is tagged twice on the same commit, and they are not
interchangeable:

| Tag | Read by | Why |
|---|---|---|
| `vX.Y.Z` | GoReleaser, the installers, `git describe` | the release proper, and the only one a human writes |
| `src/vX.Y.Z` | the Go module proxy, and nothing else | Go resolves a subdirectory module through a tag prefixed with that subdirectory, so `go install .../src/v3@vX.Y.Z` finds nothing without it |

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
   A tag with a pre-release suffix (`-rc.1`, `-beta`) is published as a GitHub
   pre-release. The workflow adds the second tag itself - see below.
3. The `Release` workflow builds every target and publishes the per-OS/arch
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
   `gh repo edit simtabi/authama --homepage "https://opensource.simtabi.com/products/simtabi/ssh-manager"`,
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
request. Both manifests download from this repository's release assets.

- **Homebrew:** the tap requires a pull request on `main`, with deploy keys as its
  one bypass actor, so the cask is committed to `main` directly. A release without
  `HOMEBREW_TAP_DEPLOY_KEY` skips it.
- **Scoop:** the bucket requires a pull request on `master` and has no bypass actor,
  so GoReleaser pushes the manifest to the branch `sshmgr-X.Y.Z` instead. After the
  release, open and merge that branch:

  ```sh
  gh pr create -R simtabi/scoop-bucket --base master --head sshmgr-X.Y.Z \
    --title "Scoop update for sshmgr version vX.Y.Z" --body "Release vX.Y.Z"
  gh pr merge <number> -R simtabi/scoop-bucket --merge --delete-branch
  ```

  GoReleaser cannot open this pull request itself: with a git deploy key it only
  pushes, and opening a pull request needs an API token for the bucket, which no
  secret provides.

The release body and the provenance attestation run even if GoReleaser fails after
publishing, so a late publisher failure no longer leaves a release unattested.

## GitHub Actions hygiene

Keep action versions current via Dependabot (weekly); re-pin on every merged
dependency PR.

For the org-wide OIDC reference across channels, see
`/opensource/package-publishing-guide.md`.

---

[← Docs index](../README.md#documentation)
