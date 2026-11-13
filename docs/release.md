# Release

Binaries ship on the GitHub release, and nowhere else. A pushed `vX.Y.Z` tag, or a
dispatch naming one, runs `release.yml`, which builds a static binary per OS/arch and
attaches them, with archives, Linux packages, checksums and a provenance attestation,
to that tag's GitHub Release. v0.1.6 is the first release since the history restarted;
users who prefer can still build from source ([installation](installation.md#install)).

```bash
git push origin v0.1.7                                            # builds and publishes v0.1.7
gh workflow run release.yml --repo sangehq/authama -f tag=v0.1.7   # the same, for a tag already pushed
```

A pushed tag publishes, and tags are final, so push one only for code that is ready.

## Version numbers since the restart

On 2026-11-13 the repository moved from `simtabi/authama` to `sangehq/authama`, and the Go
module path from `github.com/simtabi/authama/src` to `github.com/sangehq/authama/src`.
GitHub redirects the old repository URLs. The old module path's versions stay cached by
the Go module proxy under that path, and nothing points at them; no doc offers either
path through the Go toolchain. The new path has no published version, so `src/go.mod`
retracts nothing.

Under the old path: v0.1.1 to v0.1.3 were retracted; v0.1.4 and v0.1.5 are tags with no
release, both pushed on `3bc0a81` before the commit that added their CHANGELOG section
(runs 43447818596 and 43450588924 refused them); v0.1.6 is the last release, tagged on the
merge commit that added its section (`6c56209`). That order holds for every release:
merge the CHANGELOG section first, then tag that merge commit, never `main` before it
merges.

The versions restart at v0.1.0 under the new module path (owner, 2026-11-13), once the
old tags are cleared.

Under the old path, v0.1.0 was kept as a release, not retracted. The proxy's copy of it is the
first commit tagged `src/v0.1.0` (`f21ef51`), which is not the commit the tag
names today (`282d76c`), so a module fetch of v0.1.0 gets the older code. Building
from a checkout of the tag builds the newer one.

## Two tags per release

Each release is tagged twice on the same commit, and they are not
interchangeable:

| Tag | Read by | Why |
|---|---|---|
| `vX.Y.Z` | GoReleaser, the installers, `git describe` | the release proper, and the only one a human writes |
| `src/vX.Y.Z` | the Go toolchain, and nothing else | Go resolves a subdirectory module through a tag prefixed with that subdirectory, so another module importing `.../src` at vX.Y.Z finds nothing without it |

They cannot be collapsed into one. OSS GoReleaser rejects `src/vX.Y.Z` outright
("current tag is not semver"), and `monorepo.tag_prefix`, which exists for this,
is a Pro feature. So the release runs off `vX.Y.Z` and the shared workflow mirrors
`src/vX.Y.Z` onto the same commit before GoReleaser runs - you tag once.
`src/...` does not match the `v[0-9]+.[0-9]+.[0-9]+` trigger, so pushing the
alias starts no second run.

## How a release is built

`release.yml` holds no pipeline of its own. Its one job calls simtabi/.github's
reusable `go-release.yml`, pinned to a commit, with the tag. That workflow, shared by
every Go repository in the org:

1. refuses anything that is not a `vX.Y.Z` or `vX.Y.Z-pre` tag already on GitHub, and
   checks out `refs/tags/<tag>`;
2. refuses a GoReleaser config holding any block that publishes elsewhere (a tap, a
   bucket, Winget, an image, an upload);
3. takes the release notes from the tag's `## [X.Y.Z]` CHANGELOG section, and fails
   when a final tag has none;
4. mirrors `src/vX.Y.Z`, runs GoReleaser with `src/.goreleaser.yaml`, and reads the
   published body back, failing if it is shorter than the section;
5. signs a build-provenance attestation for everything in `checksums.txt`.

Each binary is pure Go with no runtime dependency of any kind - there is no
embedded interpreter and no `-tags bundled` build any more, so every target
cross-compiles from one Linux runner (`CGO_ENABLED=0`) instead of needing a
matching native one per OS. The binary version comes from the tag via ldflags
(`internal/version.Version`).

Change the pipeline in simtabi/.github, then move the pin in `release.yml`. Three
tests here hold the rest: `release.yml` triggers on exactly a version-tag push and a
dispatch with a required `tag`, and calls the shared workflow
(`TestReleaseRunsOnAVersionTagOrADispatch`); `src/.goreleaser.yaml` holds no
publishing block (`TestGoReleaserPublishesOnlyToTheGitHubRelease`); and no doc offers
`go install` or `go get` of the module (`TestNoDocOffersGoInstall`).

## Cut a release

1. Update `CHANGELOG.md` (move Unreleased into a dated, semver-tagged section).
2. Tag and push: `git tag -a v0.1.7 -m v0.1.7 && git push origin v0.1.7`. The push
   starts the release. A tag with a pre-release suffix (`-rc.1`) is published as a
   GitHub pre-release and needs no CHANGELOG section; test with one before a final tag.
3. To build a tag that is already pushed (a first release of an older tag, or a
   re-run after a failure): `gh workflow run release.yml --repo sangehq/authama -f tag=v0.1.7`.
4. The run publishes the per-OS/arch binaries and archives, the deb/rpm/apk packages,
   per-archive SBOMs, the source tarball, the two install scripts, `checksums.txt`, and
   a signed build-provenance attestation. Verify any downloaded artifact with
   `gh attestation verify <file> --repo sangehq/authama --signer-workflow simtabi/.github/.github/workflows/go-release.yml`.
   The shared workflow is what signs, so its path is the signer; without the flag `gh`
   expects a workflow in this repository and the check fails.

> There is no cosign signature. Provenance attestation is the org default and is
> what the shared workflow produces; a `signs:` block would be added against a named
> downstream need, not speculatively.

## First release (one-time setup)

1. Make the repo public and enable Issues.
2. Set the metadata:
   `gh repo edit sangehq/authama --homepage "https://github.com/sangehq/authama#readme"`,
   topics `oss ssh ssh-keys ssh-config key-rotation cli go`.
3. The `release` GitHub Environment the old pipeline ran in is no longer used: a
   reusable workflow's job cannot name one from the caller.
4. Green on macOS, Linux and Windows: `make check`, `make e2e`, `make feature-check`.
5. After the release publishes, download the binary for one platform from the release
   page, check it against `checksums.txt`, and run `authama version`.

## Frozen channels

Up to v0.1.3 GoReleaser also published a Homebrew cask, a Scoop manifest and,
switched off, a Winget manifest. Those blocks were removed from `src/.goreleaser.yaml`
on 2026-11-13: binaries ship on the GitHub release only. The repositories they wrote to
are frozen, not deleted, and nothing writes to them again:

- **Homebrew:** `simtabi/homebrew-tap` carries no authama cask. The cask and the
  `sshmgr` → `authama` rename were withdrawn (simtabi/homebrew-tap d9969e2) because they
  named v0.1.3 downloads that no longer exist.
- **Scoop:** `simtabi/scoop-bucket` carries neither `bucket/authama.json` nor
  `bucket/sshmgr.json`; both were withdrawn with the cask (simtabi/scoop-bucket f253f9c).
  Its `auto-merge-manifest.yml` workflow, which merged a manifest pushed to an
  `authama-*` branch, has nothing left to merge.
- **Winget:** never published. The `simtabi/winget-pkgs` fork it needed does not exist
  (checked 2026-11-02 with `gh api repos/simtabi/winget-pkgs`, which answers 404).

The repository secrets the old pipeline read, `HOMEBREW_TAP_DEPLOY_KEY`,
`SCOOP_BUCKET_DEPLOY_KEY` and `TAP_GITHUB_TOKEN`, and the variables `SKIP_WINDOWS_PKGS`
and `SKIP_SCOOP`, are read by nothing now.

## GitHub Actions hygiene

Keep action versions current via Dependabot (weekly); re-pin on every merged
dependency PR.

---

[← Docs index](../README.md#documentation)
