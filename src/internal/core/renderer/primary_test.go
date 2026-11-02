package renderer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrimaryForPrefersAuthamaOverTheDeprecatedName(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "sshmgr")
	primary := filepath.Join(dir, "authama")
	for _, p := range []string{legacy, primary} {
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	missing := func(string) (string, error) { return "", errors.New("not found") }
	if got := primaryFor(legacy, missing); got != primary {
		t.Errorf("sibling: got %q, want %q", got, primary)
	}
	if got := primaryFor(primary, missing); got != primary {
		t.Errorf("authama itself: got %q", got)
	}

	alone := filepath.Join(t.TempDir(), "sshmgr")
	if got := primaryFor(alone, func(string) (string, error) { return "/usr/bin/authama", nil }); got != "/usr/bin/authama" {
		t.Errorf("PATH fallback: got %q", got)
	}
	if got := primaryFor(alone, missing); got != alone {
		t.Errorf("no authama anywhere: got %q, want the alias kept", got)
	}
}
