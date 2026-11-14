# authama (Authentication Access Manager) - dev front door. Run from the repo root:  make <target>
#
# Pure Go: no venv, no interpreter, no bootstrap step. `make build` is enough to
# get a working binary, and every other target works off the toolchain already
# needed to compile it.
#
# Two roots, and the distinction is load-bearing. The Go module is src/ - that is
# where go.mod lives and where every `go` invocation has to run, which is what
# `go -C` below is for. Build output is a property of the repository, not of the
# module, so it lands in build/ at the root: that is where you look for the
# binary, and it stays one directory rather than one per module.
#
# Inside build/ there is one folder per OS and nothing else (the standard every
# Simtabi Go project follows): `make build` writes build/{host-os}/authama,
# `make cross` and `make dist` write build/{os}/authama_{os}_{arch} and
# build/archives/. The OS folder is macos, not darwin.
MODULE    := src
HOST_GOOS := $(shell go env GOOS 2>/dev/null)
HOST_OS   := $(if $(filter darwin,$(HOST_GOOS)),macos,$(HOST_GOOS))
BIN       := build/$(HOST_OS)/authama
PKG       := ./cmd/authama

# Our own modules come from git, never proxy.golang.org or sum.golang.org, so a
# local build cannot feed a version into the public cache. Overridable.
export GOPRIVATE ?= github.com/sangehq/*,github.com/simtabi/*
export GONOSUMDB ?= github.com/sangehq/*,github.com/simtabi/*

# A release carries two tags on the same commit, and this matches the first:
#
#   v0.1.0      the release. GoReleaser, the installers and `git describe` use
#               it, and it is what a human reads.
#   src/v0.1.0  the module alias. Go requires a subdirectory module's tags to be
#               prefixed with the subdirectory, so `go install .../src@v0.1.0`
#               resolves through this one and nothing else reads it.
#
# The split exists because OSS GoReleaser rejects a prefixed tag outright
# ("current tag is not semver"), and monorepo.tag_prefix is a Pro feature. The
# release workflow creates the alias so it cannot be forgotten.
#
# --match 'v[0-9]*' so no non-release tag can become the version. 'v*' was not
# enough: it excludes v1-final but admits anything else beginning with v,
# and a tag like v2-migration-record would then be stamped into the binary.
VERSION := $(shell git describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/sangehq/authama/src/internal/version.Version=$(VERSION)
GO      := go -C $(MODULE)

.PHONY: help build build-all test vet fmt fmt-check lint lint-tool lint-all ci check gates ci-linux e2e e2e-web feature-check docker-shell docker-run docker-down \
        cross dist release-snapshot clean install link uninstall doctor reconcile render rotate bundle settings generate tabler-check tabler-update tabler-vendor man

help:  ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sed -E 's/:.*## /\t/' | sort

# The binary on disk is always one a build that succeeded wrote. A stale binary
# is not a hypothetical here: a bug was reported against this tool from a binary
# built one minute before the fix landed, and it looked exactly like the fix not
# working. That once meant emptying build/ first; it now means compiling to
# authama.tmp beside the binary and renaming it over the old one only when the
# compile succeeds, so build/<host-os>/authama is never missing while a build
# runs. `make link` points ~/.local/bin/authama there and web.auto_restart
# watches it, so emptying build/ first left authama off PATH for the whole
# compile. A failed build exits non-zero and leaves the previous binary in
# place. build no longer runs clean; `make clean` empties build/ when asked. The
# rename also gives the new binary a fresh inode, which macOS needs to accept a
# re-signed executable.
#
# What it does NOT do is discard Go's build cache. That cache is keyed by a hash
# of the actual inputs, so a cached object is only reused when recompiling would
# produce the same bytes - "rebuild everything" buys no correctness there, it
# just adds minutes. FORCE=1 passes -a for the rare case of suspecting the
# toolchain itself.
#
# -o is resolved relative to $(MODULE) because of `go -C`, hence the ../.
build: ## compile the binary into build/{host-os}/, replacing it in one step (FORCE=1 to rebuild deps too)
	@mkdir -p build/$(HOST_OS)
	$(GO) build $(if $(FORCE),-a,) -trimpath -ldflags '$(LDFLAGS)' -o ../$(BIN).tmp$(EXE) $(PKG)
	mv -f $(BIN).tmp$(EXE) $(BIN)$(EXE)
	@echo "built $(BIN)$(EXE)  ->  $$(./$(BIN)$(EXE) version)"
ifeq ($(HOST_OS),macos)
	@$(MODULE)/scripts/touchid-helper.sh build/macos/authama-touchid.tmp
	@if [ -f build/macos/authama-touchid.tmp ]; then mv -f build/macos/authama-touchid.tmp build/macos/authama-touchid; fi
endif

build-all: ## compile every package (what CI gated on before `ci` existed)
	$(GO) build ./...

test: ## run the unit suite (TESTFLAGS=... to pass more, e.g. -run or -count=1)
	$(GO) test $(TESTFLAGS) ./...

vet: ## go vet
	$(GO) vet ./...

fmt: ## gofmt every file in place
	gofmt -w $(MODULE)/cmd $(MODULE)/internal

fmt-check: ## fail if anything is unformatted (what CI gates on)
	@test -z "$$(gofmt -l $(MODULE)/cmd $(MODULE)/internal)" || { gofmt -l $(MODULE)/cmd $(MODULE)/internal; exit 1; }

# golangci-lint from PATH, else where `go install` puts it ($GOPATH/bin), which
# is often not on PATH: a shell without it failed `make gates` with "command not
# found" while the linter sat installed in ~/go/bin. GOLANGCI_LINT=... overrides.
GOLANGCI_LINT ?= $(or $(shell command -v golangci-lint 2>/dev/null),$(firstword $(subst :, ,$(shell go env GOPATH 2>/dev/null)))/bin/golangci-lint)

lint-tool:
	@test -x "$(GOLANGCI_LINT)" || { \
		echo "golangci-lint is not installed (looked on PATH and in \$$GOPATH/bin). Install it with:"; \
		echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
		exit 1; }

lint: lint-tool ## golangci-lint (install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)
	cd $(MODULE) && "$(GOLANGCI_LINT)" run ./...

lint-all: lint-tool ## lint every GOOS - one run only sees one, so build-tagged files hide
	@for os in darwin linux windows; do \
		echo "--- GOOS=$$os ---"; \
		( cd $(MODULE) && GOOS=$$os "$(GOLANGCI_LINT)" run ./... ) || exit 1; \
	done

# `ci` is the gate the workflow runs, so there is one definition of it rather
# than a copy in ci.yml that drifts from this one. Lint is deliberately NOT in
# it: CI gets golangci-lint from a pinned action that also annotates the diff,
# and folding it in here would silently drop both the pin and the cache.
ci: fmt-check build-all vet test ## the gate CI runs (lint runs there as its own step)

# The gate runs the tests; it never replays a cached result. Caching would also
# cost more than it saves here: before storing a result, go test hashes every
# file and directory the test binary touched, and the cli tests touch thousands
# (each spawned tool is a PATH x PATHEXT search on Windows). On windows-latest
# that bookkeeping alone took ~6.5 minutes after internal/cli had already passed,
# which is what pushed the job to its 15-minute timeout. -count=1 skips it.
ci: TESTFLAGS += -count=1

check: ci lint ## everything: the CI gate plus lint, for humans

# One command for every local gate a change passes before it is pushed: the CI
# gate, lint for every GOOS, the end-to-end smoke and the browser suite. Run in
# that order, cheapest first, and it stops at the first that fails.
gates: ci lint-all e2e e2e-web ## every local gate before a push: ci, lint-all, e2e, e2e-web (needs node, age)

# The ubuntu-latest leg, runnable without GitHub. `make ci` only ever tests the
# machine you are on, and `lint-all` cross-compiles for other systems without
# running anything on them - so a Linux-only failure is invisible here until CI
# says so. When CI cannot say so (a runner outage, a fork without Actions, a
# flight), this is the same gate on the same Go version the module asks for.
#
# It reads the version from go.mod rather than pinning one, so it cannot drift
# from what setup-go installs in the workflow.
GOVERSION = $(shell awk '/^go /{print $$2}' $(MODULE)/go.mod)

ci-linux: ## run the CI gate inside a linux container (needs docker)
	@command -v docker >/dev/null 2>&1 || { echo "docker is not installed"; exit 1; }
	docker run --rm -v "$(CURDIR)":/w -w /w golang:$(GOVERSION) \
		sh -c 'apt-get -qq update >/dev/null && apt-get -qq install -y openssh-client >/dev/null && \
		       go -C $(MODULE) build ./... && go -C $(MODULE) vet ./... && go -C $(MODULE) test ./...'

# The dev container (Dockerfile + compose.yaml). Its home is ./.authama-home,
# not yours, unless AUTHAMA_DEV_HOME says otherwise.
COMPOSE := docker compose

docker-shell: ## build + start the dev container and open a shell in it
	@command -v docker >/dev/null 2>&1 || { echo "docker is not installed"; exit 1; }
	@mkdir -p $${AUTHAMA_DEV_HOME:-.authama-home}
	$(COMPOSE) up -d --build
	$(COMPOSE) exec authama bash

docker-run: ## run one command in the dev container: make docker-run ARGS="token list"
	@mkdir -p $${AUTHAMA_DEV_HOME:-.authama-home}
	$(COMPOSE) up -d --build
	$(COMPOSE) exec authama authama $(ARGS)

docker-down: ## stop and remove the dev container
	$(COMPOSE) down

# Tagged out of the ordinary suite: it mints six real keypairs and does an age
# round trip. It builds its own binary, so it does not depend on `build`.
#
# The timeout covers a cold cross-compile of every declared release target
# (TestEveryDeclaredReleaseTargetCompiles). In parallel that is still 3-4 minutes
# on a 3-core hosted runner with an empty build cache (macos-latest: 3m46s), so
# 300s left about a minute of headroom; 600s is a hang detector, not a budget.
# The CI job keeps its own 15-minute limit.
e2e: ## end-to-end smoke in a throwaway sandbox
	$(GO) test -tags e2e -count=1 -timeout 600s ./cmd/authama/

# The web UI in a real browser: Playwright drives every page and the write flows
# of `authama serve`, and axe checks each page in both themes. It builds the
# binary first, seeds a throwaway HOME (see e2e/web/support/sandbox.ts) and needs
# Node, ssh-keygen and age. --no-package-lock: no lock file is written or kept.
# Binding a local port is the one thing it cannot do without, so in a sandbox
# that refuses one it runs only in CI. E2E_WEB_ARGS passes more to Playwright.
e2e-web: build ## browser e2e + axe for `authama serve` (needs node, age)
	cd e2e/web && npm install --no-audit --no-fund --no-package-lock
	cd e2e/web && npx playwright install chromium
	cd e2e/web && npx tsc --noEmit
	cd e2e/web && npx tsc -p tsconfig.ui.json
	cd e2e/web && npx playwright test $(E2E_WEB_ARGS)

# The per-command assertions live in internal/cli/commands_test.go now, so they
# run in the ordinary suite. Kept as a target because the docs and the shipping
# checklist name it.
#
# It runs the whole package rather than a -run regex. The regex was
# 'TestCommandSurface|TestVerbs', which selected 9 tests and silently excluded
# TestTheCommandSurfaceMatchesTheOneItReplaced - the one test that pins the
# command surface against the implementation it replaced, and the thing a target
# called "feature-check" most obviously promises. A pattern that has to be kept
# in step with test names drifts the moment one is renamed, and says nothing when
# it does.
generate: ## regenerate docs/reference/cli.md and docs/schema/authama.schema.json from the code
	UPDATE_CLI_REFERENCE=1 $(GO) test ./internal/cli/ -run TestCLIReferenceIsCurrent
	UPDATE_SETTINGS_SCHEMA=1 $(GO) test ./internal/services/settings/ -run TestSchemaIsCurrent

feature-check: ## exercise every command with assertions
	$(GO) test -count=1 ./internal/cli/

# GoReleaser is configured in src/.goreleaser.yaml but runs from the repo root:
# its archives bundle LICENSE/README/CHANGELOG, and GoReleaser's file globbing
# will not climb out of its working directory to reach them. builds[].dir points
# the compile step back into the module. CI passes the same --config.
GORELEASER := goreleaser --config $(MODULE)/.goreleaser.yaml

cross: ## build every release target into build/{os}/, with build/checksums.txt
	ARCHIVES=0 ./$(MODULE)/scripts/build-all.sh

dist: ## cross, plus build/archives/: a tar.gz or zip per target, Authama.app for macOS, checksums
	./$(MODULE)/scripts/build-all.sh

# GoReleaser is the release path (release.yml, through simtabi/.github go-release.yml). Its output is an
# intermediate and goes to .goreleaser-dist/ at the root, never into build/.
release-snapshot: ## check the GoReleaser config locally, into .goreleaser-dist/ (needs goreleaser; publishes nothing)
	$(GORELEASER) release --clean --snapshot

# The web UI's vendored Tabler files. Each target downloads the npm tarballs,
# checks them against the registry's integrity hash before extracting, and
# rewrites the files, the licences, MANIFEST.sha256 and third-party-notices.md
# together, so a version never moves without its notices and hashes.
tabler-check: ## say whether a newer Tabler or Tabler Icons release is out (writes nothing)
	go -C $(MODULE) run ./internal/tools/tablervendor -check -root ..

tabler-update: ## move the web UI to the newest Tabler + Tabler Icons; re-vendor, notices, hashes
	go -C $(MODULE) run ./internal/tools/tablervendor -latest -root ..

MAN_DIR ?= $(or $(TMPDIR),/tmp)/authama-man

man: build ## write a manual page per command to MAN_DIR, and pack them into build/archives/authama_man.tar.gz
	./$(BIN)$(EXE) man "$(MAN_DIR)/man1"
	@mkdir -p build/archives
	tar -czf build/archives/authama_man.tar.gz -C "$(MAN_DIR)" man1
	@echo "read them with: man -M $(MAN_DIR) authama"

tabler-vendor: ## re-vendor exactly what src/internal/web/assets/vendor/TABLER_VERSION pins
	go -C $(MODULE) run ./internal/tools/tablervendor -root ..

# Everything generated lives under build/, so this empties it. It used to remove
# `bin dist`, and GoReleaser has written to build/dist since the single-folder
# layout was adopted - so `make clean` left every release artifact behind while
# reporting success. Then it named $(BIN) and build/dist explicitly, which left
# anything else that had found its way in - a renamed artifact, a file from an
# older layout, output from a tool run by hand.
#
# So it removes everything except what git tracks. KEEP is that list, and
# TestBuildDirKeepListMatchesGit fails if the two ever disagree.
KEEP := targets.txt

clean: ## empty build/, keeping only the files git tracks there
	@if [ -d build ]; then \
		find build -mindepth 1 -maxdepth 1 $(foreach k,$(KEEP),! -name '$(k)') -exec rm -rf {} + ; \
	fi

# --- installing the binary you just built ------------------------------------
# install copies build/<host-os>/authama to BINDIR, ~/.local/bin unless told otherwise
# (%LOCALAPPDATA%\Programs\authama on Windows, where install.ps1 puts it too).
# It builds first, so what lands on PATH is this checkout as it is now.
# uninstall removes that one file, and only after the file answers `version` as
# authama: a BINDIR typed wrong cannot delete something else. Neither touches the
# config home, the state home, ~/.ssh or a token file; uninstall says where they
# are so a full removal is a decision, not a side effect.
ifeq ($(OS),Windows_NT)
EXE    := .exe
BINDIR ?= $(LOCALAPPDATA)/Programs/authama
else
EXE    :=
BINDIR ?= $(HOME)/.local/bin
endif
INSTALLED := $(BINDIR)/authama$(EXE)

install: build ## copy build/<host-os>/authama to BINDIR (default ~/.local/bin) and say whether it is on PATH
	@mkdir -p "$(BINDIR)"
	@cp "$(BIN)$(EXE)" "$(INSTALLED).tmp" && chmod 0755 "$(INSTALLED).tmp" && mv -f "$(INSTALLED).tmp" "$(INSTALLED)"
	@echo "installed $(INSTALLED)  ->  $$("$(INSTALLED)" version | head -1)"
	@if [ "$(HOST_OS)" = macos ] && [ -f build/macos/authama-touchid ]; then \
		cp build/macos/authama-touchid "$(BINDIR)/authama-touchid.tmp" && chmod 0755 "$(BINDIR)/authama-touchid.tmp" && \
		mv -f "$(BINDIR)/authama-touchid.tmp" "$(BINDIR)/authama-touchid" && echo "installed $(BINDIR)/authama-touchid (Touch ID)"; fi
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "note: $(BINDIR) is not on PATH; add it to your shell profile";; esac
	@found="$$(command -v authama 2>/dev/null)"; if [ -n "$$found" ] && [ "$$found" != "$(INSTALLED)" ]; then \
		echo "note: 'authama' on PATH resolves to $$found, not $(INSTALLED)"; fi

# link points BINDIR/authama at this checkout's build/<host-os>/authama instead
# of copying it, so every `make build` is what PATH runs, and a running
# `authama serve` restarts into it (web.auto_restart). It replaces whatever is
# at BINDIR/authama, an installed copy included, and says so. Windows has no
# symlinks a user can make by default: use `make install` there.
ifeq ($(OS),Windows_NT)
link:
	@echo "make link needs symlinks; on Windows use: make install" >&2; exit 1
else
link: build ## point BINDIR/authama (default ~/.local/bin) at this checkout's build, so PATH follows every build
	@mkdir -p "$(BINDIR)"
	@if [ -e "$(INSTALLED)" ] && [ ! -L "$(INSTALLED)" ]; then echo "replacing the copy at $(INSTALLED) with a link"; fi
	ln -sfn "$(CURDIR)/$(BIN)" "$(INSTALLED)"
	@echo "linked $(INSTALLED) -> $(CURDIR)/$(BIN)  ->  $$("$(INSTALLED)" version | head -1)"
	@if [ "$(HOST_OS)" = macos ] && [ -f build/macos/authama-touchid ]; then \
		ln -sfn "$(CURDIR)/build/macos/authama-touchid" "$(BINDIR)/authama-touchid" && \
		echo "linked $(BINDIR)/authama-touchid (Touch ID)"; fi
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "note: $(BINDIR) is not on PATH; add it to your shell profile";; esac
	@found="$$(command -v authama 2>/dev/null)"; if [ -n "$$found" ] && [ "$$found" != "$(INSTALLED)" ]; then \
		echo "note: 'authama' on PATH resolves to $$found, not $(INSTALLED)"; fi
endif

uninstall: ## remove the authama binary, or this checkout's link, from BINDIR (default ~/.local/bin); config, state and keys stay
	@if [ -L "$(INSTALLED)" ] && [ "$$(readlink "$(INSTALLED)")" = "$(CURDIR)/$(BIN)" ]; then \
		unlink "$(INSTALLED)" && echo "removed the link $(INSTALLED)"; \
		if [ -L "$(BINDIR)/authama-touchid" ]; then unlink "$(BINDIR)/authama-touchid" && echo "removed the link $(BINDIR)/authama-touchid"; fi; \
		exit 0; fi; \
	if [ ! -e "$(INSTALLED)" ]; then echo "nothing to remove: $(INSTALLED) does not exist"; exit 0; fi; \
	case "$$("$(INSTALLED)" version 2>/dev/null | head -1)" in \
		authama\ *) ;; \
		*) echo "refusing: $(INSTALLED) does not answer 'version' as authama" >&2; exit 1;; \
	esac; \
	homes="$$("$(INSTALLED)" settings path 2>/dev/null | grep -E '^(config|state) home:')"; \
	rm -f "$(INSTALLED)" && echo "removed $(INSTALLED)"; \
	if [ -e "$(BINDIR)/authama-touchid" ]; then rm -f "$(BINDIR)/authama-touchid" && echo "removed $(BINDIR)/authama-touchid"; fi; \
	echo "kept, as they were: ~/.ssh, every token file, and"; \
	printf '%s\n' "$$homes" | sed 's/^/  /'; \
	if [ -e "$(BINDIR)/sshmgr$(EXE)" ]; then echo "note: an older sshmgr is still at $(BINDIR)/sshmgr$(EXE)"; fi

# --- running the tool against your own config -------------------------------
# These build first and run the binary you just built, never an installed one,
# so what you exercise is what you changed.

settings: build ## where the config and state homes are, and every setting with its source
	$(BIN)$(EXE) settings path
	$(BIN)$(EXE) settings show --effective

doctor: build ## verify environment (FIX=1 to auto-fix perms first)
	$(BIN)$(EXE) doctor $(if $(FIX),--fix,)

reconcile: build ## make ~/.ssh match the manifest
	$(BIN)$(EXE) ssh reconcile

render: build ## re-render config from the manifest
	$(BIN)$(EXE) ssh config render

rotate: build ## rotate a key:  make rotate KEY=<profile/key>
	$(BIN)$(EXE) ssh rotate $(KEY)

bundle: build ## encrypted backup
	$(BIN)$(EXE) ssh bundle
