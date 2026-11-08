package renderer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The client exports, the Termius CSV and the flat ssh_config are pinned byte
// for byte to files under testdata/golden, written from the code before the
// writers moved into util/encode. Whatever path renders them now has to give
// the same bytes. Regenerate a file only for a change meant to alter it:
//
//	UPDATE_GOLDEN=1 go test ./internal/core/renderer/ -run Golden
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // a testdata path
	if err != nil {
		t.Fatalf("%v; write it with UPDATE_GOLDEN=1", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from its golden file:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

func TestClientExportsMatchTheirGoldenFiles(t *testing.T) {
	m := mustManifest(t, exportJSON)
	names := ClientFormatNames()
	if len(names) < 7 {
		t.Fatalf("only %d client formats: %v", len(names), names)
	}
	files := 0
	for _, f := range names {
		exp, err := RenderClientExport(m, testExportPaths, f, TermiusOptions{IncludeGit: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range exp.Files {
			golden(t, f+"/"+file.Name, file.Data)
			files++
		}
		golden(t, f+"/report.txt", []byte(strings.Join(append(append([]string{}, exp.Skipped...), exp.Notes...), "\n")+"\n"))
	}
	if files < 9 {
		t.Fatalf("checked %d files; remmina alone writes one per host", files)
	}
}

func TestTermiusAndFlatConfigMatchTheirGoldenFiles(t *testing.T) {
	prev, prevMux := SelfPath, Multiplex
	SelfPath = func() string { return "/usr/local/bin/authama" }
	Multiplex = func() bool { return true } // the default everywhere but Windows
	t.Cleanup(func() { SelfPath, Multiplex = prev, prevMux })
	m := mustManifest(t, exportJSON)
	exp, err := RenderTermiusCSV(m, testExportPaths, TermiusOptions{IncludeGit: true})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "termius/termius-hosts.csv", []byte(exp.CSV))
	flat, err := RenderFlatConfig(m, testExportPaths, false)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "ssh_config/ssh_config", []byte(flat))
}

// The flat export of every host, named one by one, is the flat export; of
// some, it is their blocks as the whole export writes them, the git block only
// with its github.com host, and Host * last.
func TestFlatConfigOfChosenHostsIsTheirPartOfTheWhole(t *testing.T) {
	m := mustManifest(t, exportJSON)
	whole, err := RenderFlatConfig(m, testExportPaths, false)
	if err != nil {
		t.Fatal(err)
	}
	all, err := RenderFlatConfigOf(m, testExportPaths, false, []string{"devbox", "box", "github-imani", "oribi-web"})
	if err != nil || all != whole {
		t.Fatalf("every host named is not the whole export (%v):\n%s", err, all)
	}
	some, err := RenderFlatConfigOf(m, testExportPaths, false, []string{"box"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(whole, some[len(FlatHeader)+1:strings.Index(some, "Host *")]) {
		t.Errorf("box's part is not as the whole export writes it:\n%s", some)
	}
	for _, gone := range []string{"Host github.com", "Host oribi-web", "profile: simtabi"} {
		if strings.Contains(some, gone) {
			t.Errorf("%q is in the export of box alone:\n%s", gone, some)
		}
	}
	withGit, _ := RenderFlatConfigOf(m, testExportPaths, false, []string{"github-imani"})
	if !strings.Contains(withGit, "Host github.com\n") || !strings.HasSuffix(strings.TrimSpace(withGit), "HashKnownHosts yes") {
		t.Errorf("the git block comes with its github host, Host * last:\n%s", withGit)
	}
	if _, err := RenderFlatConfigOf(m, testExportPaths, false, []string{"nope"}); err == nil {
		t.Error("an unknown alias was accepted")
	}
}
