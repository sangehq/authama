package configsvc

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// A stale config that could not be removed is an error, not a line under
// "removed stale" (review R-BUG-15: the Remove error was dropped and the file
// reported pruned while it was still there).
func TestAPruneThatFailsIsReportedNotListed(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the user cannot write")
	}
	ssh := t.TempDir()
	dir := filepath.Join(ssh, "profiles", "work")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("Host old\n    HostName old.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	res, err := New(ssh, load(t, manifestJSON), false).Write(false)
	if err == nil {
		t.Error("a prune that failed was not reported")
	}
	if res != nil && slices.Contains(res.Pruned, "profiles/work/config") {
		t.Error("a config still on disk was listed as removed")
	}
}
