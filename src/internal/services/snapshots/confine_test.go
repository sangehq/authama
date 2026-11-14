package snapshots

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Extraction is rooted at the parent of ~/.ssh, which is $HOME, so a member
// named .zshrc was written there (review R-SEC-15). Every member must now be
// inside .ssh/, and the whole archive is refused before anything is written.
func TestRestoreRefusesMembersOutsideTheSSHDir(t *testing.T) {
	base := t.TempDir()
	ssh := filepath.Join(base, ".ssh")
	writeTree(t, ssh)
	tarball := filepath.Join(base, "crafted.tar.gz")
	craftArchive(t, tarball, map[string]string{".ssh/config": "# restored\n", ".zshrc": "echo owned\n"})
	if err := Restore(tarball, ssh); err == nil {
		t.Error("an archive with a member outside .ssh/ was restored")
	}
	if _, err := os.Lstat(filepath.Join(base, ".zshrc")); err == nil {
		t.Fatal(".zshrc was written beside ~/.ssh")
	}
	if got, _ := os.ReadFile(filepath.Join(ssh, "config")); string(got) == "# restored\n" {
		t.Error("the refused archive still changed ~/.ssh/config")
	}
}

// The modes in an archive are not trusted: a world-writable config or a
// group-writable directory comes back owner-writable only.
func TestRestoreCapsTheModesInTheArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	base := t.TempDir()
	ssh := filepath.Join(base, ".ssh")
	writeTree(t, ssh)
	tarball := filepath.Join(base, "loose.tar.gz")
	f, err := os.Create(tarball)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, h := range []*tar.Header{
		{Name: ".ssh/", Mode: 0o777, Typeflag: tar.TypeDir},
		{Name: ".ssh/sub/", Mode: 0o775, Typeflag: tar.TypeDir},
		{Name: ".ssh/config", Mode: 0o666, Size: 3, Typeflag: tar.TypeReg},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte("# x"))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	if err := Restore(tarball, ssh); err != nil {
		t.Fatal(err)
	}
	for p, max := range map[string]os.FileMode{ssh: 0o700, filepath.Join(ssh, "sub"): 0o700, filepath.Join(ssh, "config"): 0o644} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&^max != 0 {
			t.Errorf("%s restored with mode %v, more open than %v", p, fi.Mode().Perm(), max)
		}
	}
}

// A snapshot that fails part-way leaves no file behind to be listed as a
// restore point and counted against retention (review R-BUG-13).
func TestASnapshotThatFailsLeavesNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unreadable file")
	}
	base := t.TempDir()
	ssh := filepath.Join(base, ".ssh")
	writeTree(t, ssh)
	cfg := filepath.Join(ssh, "config")
	if err := os.Chmod(cfg, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfg, 0o600) })
	dir := filepath.Join(base, "snapshots")
	if _, err := Snapshot(ssh, dir, 10, "20260101-000000"); err == nil {
		t.Fatal("a snapshot of an unreadable config succeeded")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("a failed snapshot left %s behind", e.Name())
	}
}
