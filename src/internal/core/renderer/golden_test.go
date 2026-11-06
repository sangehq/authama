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
	prev := SelfPath
	SelfPath = func() string { return "/usr/local/bin/authama" }
	t.Cleanup(func() { SelfPath = prev })
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
