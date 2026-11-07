package cli

import (
	"os/exec"
	"strings"
	"testing"
)

// v1 scripts passed --no-passphrase to keygen, reconcile and rotate; the 2.0.0
// rewrite dropped it and those scripts failed with "unknown flag". It is back
// as a hidden no-op, and it still cannot be combined with --passphrase.
func TestNoPassphraseIsAcceptedAndExclusive(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")

	if out, err := runErr(t, "ssh", "reconcile", "--no-passphrase", "--no-pin", "-y"); err != nil {
		t.Fatalf("reconcile --no-passphrase: %v\n%s", err, out)
	}
	if out, err := runErr(t, "ssh", "keygen", "work", "--no-passphrase"); err != nil {
		t.Fatalf("keygen --no-passphrase: %v\n%s", err, out)
	}
	for _, verb := range []string{"keygen", "reconcile", "rotate"} {
		args := []string{"ssh", verb, "--passphrase", "--no-passphrase"}
		if verb != "reconcile" {
			args = append(args, "work")
		}
		out, err := runErr(t, args...)
		if err == nil || !strings.Contains(err.Error()+out, "none of the others can be") {
			t.Errorf("%s accepted --passphrase with --no-passphrase: %v\n%s", verb, err, out)
		}
	}
}

func TestNoPassphraseStaysOutOfHelp(t *testing.T) {
	for _, verb := range []string{"keygen", "reconcile", "rotate"} {
		out := run(t, "ssh", verb, "--help")
		if strings.Contains(out, "no-passphrase") {
			t.Errorf("%s --help shows the compatibility flag:\n%s", verb, out)
		}
	}
}
