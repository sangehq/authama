package web

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The three sange-family repositories (sange, authama, imprimatur) vendor one
// Tabler set (owner, 2026-11-25: "use sanges newest and keep them in sync
// across all 3 sange repos"). This test compares authama's TABLER_VERSION with
// sange's src/internal/web/vendor.md and imprimatur's TABLER_VERSION, each read
// from the sibling's origin/main rather than its working tree, when the
// checkouts sit next to this one. Where a sibling is absent (CI clones only
// this repository) it skips with the reason. Bootstrap is the one documented
// difference: sange builds 5.3.9 from source, while tabler.min.js 1.7.5 bundles
// 5.3.8, which is what authama and imprimatur pin.

const minPins = 5

// siblingRoot is the directory holding the sibling checkouts: the repository
// root is three levels above this package, and its parent holds the others.
func siblingRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// pinsFromTablerVersion reads "name version" lines, skipping comments, and
// names the icon sprite by its package family so it compares with sange's
// @tabler/icons.
func pinsFromTablerVersion(text string) map[string]string {
	pins := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		name := f[0]
		if name == "@tabler/icons-sprite" {
			name = "@tabler/icons"
		}
		pins[name] = f[1]
	}
	return pins
}

var vendorMDRow = regexp.MustCompile("^\\| `([^`]+)` \\| ([0-9][^ |]*) \\|")

// pinsFromVendorMD reads the package table in sange's vendor.md.
func pinsFromVendorMD(text string) map[string]string {
	pins := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if m := vendorMDRow.FindStringSubmatch(line); m != nil {
			pins[m[1]] = m[2]
		}
	}
	return pins
}

// showOriginMain reads path from the sibling's origin/main, or skips when the
// sibling checkout or its origin/main is not there.
func showOriginMain(t *testing.T, repo, path string) string {
	t.Helper()
	dir := filepath.Join(siblingRoot(t), repo)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Skipf("no %s checkout at %s, so its Tabler pins are not compared", repo, dir)
	}
	if err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "-q", "origin/main").Run(); err != nil {
		t.Skipf("%s at %s has no origin/main, so its Tabler pins are not compared", repo, dir)
	}
	out, err := exec.Command("git", "-C", dir, "show", "origin/main:"+path).Output()
	if err != nil {
		t.Fatalf("git -C %s show origin/main:%s: %v", dir, path, err)
	}
	return string(out)
}

func ourPins(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("assets", "vendor", "TABLER_VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	ours := pinsFromTablerVersion(string(b))
	if len(ours) < minPins {
		t.Fatalf("read %d pins from TABLER_VERSION, want at least %d", len(ours), minPins)
	}
	return ours
}

// drift lists every package whose version differs, or that only one side
// pins, leaving out the names in except.
func drift(ours, theirs map[string]string, except ...string) []string {
	skip := map[string]bool{}
	for _, e := range except {
		skip[e] = true
	}
	names := map[string]bool{}
	for n := range ours {
		names[n] = true
	}
	for n := range theirs {
		names[n] = true
	}
	var out []string
	for n := range names {
		if skip[n] || ours[n] == theirs[n] {
			continue
		}
		out = append(out, n+": authama "+orNone(ours[n])+", sibling "+orNone(theirs[n]))
	}
	sort.Strings(out)
	return out
}

func orNone(v string) string {
	if v == "" {
		return "(not pinned)"
	}
	return v
}

func TestTablerPinsMatchSange(t *testing.T) {
	ours := ourPins(t)
	theirs := pinsFromVendorMD(showOriginMain(t, "sange", "src/internal/web/vendor.md"))
	if len(theirs) < minPins {
		t.Fatalf("read %d pins from sange's vendor.md, want at least %d", len(theirs), minPins)
	}
	if d := drift(ours, theirs, "bootstrap"); len(d) > 0 {
		t.Errorf("authama and sange vendor different Tabler sets:\n  %s", strings.Join(d, "\n  "))
	}
}

func TestTablerPinsMatchImprimatur(t *testing.T) {
	ours := ourPins(t)
	theirs := pinsFromTablerVersion(showOriginMain(t, "imprimatur", "src/internal/web/assets/vendor/TABLER_VERSION"))
	if len(theirs) < minPins {
		t.Fatalf("read %d pins from imprimatur's TABLER_VERSION, want at least %d", len(theirs), minPins)
	}
	if d := drift(ours, theirs); len(d) > 0 {
		t.Errorf("authama and imprimatur vendor different Tabler sets:\n  %s", strings.Join(d, "\n  "))
	}
}
