package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// File hygiene, in the language the repository is written in.
//
// These checks used to come from pre-commit, which needs an interpreter this
// repository does not otherwise use - the last thing that did, and a second CI job with
// its own toolchain setup to run four rules about whitespace. The rules are
// worth keeping; the runtime was not.
//
// Two of pre-commit's hooks are not reimplemented here because they were already
// covered twice over: gitleaks runs as its own CI job against the full history
// (which the hook could not do), and gofmt/go vet are in `make ci`. What is left
// is what this file does, and it now runs in `make check` as well as in CI,
// where before it only ran for people who had installed pre-commit.
//
// Not covered: YAML and TOML parse checks. There is no TOML left in the tree,
// and validating YAML would mean adding a dependency to a binary whose linked
// modules are pinned by TestTheBinaryLinksOnlyItsTwoChosenDependencies.
// The YAML that matters is checked anyway - GitHub rejects a malformed workflow,
// and `goreleaser check` parses the release config.

// maxTrackedFileSize matches pre-commit's check-added-large-files default. The
// repository ships no binaries, so anything approaching this is a mistake.
const maxTrackedFileSize = 500 * 1024

// vendoredUpstream are third-party files committed byte for byte as their
// release ships them, so neither the size cap nor the text rules apply: a
// newline added to tabler.min.css would be an edit to someone else's file, and
// TestVendoredFilesMatchTheManifest in internal/web would rightly fail on it.
// Each entry is a ceiling, measured 2026-10-27 at @tabler/core 1.7.1: a file that
// grows past it fails, so an upgrade is a decision, not a silent 2 MB commit.
var vendoredUpstream = map[string]int{
	"src/internal/web/assets/vendor/tabler/tabler.min.css":      700 * 1024, // 683,670 bytes
	"src/internal/web/assets/vendor/tabler/tabler.min.js":       160 * 1024, // 148,876 bytes
	"src/internal/web/assets/vendor/tabler/tabler-theme.min.js": 4 * 1024,   // 1,402 bytes
}

// trackedFiles lists what git tracks, so generated and ignored files are out of
// scope by construction rather than by an exclusion list that drifts.
func trackedFiles(t *testing.T) (root string, files []string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Dir(moduleRoot)

	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		t.Fatal("git tracks no files; this test would pass vacuously")
	}
	return root, files
}

// isBinary follows .gitattributes: those extensions, and golden files (which
// are compared byte for byte, CRLF and UTF-16 included, so git keeps them
// -text), are the only ones exempt from the text rules below.
func isBinary(name string) bool {
	if strings.Contains(filepath.ToSlash(name), "/testdata/golden/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".age", ".gz", ".zip", ".ico":
		return true
	}
	return false
}

func TestTrackedFilesAreHygienic(t *testing.T) {
	root, files := trackedFiles(t)
	for name := range vendoredUpstream {
		if !slices.Contains(files, name) {
			t.Errorf("%s is exempted as vendored but not tracked; remove the stale entry", name)
		}
	}
	for _, name := range files {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if ceiling, ok := vendoredUpstream[name]; ok {
			if len(body) > ceiling {
				t.Errorf("%s is %d bytes, over its recorded ceiling of %d; raise it on purpose, with the new measurement",
					name, len(body), ceiling)
			}
			continue
		}
		if len(body) > maxTrackedFileSize {
			t.Errorf("%s is %d bytes, over the %d-byte limit for a tracked file",
				name, len(body), maxTrackedFileSize)
		}
		if isBinary(name) || len(body) == 0 {
			continue
		}
		text := string(body)

		// .gitattributes says `* text=auto eol=lf`, so a CR in the working tree
		// is a file that escaped normalization.
		if strings.Contains(text, "\r") {
			t.Errorf("%s contains a carriage return; .gitattributes normalizes to LF", name)
		}
		// Exactly one trailing newline: none makes a diff noisy the next time the
		// last line changes, several are accidental.
		if !strings.HasSuffix(text, "\n") {
			t.Errorf("%s does not end with a newline", name)
		} else if strings.HasSuffix(text, "\n\n") {
			t.Errorf("%s ends with a blank line", name)
		}
		for i, line := range strings.Split(text, "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("%s:%d ends in whitespace", name, i+1)
			}
		}
	}
}

// A merge marker committed to a tracked file is a broken file that still parses
// often enough to be missed.
func TestNoTrackedFileHasMergeMarkers(t *testing.T) {
	root, files := trackedFiles(t)
	// Split so this file does not match its own check.
	markers := []string{"<<<<<<" + "<", "=======" + "\n>>>>>>" + ">", ">>>>>>" + ">"}
	for _, name := range files {
		if isBinary(name) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		for _, m := range markers {
			if strings.Contains(string(body), m) {
				t.Errorf("%s contains a merge conflict marker", name)
				break
			}
		}
	}
}

