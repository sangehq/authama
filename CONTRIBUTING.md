# Contributing to Authentication Access Manager (authama)

Where this file is silent, the [Simtabi contributing guide](https://github.com/simtabi/.github/blob/HEAD/CONTRIBUTING.md) applies.

Thanks for your interest! Authentication Access Manager (`authama`) is a Simtabi LLC open-source project.

## Ground rules (the invariants)

A small set of invariants must never be weakened for convenience:

1. **The manifest is the source of truth**; `~/.ssh` is generated output.
2. **The tool owns the SSH config** - never hand-edit rendered files; change the
   manifest (or `host`/`profile` verbs) and re-render. Unmodeled options go
   through a host `raw_options` passthrough.
3. **One renderer** drives `config render`, `config check`, and `reconcile`.
4. **Secrets never touch git**; **atomic writes** under an **advisory lock** for
   all state; **perms are load-bearing** (700/600/644).

If an invariant seems to block you, raise it in an issue rather than routing
around it.

## Dev setup

```sh
git clone https://github.com/sangehq/authama && cd authama
make build               # -> build/<your-os>/authama, e.g. build/macos/authama, replaced in one step
make link                # build, then link ~/.local/bin/authama to it: PATH follows every build
make install             # or: build, then copy it to ~/.local/bin (BINDIR=... for elsewhere)
make uninstall           # remove that binary or link again; config, state and keys stay
make clean               # empty build/ (keeping targets.txt); build no longer does this first
make doctor              # verify the environment, using the binary you just built
```

There is no bootstrap step: Go 1.27 is the only prerequisite. authama's config
home is `~/.config/authama` and its state home `~/.local/state/authama`; set
`AUTHAMA_CONFIG_DIR="$PWD/.devhome"` to sandbox a throwaway one (state included)
while developing, or use `--dev-root` to sandbox `~/.ssh` as well.

## Before you push

```sh
make gates               # ci and lint-all, then e2e and e2e-web, in one go; stops at the first failure
make check               # the CI gate plus lint: gofmt, build, vet, tests, golangci-lint
make ci-linux            # the same gate inside a linux container (needs docker)
make lint-all            # lint every GOOS - one run only ever sees one
make e2e                 # end-to-end smoke against a real binary (build-tagged)
make e2e-web             # the web UI in a browser: Playwright e2e and axe (needs node, age)
make feature-check       # the per-command assertions
```

`make ci` is the gate CI itself runs, and `check` is a strict superset of it, so
the two cannot drift. `make ci-linux` runs that gate on Linux, on the Go version
`go.mod` asks for — `check` only ever tests the machine you are on, and
`lint-all` cross-compiles for other systems without running anything on them, so
a Linux-only failure is otherwise invisible until CI reports it. That matters
when CI cannot: a runner outage, a fork without Actions, or no network. Run `lint-all` when touching `internal/platform` or any
build-tagged file: a single golangci-lint run analyses one GOOS and reports
everything the others reference as dead code.

### The web UI in a browser

`make e2e-web` builds the binary and runs `e2e/web/`, Playwright tests for
`authama serve`. The global setup (`e2e/web/support/sandbox.ts`) makes a throwaway
home: a temp `HOME`, `XDG_CONFIG_HOME` and `XDG_STATE_HOME`, every `AUTHAMA_*`
variable dropped, the shipped example manifest reconciled into it, and one
encrypted bundle. It refuses to go on if the resolved `~/.ssh` is outside that
directory, so your own keys are never in reach. Then it starts `authama serve
--port 0` and signs each test in with the printed token.

- `tests/pages.spec.ts` walks every page and checks what it shows against the
  manifest: the profiles, hosts, keys and fingerprints, the doctor report, the
  history, and the 401 without the token.
- `tests/flows.spec.ts` previews a change and makes it, has a stale preview
  refused, and runs a bundle restore as a job, reading its event stream to the end.
- `tests/a11y.spec.ts` runs axe over the same pages in the light and the dark
  theme, and fails on a serious or critical violation.

It needs Node, `ssh-keygen` and `age`. The Node dependencies take ranges and no
lock file is kept: `npm install --no-package-lock` writes none, and
`e2e/web/.gitignore` refuses one. CI runs it on Linux only, in the `web-e2e` job.
Binding a local port is the one thing it cannot do without, so a sandbox that
refuses one can run only the typecheck (`cd e2e/web && npx tsc --noEmit`).
Pass Playwright flags through `E2E_WEB_ARGS`, e.g.
`make e2e-web E2E_WEB_ARGS='--grep doctor --headed'`, and set
`AUTHAMA_E2E_KEEP=1` to keep the sandbox home for a look afterwards.

## Invariants

These are enforced by tests, not by convention. Breaking one turns a test red.

- **Identity is `profile/key`, never a bare key name.** Two profiles may declare
  the same file name — one person under two organisations does exactly that — so
  anything keyed on the name alone will act on the wrong key.
- **A command that edits the manifest re-renders the config; one that only
  touches key files does not.** Leaving the render to a later `reconcile` means a
  successful edit ends with the tool reporting drift it just created.
- **An inventory record survives while any `KeyRef` owns it.** Deriving the
  surviving set from hosts alone drops the record of a declared key that has
  merely become unwired, and expiry then cannot see it.
- **`keysvc` reports, `keyaudit` interprets.** A new dangling state belongs in
  `keyaudit`; a new fact about a key belongs in `keysvc`.
- **No business logic in `internal/cli`** — verbs parse flags and call a service.
  `internal/cli` is a leaf: nothing under `internal/` may import it.
- **State I/O goes through `internal/util/fs` under `internal/util/lock`**, and
  every external command is an argv slice, never a shell string. The tool's whole
  external-binary surface is pinned by a test.

## Two roots

The Go module is `src/`, not the repository root. `go.mod` is there, so every
`go` command has to run there — the Makefile does this for you with `go -C src`,
and `make` is the supported way to build. Running `go test ./...` from the
repository root finds no packages and passes without testing anything.

Everything the repository *presents* stays at the top: `README.md`, `LICENSE`,
`CHANGELOG.md`, `docs/`, `.github/`, `Makefile`, and `build/`.

| Where | What |
|---|---|
| `src/` | `go.mod`, `cmd/`, `internal/`, `config/`, `scripts/`, `.goreleaser.yaml`, `.golangci.yml` |
| repo root | the README and friends, `docs/`, `.github/`, `Makefile`, `build/` |

Import paths start with the module path, `github.com/sangehq/authama/src/internal/cli`;
versions restarted at v0.1.0, so there is no major-version suffix. Package names in
this document are written relative to the module, so `internal/cli` means
`src/internal/cli` on disk.

Two things deliberately cross the boundary, and both say so where they occur:
GoReleaser runs from the repository root with `builds[].dir: src`, because its
archive globbing cannot climb out of its working directory to reach `LICENSE`;
and `scripts/build-all.sh` runs in the module — `go build` needs that — while
writing into the root `build/`.

## Build output

Everything generated goes to `build/` at the repository root — one directory,
not one per module — and nothing goes anywhere else:

| Path | What | Tracked |
|---|---|---|
| `build/targets.txt` | the release target matrix | yes |
| `build/{os}/` | binaries, one folder per OS (`macos`, `linux`, `windows`, `freebsd`): `make build` writes the host's `build/<os>/authama`, `make cross` every target as `authama_{os}_{arch}[.exe]` | no |
| `build/checksums.txt` | SHA-256 over every binary, as `{os}/{file}`, so `cd build && shasum -a 256 -c checksums.txt` verifies as-is | no |
| `build/archives/` | `make dist`: a `.tar.gz` or `.zip` per target (the Windows zips add `Start authama.cmd`), `authama_macos_app.zip` with `Authama.app`, and `make man`'s pages | no |

Nothing else goes in `build/`: no `build/dist/`, no per-target subfolders. GoReleaser, the release
path, works in `.goreleaser-dist/` at the root instead (`make release-snapshot` runs it locally and
publishes nothing).

`make clean` empties it, keeping `targets.txt`. There was a `bin/` alongside it
until this layout was settled — two directories for one idea — and `make clean`
removed a top-level `dist/` that GoReleaser had stopped writing to, so it left
every release artifact in place while reporting success. A `build/` appearing
anywhere else (`src/build/`, say, from a tool run in the wrong directory) is a
mistake, and `.gitignore` refuses to track one.

## Where things live

- `internal/services/lifecycle` owns all three guarded deletions (profile, host,
  key); `internal/services/editor` owns the manifest edits underneath them.
- Releases carry two tags on one commit: `vX.Y.Z` drives the release, and
  `src/vX.Y.Z` is how Go resolves a module that lives in a subdirectory. The
  workflow creates the second one; see [docs/release.md](docs/release.md).
- `internal/services/keyaudit` owns every dangling-key state and the text that
  explains it.
- `src/config/manifest.json` is the shipped example *and* the renderer's golden
  fixture *and* the fixture the e2e test copies in. Changing its shape means
  updating the renderer goldens in the same commit.
- **Idempotency tests** for every converging command (run twice → no diff, no
  clobbered keys). **Security tests** for perms and secret exclusion.
- `gitleaks` runs in CI over the full history — never commit a secret. A finding
  on a published commit cannot be fixed by editing the working tree; retire it by
  fingerprint in `.gitleaksignore`, with a note saying what the string is and why
  it is not a secret.

## The frozen command surface

`internal/cli/surface_test.go` holds `v1Surface`: the verb and flag list this
tool shipped before the Go rewrite, transcribed into the test.
`TestTheCommandSurfaceMatchesTheOneItReplaced` compares the whole cobra tree
against it in both directions, so a verb cannot vanish and a flag cannot appear
without an entry in `goOnly` saying why.

It is a frozen baseline, not a pointer at anything — the data is in the file, and
the test needs nothing else to run. Adding a verb means adding a `goOnly` entry;
removing one that the baseline lists means the test fails, which is the point.

## Commits & PRs

- Author commits with your GitHub noreply address
  (`git config user.email "<id>+<username>@users.noreply.github.com"`) so no
  personal address lands in the history.
- Subject ≤ 72 chars, imperative mood; body explains *why*. No emoji.
- Keep PRs small, green, and scoped to one concern. Update `CHANGELOG.md` and
  `README.md` as verbs land.
- If CI does not start on a push, look at the PR's merge state first: GitHub
  starts no `pull_request` run while the PR conflicts with `main`. Resolve the
  conflict rather than dispatching CI by hand, which tests the branch and not
  the merge.
- Pull requests test on Linux and Windows, and run `darwin cross-check`, which
  builds and vets every package for macOS (arm64 and amd64) on Linux. The full
  macOS test runs on the weekly schedule and on `workflow_dispatch`
  (`gh workflow run ci.yml --repo sangehq/authama --ref <branch>`), because
  GitHub's macOS runners went unscheduled for days and a required macOS leg
  blocked every merge. A `test (macos-latest)` leg that fails after about 15
  minutes with no step run never got a runner; `requeue.yml` re-runs it once.
