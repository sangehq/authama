package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/envnames"
)

// recordSSH puts an ssh stub first on PATH that appends each argv to a log,
// each call ended by a marker, and returns a reader for the log.
func recordSSH(t *testing.T) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv.txt")
	stub := "#!/bin/sh\nprintf '%s ' \"$@\" >> " + log + "\nprintf '@@END@@' >> " + log + "\ncat > /dev/null\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		b, _ := os.ReadFile(log)
		_ = os.Remove(log)
		return strings.Split(strings.TrimSuffix(string(b), "@@END@@"), "@@END@@")
	}
}

// Every ssh authama runs against a host - deploy, verify, remove, the
// authorized_keys read and GitHub's greeting - sends trust-on-first-use only
// while auto-pin is on. With AUTHAMA_AUTO_PIN=0 it says nothing about host
// keys, so the user's own ssh_config (StrictHostKeyChecking yes, say) decides,
// rather than a command-line value that wins over it (audit S-03, the rule
// R-SEC-16 set for the reachability probe).
func TestHostKeyCheckingFollowsAutoPin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the ssh stub is POSIX sh")
	}
	target := Target{Alias: "a", Hostname: "h", User: "u", Port: 22, IdentityPath: "/id",
		PubkeyText: "ssh-ed25519 " + targetBody + " me@host", KnownHosts: "/kh"}
	calls := map[string]func(){
		"deploy": func() { (GenericSSH{}).deployOverSSH(target) },
		"verify": func() { (GenericSSH{}).Verify(target) },
		"remove": func() { (GenericSSH{}).Remove(target) },
		"read":   func() { (GenericSSH{}).ListDeployed(target) },
		"github": func() { (GitHub{}).sshLogin(target) },
	}
	for _, tc := range []struct {
		pin  string
		want bool
	}{{"", true}, {"1", true}, {"0", false}, {"off", false}, {"false", false}} {
		for name, call := range calls {
			t.Run(name+"/pin="+tc.pin, func(t *testing.T) {
				read := recordSSH(t)
				t.Setenv(envnames.AutoPin, tc.pin)
				call()
				argv := read()
				if len(argv) != 1 || argv[0] == "" {
					t.Fatalf("want one ssh call, have %q", argv)
				}
				got := strings.Contains(argv[0], "StrictHostKeyChecking")
				if got != tc.want {
					t.Errorf("StrictHostKeyChecking on the command line = %v, want %v: %s", got, tc.want, argv[0])
				}
				if tc.want && !strings.Contains(argv[0], "StrictHostKeyChecking=accept-new") {
					t.Errorf("auto-pin on should keep accept-new: %s", argv[0])
				}
			})
		}
	}
}
