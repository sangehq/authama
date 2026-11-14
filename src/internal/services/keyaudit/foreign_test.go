package keyaudit

import (
	"os"
	"path/filepath"
	"testing"
)

// A key a hand-written Host block in ~/.ssh/config uses is not loose: it was
// reported as "a key in ~/.ssh that no Host block uses", which made
// `doctor --strict` fail on a home that was fine (review R-BUG-08). An
// IdentityFile in an Included file counts too.
func TestAKeyAForeignHostUsesIsNotLoose(t *testing.T) {
	f := newFixture(t)
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJ imani@laptop\n"
	write := func(name, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(f.ssh, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.ssh, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"id_legacy", "id_included", "id_unused"} {
		write(k, "PRIVATE\n")
		write(k+".pub", pub)
	}
	write("config", "Include "+filepath.Join(f.ssh, "conf.d", "*")+"\n\nHost legacy\n    HostName legacy.example\n    IdentityFile "+filepath.Join(f.ssh, "id_legacy")+"\n")
	write("conf.d/extra", "Host extra\n    IdentityFile "+filepath.Join(f.ssh, "id_included")+"\n")

	got := subjects(audit(t, f, false), Loose)
	if len(got) != 1 || got[0] != "id_unused" {
		t.Errorf("loose = %v, want only id_unused", got)
	}
}