// Every tracked JSON file parses. Several are shipped as data - the example
// manifest, the provider catalog, the schemas - and a malformed one is a runtime
// failure for a user rather than a build failure for us.
func TestEveryTrackedJSONFileParses(t *testing.T) {
	root, files := trackedFiles(t)
	var checked int
	for _, name := range files {
		if filepath.Ext(name) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var any interface{}
		if err := json.Unmarshal(body, &any); err != nil {
			t.Errorf("%s is not valid JSON: %v", name, err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no JSON files were checked; this test has stopped measuring anything")
	}
}

// A documented raw.githubusercontent URL has to point at a file that exists in
// this repository. The install command in the README pointed at a vanity path
// that was never published and returned 404 - the primary way to install this
// tool could not fetch the script at all - and nothing noticed, because nothing
// checks a URL. This cannot verify what a host serves, but it does verify the
// half that is knowable here: that the path names a file we actually ship. It is
// the check that would have caught the tree moving under src/.
func TestDocumentedRawURLsPointAtFilesThatExist(t *testing.T) {
	root, files := trackedFiles(t)
	re := regexp.MustCompile(`https://raw\.githubusercontent\.com/simtabi/authama/[^/]+/([^\s)"'` + "`" + `]+)`)

	var checked int
	for _, name := range files {
		if isBinary(name) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(string(body), -1) {
			path := strings.TrimRight(m[1], ".,")
			if _, err := os.Stat(filepath.Join(root, path)); err != nil {
				t.Errorf("%s links to %s, which is not a file in this repository", name, path)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no raw URLs were checked; the install commands are documented with one, so this has stopped measuring anything")
	}
}

// TestMarkdownFileNamesAreLowercase keeps documentation file names as lowercase slugs. The
// exceptions are the conventional upper-case names GitHub and readers look for.
func TestMarkdownFileNamesAreLowercase(t *testing.T) {
	_, files := trackedFiles(t)
	conventional := []string{"README.md", "CHANGELOG.md", "CONTRIBUTING.md", "SECURITY.md", "LICENSE.md", "CODE_OF_CONDUCT.md", "PULL_REQUEST_TEMPLATE.md"}
	for _, f := range files {
		base := filepath.Base(f)
		if !strings.EqualFold(filepath.Ext(base), ".md") || slices.Contains(conventional, base) {
			continue
		}
		if base != strings.ToLower(base) {
			t.Errorf("%s: markdown file names are lowercase slugs (rename to %s)", f, strings.ToLower(base))
		}
	}
}

// minTestFiles is the floor for TestTestsGenerateTheirCredentials: 170 tracked
// *_test.go files on 2026-11-02. A glob that stops matching would otherwise
// report a clean tree having read nothing.
const minTestFiles = 150

// minBrowserTestFiles is the same floor for the browser tests under e2e/web:
// 5 tracked TypeScript files on 2026-11-02.
const minBrowserTestFiles = 4

// credentialLiteral matches what a secret scanner reports in a test: a value
// shaped like a provider token, a SECRET_* placeholder, or any literal assigned
// to a field, variable or environment key named for a credential. Quotes are
// any of " ' and ` so the same rules read Go and the TypeScript browser tests.
var credentialLiteral = regexp.MustCompile(
	"[\"'`](?:ghp|gho|ghs|ghu|github_pat|glpat|npm|xox[abp])[_-][A-Za-z0-9_]+[\"'`]" +
		"|[\"'`]SECRET_[A-Z0-9_]+[\"'`]" +
		"|(?:^|[\\s,{(])(?:[Pp]assword|[Pp]assphrase|[Ss]ecret|[Tt]oken)\\s*(?::|=|:=)\\s*[\"'`][^\"'`]+[\"'`]" +
		"|\\[[\"'`][A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD)[A-Z0-9_]*[\"'`]\\]\\s*=\\s*[\"'`][^\"'`]+[\"'`]")

// credentialExempt marks a line whose literal must be exact bytes (a test
// vector, a fingerprinted key). It goes on the line above, so the literal's
// own line never shows up in a diff because of the marker.
const credentialExempt = "@credential-literal-exempt:"

// Tests get credentials from internal/util/testcred at run time. A literal is a
// scanner incident somebody closes by hand, however obviously fake it is.
func TestTestsGenerateTheirCredentials(t *testing.T) {
	root, files := trackedFiles(t)
	inspected, browser := 0, 0
	for _, f := range files {
		switch {
		case strings.HasPrefix(f, "src/") && strings.HasSuffix(f, "_test.go"):
			inspected++
		case strings.HasPrefix(f, "e2e/web/") && strings.HasSuffix(f, ".ts"):
			browser++
		default:
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(body), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "//") || !credentialLiteral.MatchString(line) {
				continue
			}
			if i > 0 && strings.Contains(lines[i-1], credentialExempt) {
				continue
			}
			t.Errorf("%s:%d: credential literal in a test; generate it at run time (testcred in Go): %s",
				f, i+1, strings.TrimSpace(line))
		}
	}
	if inspected < minTestFiles {
		t.Fatalf("inspected %d test files, want at least %d: the file list is not matching", inspected, minTestFiles)
	}
	if browser < minBrowserTestFiles {
		t.Fatalf("inspected %d browser test files under e2e/web, want at least %d", browser, minBrowserTestFiles)
	}
}

// goFetch is a command that installs or fetches this module through the Go
// toolchain, which reads the public module proxy.
var goFetch = regexp.MustCompile(`go (?:install|get) \S*github\.com/simtabi/authama`)

// TestNoDocOffersGoInstall keeps every doc, the release page and the migration
// guide included, from offering `go install` or `go get` of the module: binaries
// ship on the GitHub release only (~/.claude/rules/go.md), and the proxy still
// serves versions cached from deleted history. CHANGELOG.md is history and is
// not read.
func TestNoDocOffersGoInstall(t *testing.T) {
	root, files := trackedFiles(t)
	pages := 0
	for _, name := range files {
		if !strings.HasSuffix(name, ".md") || name == "CHANGELOG.md" || strings.HasPrefix(name, ".claude/") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for i, line := range strings.Split(string(body), "\n") {
			if goFetch.MatchString(line) {
				t.Errorf("%s:%d offers the module through the Go toolchain; binaries ship on the GitHub release: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if pages < 40 {
		t.Fatalf("read %d markdown files; the walk is not finding them", pages)
	}
}

// sharedSigner is the workflow that signs every release's provenance: the
// shared release workflow in simtabi/.github, which release.yml calls.
const sharedSigner = "--signer-workflow simtabi/.github/.github/workflows/go-release.yml"

// TestAttestationChecksNameTheSharedSigner keeps every `gh attestation verify`
// a doc shows runnable. The shared workflow signs the provenance, so without
// --signer-workflow naming it, gh expects a workflow in this repository and
// the check fails (measured against v0.1.6 on 2026-11-13). A command continued
// with a trailing backslash is read as one line.
func TestAttestationChecksNameTheSharedSigner(t *testing.T) {
	root, files := trackedFiles(t)
	commands := 0
	for _, name := range files {
		if !strings.HasSuffix(name, ".md") || name == "CHANGELOG.md" || strings.HasPrefix(name, ".claude/") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(body), "\\\n", " ")
		for i, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, "gh attestation verify") {
				continue
			}
			commands++
			if !strings.Contains(strings.Join(strings.Fields(line), " "), sharedSigner) {
				t.Errorf("%s (near line %d) shows gh attestation verify without %s; it fails for a release the shared workflow signed: %s", name, i+1, sharedSigner, strings.TrimSpace(line))
			}
		}
	}
	if commands < 2 {
		t.Fatalf("found %d gh attestation verify commands in the docs, want at least 2 (installation.md and release.md)", commands)
	}
}

// releaseChannel is a command that installs authama from a release channel:
// the Homebrew cask, the Scoop manifest, go install, or a piped install script.
var releaseChannel = regexp.MustCompile(`brew install --cask simtabi/|scoop install simtabi/|go install github\.com/simtabi/authama|install\.(?:sh|ps1)\S* *\| *(?:ba)?sh|irm \S+install\.ps1`)

// TestDocsOfferNoReleaseChannelAsCurrent keeps the docs to the current ways to
// install: a binary from the GitHub release, or a build from source. The
// release page names the frozen channels, the "To 0.1.x" sections of the
// migration guide are upgrade notes for withdrawn builds people still have,
// and installation.md's "Earlier binary releases" says those channels are
// frozen; everywhere else a channel command reads as an instruction that no
// longer works (the cask and the Scoop manifest point at withdrawn assets).
func TestDocsOfferNoReleaseChannelAsCurrent(t *testing.T) {
	root, files := trackedFiles(t)
	heading := regexp.MustCompile(`^#{1,6} `)
	historical := regexp.MustCompile(`^#{1,6} (To 0\.|Earlier binary releases)`)
	pages := 0
	for _, name := range files {
		if !strings.HasSuffix(name, ".md") || (!strings.HasPrefix(name, "docs/") && name != "README.md") || name == "docs/release.md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		pages++
		level, inHistory, fenced := 0, false, false
		for i, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fenced = !fenced
			}
			if !fenced && heading.MatchString(line) {
				l := strings.Index(line, " ")
				if historical.MatchString(line) {
					level, inHistory = l, true
				} else if inHistory && l <= level {
					inHistory = false
				}
			}
			if !inHistory && releaseChannel.MatchString(line) {
				t.Errorf("%s:%d offers a release channel as a way to install; binaries ship on the GitHub release only, so point at it or at building from source: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if pages < 40 {
		t.Fatalf("read %d docs pages; the walk is not finding them", pages)
	}
}
