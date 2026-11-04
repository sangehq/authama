package events

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

func home(t *testing.T) (paths.Paths, string) {
	t.Helper()
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	for _, d := range []string{p.SSHDir, p.ConfigDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return p, filepath.Join(base, ".npmrc")
}

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// Every kind of change to every area is recorded, and nothing of any file's
// contents reaches the log or the state.
func TestScanRecordsEveryChangeAndNoContents(t *testing.T) {
	p, npmrc := home(t)
	secret := testcred.Secret("npm")
	key := filepath.Join(p.SSHDir, "id_test")
	manifest := filepath.Join(p.ConfigDir, "manifest.json")
	write(t, key, "private\n", 0o600)
	write(t, manifest, "{}\n", 0o600)
	roots := Roots(p, []string{npmrc})
	t0 := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

	evs, err := Scan(p, roots, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Kind != KindStarted || !strings.Contains(evs[0].Detail, "2 file(s)") {
		t.Fatalf("first scan = %+v", evs)
	}

	write(t, npmrc, "//registry.npmjs.org/:_authToken="+secret+"\n", 0o600) // added, auth
	write(t, manifest, `{"version": 1}`+"\n", 0o600)                        // changed, config
	if err := os.Chmod(key, 0o644); err != nil {                            // mode, ssh
		t.Fatal(err)
	}
	pub := filepath.Join(p.SSHDir, "id_test.pub")
	write(t, pub, "ssh-ed25519 AAAA\n", 0o644)
	evs, err = Scan(p, roots, t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range evs {
		got[e.Path] = e.Kind + "/" + e.Area
	}
	want := map[string]string{npmrc: "added/auth", manifest: "changed/config", key: "mode/ssh", pub: "added/ssh"}
	if runtime.GOOS == "windows" {
		// Go's chmod on Windows sets only the read-only bit: 0600 to 0644 is
		// no change there, so there is no mode event to see.
		delete(want, key)
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%s: %q, want %q (all: %v)", filepath.Base(path), got[path], w, got)
		}
	}

	if err := os.Remove(pub); err != nil {
		t.Fatal(err)
	}
	evs, _ = Scan(p, roots, t0.Add(2*time.Minute))
	if len(evs) != 1 || evs[0].Kind != KindRemoved || evs[0].Path != pub {
		t.Errorf("removal = %+v", evs)
	}
	if evs, _ := Scan(p, roots, t0.Add(3*time.Minute)); len(evs) != 0 {
		t.Errorf("a scan with nothing changed recorded %+v", evs)
	}

	for _, f := range []string{StateFile(p), LogFile(p)} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), secret) || strings.Contains(string(b), "private") {
			t.Errorf("%s holds a file's contents", filepath.Base(f))
		}
		if fi, _ := os.Stat(f); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
			t.Errorf("%s is %o, want 0600", filepath.Base(f), fi.Mode().Perm())
		}
	}

	all, err := List(p, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	wantAll, wantSSH := 6, 3
	if runtime.GOOS == "windows" {
		wantAll, wantSSH = 5, 2
	}
	if len(all) != wantAll || all[0].Kind != KindRemoved {
		t.Errorf("List = %d events, newest %+v", len(all), all[0])
	}
	if ssh, _ := List(p, Filter{Area: AreaSSH}); len(ssh) != wantSSH {
		t.Errorf("ssh events = %d, want %d", len(ssh), wantSSH)
	}
	if lim, _ := List(p, Filter{Limit: 2}); len(lim) != 2 {
		t.Errorf("limit 2 gave %d", len(lim))
	}
}

// A home that was never scanned lists nothing, and a torn log line is skipped.
func TestListToleratesNoLogAndATornLine(t *testing.T) {
	p, _ := home(t)
	if evs, err := List(p, Filter{}); err != nil || evs != nil {
		t.Fatalf("no log: %v %v", evs, err)
	}
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, LogFile(p), `{"time":"2026-11-04T12:00:00Z","kind":"added","area":"ssh","path":"/x"}`+"\n"+`{"time":`, 0o600)
	if evs, err := List(p, Filter{}); err != nil || len(evs) != 1 {
		t.Fatalf("torn log: %v %v", evs, err)
	}
}
