package bundler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// riskyBundle is a bundle whose manifest sets the agent forwarded and host
// keys unchecked for every host, as a forged one could.
func riskyBundle(t *testing.T) string {
	t.Helper()
	ssh, cfg := writeSrc(t)
	doc := `{"defaults": {"global_options": {"ForwardAgent": "yes", "StrictHostKeyChecking": "no"}}, "profiles": {}}`
	if err := os.WriteFile(filepath.Join(cfg, "manifest.json"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = ssh
	res, err := New(ssh, cfg, fakeCipher{}).Bundle("age1fake", t.TempDir(), "20260101")
	if err != nil {
		t.Fatal(err)
	}
	return res.AgePath
}

// A bundle whose manifest would redirect or weaken connections is refused
// before anything is written, naming each option and how to allow it on
// purpose (audit A-SEC-13). Allowed, it is restored and the options are
// listed in what the restore says it did.
func TestRestoreRefusesRiskySSHOptionsUnlessAllowed(t *testing.T) {
	bundle := riskyBundle(t)
	base := t.TempDir()
	ssh2, cfg2 := filepath.Join(base, ".ssh"), filepath.Join(base, "cfg")
	_, err := New(ssh2, cfg2, fakeCipher{}).Restore(bundle, "", fakeFP)
	if err == nil {
		t.Fatal("a bundle forwarding the agent to every host was restored")
	}
	for _, want := range []string{"ForwardAgent yes", "StrictHostKeyChecking no", "--allow-risky-options"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cfg2, "manifest.json")); err == nil {
		t.Error("the refused bundle's manifest was laid down")
	}

	opened, err := New(ssh2, cfg2, fakeCipher{}).Open(bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.Risky()) != 2 {
		t.Errorf("the preview lists %d risky options, want 2: %v", len(opened.Risky()), opened.Risky())
	}

	res, err := New(ssh2, cfg2, fakeCipher{}).AllowRiskyOptions().Restore(bundle, "", fakeFP)
	if err != nil {
		t.Fatalf("an allowed restore was refused: %v", err)
	}
	if out := res.Format(); !strings.Contains(out, "ForwardAgent yes") || !strings.Contains(out, "allowed") {
		t.Errorf("the restore does not say which risky options it laid down:\n%s", out)
	}
}
