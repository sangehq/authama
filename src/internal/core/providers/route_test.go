package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// argvStub puts an ssh and an ssh-copy-id on PATH that log their argv, one
// line per argument, and return what readArgv reads back.
func argvStub(t *testing.T) (readArgv func() []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	logf := filepath.Join(dir, "argv.txt")
	stub := "#!/bin/sh\nprintf '%s\\n' \"--call\" >> " + logf + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + logf + "; done\ncat > /dev/null\n"
	for _, name := range []string{"ssh", "ssh-copy-id"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		b, _ := os.ReadFile(logf)
		_ = os.Remove(logf)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
}

// A routed target reaches the server as `ssh <alias>` does: -F the route
// config, the alias after "--", and no -p (the config has the port). deploy,
// verify, remove and the authorized_keys read all go that way, and deploy
// uses plain ssh, since an older ssh-copy-id has no -F.
func TestARoutedTargetGoesThroughItsAlias(t *testing.T) {
	read := argvStub(t)
	tgt := Target{Alias: "app-web", Hostname: "10.0.0.5", User: "deploy", Port: 2222,
		PubkeyPath: "/k.pub", PubkeyText: "ssh-ed25519 " + targetBody + " me@h", IdentityPath: "/k",
		ConfigFile: "/state/route.conf"}
	g := GenericSSH{}
	check := func(what string, argv []string) {
		t.Helper()
		joined := strings.Join(argv, " ")
		if argv[0] != "--call" || argv[1] == "" {
			t.Fatalf("%s: no call: %q", what, argv)
		}
		if !strings.Contains(joined, "-F /state/route.conf") || !strings.Contains(joined, "-- app-web") {
			t.Errorf("%s does not go through the alias: %s", what, joined)
		}
		if strings.Contains(joined, "10.0.0.5") || strings.Contains(joined, "-p 2222") {
			t.Errorf("%s still names the hostname or port: %s", what, joined)
		}
	}
	g.Deploy(tgt)
	check("deploy", read())
	g.Verify(tgt)
	check("verify", read())
	g.Remove(tgt)
	check("remove", read())
	g.ListDeployed(tgt)
	check("list", read())
}

// The direct route (no ConfigFile) is the old command line: user@hostname
// after "--", and -p for a port other than 22.
func TestADirectTargetConnectsToItsHostname(t *testing.T) {
	read := argvStub(t)
	tgt := Target{Alias: "app-web", Hostname: "10.0.0.5", User: "deploy", Port: 2222,
		PubkeyPath: "/k.pub", PubkeyText: "ssh-ed25519 " + targetBody + " me@h", IdentityPath: "/k"}
	(GenericSSH{}).Verify(tgt)
	joined := strings.Join(read(), " ")
	if !strings.Contains(joined, "-p 2222") || !strings.Contains(joined, "-- deploy@10.0.0.5") || strings.Contains(joined, "-F ") {
		t.Errorf("direct verify: %s", joined)
	}
}

// An alias that could read as an option is refused like any destination.
func TestARoutedAliasIsStillChecked(t *testing.T) {
	read := argvStub(t)
	tgt := Target{Alias: "-oProxyCommand=x", Hostname: "h", User: "u", Port: 22, IdentityPath: "/k", ConfigFile: "/c"}
	if (GenericSSH{}).Verify(tgt) {
		t.Fatal("a dash alias verified")
	}
	if argv := read(); len(argv) > 1 {
		t.Errorf("ssh ran for a dash alias: %q", argv)
	}
}
