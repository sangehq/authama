package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildEntries is everything build/ may hold at its top: the tracked inputs,
// the checksums over the binaries, the packages, and one folder per OS.
var buildEntries = map[string]bool{
	"targets.txt": true, "nfpm.yaml": true, "checksums.txt": true, "archives": true,
	"macos": true, "linux": true, "windows": true, "freebsd": true, "openbsd": true, "netbsd": true,
}

// osMetadata are files a desktop writes into any folder it shows - Finder's
// .DS_Store, Explorer's desktop.ini and Thumbs.db. Opening build/ to
// double-click a binary makes them; no tool of ours does, so they are not
// layout drift.
var osMetadata = map[string]bool{".DS_Store": true, "desktop.ini": true, "Thumbs.db": true}

func repoTop(t *testing.T) string {
	t.Helper()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(module)
}

// build/ holds one folder per OS and nothing else, and every OS folder holds
// only binaries: no build/dist/, no per-target subfolders, no tool output.
// The tools that write build/ are checked too, so the rule holds on a fresh
// checkout where build/ holds nothing yet.
func TestBuildHoldsOneFolderPerOSAndNothingElse(t *testing.T) {
	top := repoTop(t)
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(top, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	// The writers. A comment may name the old build/dist/; a setting may not.
	settings := func(text string) string {
		var keep []string
		for _, line := range strings.Split(text, "\n") {
			if trimmed := strings.TrimSpace(line); !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	}
	checked := 0
	for _, rel := range []string{"Makefile", "src/scripts/build-all.sh", "src/scripts/macos-app.sh", "src/.goreleaser.yaml", ".github/workflows/release.yml"} {
		if strings.Contains(settings(read(rel)), "build/dist") {
			t.Errorf("%s still writes or reads build/dist", rel)
		}
		checked++
	}
	if checked < 5 {
		t.Fatalf("inspected %d writers", checked)
	}
	if !regexp.MustCompile(`(?m)^dist:\s*\.goreleaser-dist\s*$`).MatchString(read("src/.goreleaser.yaml")) {
		t.Error("GoReleaser's dist: is not .goreleaser-dist (its working output must stay out of build/)")
	}
	if !regexp.MustCompile(`(?m)^BIN\s*:=\s*build/\$\(HOST_OS\)/authama\s*$`).MatchString(read("Makefile")) {
		t.Error("make build does not write build/$(HOST_OS)/authama")
	}

	// What is on disk now, if anything has been built.
	entries, err := os.ReadDir(filepath.Join(top, "build"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if osMetadata[e.Name()] {
			continue
		}
		if !buildEntries[e.Name()] {
			t.Errorf("build/%s is not one of the OS folders, archives/, checksums.txt or the tracked inputs", e.Name())
			continue
		}
		if !e.IsDir() || e.Name() == "archives" {
			continue
		}
		inner, err := os.ReadDir(filepath.Join(top, "build", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range inner {
			if f.IsDir() && !osMetadata[f.Name()] {
				t.Errorf("build/%s/%s is a folder; an OS folder holds binaries only", e.Name(), f.Name())
			}
		}
	}
}
