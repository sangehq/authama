//go:build windows

package owner

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTheHelloScriptIsEncodedAsPowerShellReadsIt(t *testing.T) {
	b, err := base64.StdEncoding.DecodeString(encodedCommand(helloScript))
	if err != nil || len(b)%2 != 0 {
		t.Fatalf("not base64 of UTF-16: %v", err)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	if string(utf16.Decode(u)) != helloScript {
		t.Fatal("the script does not round-trip")
	}
	if !strings.Contains(helloScript, "IAsyncOperation`1") {
		t.Fatal("the generic type name lost its backtick")
	}
}

func TestWindowsHelloMapsEachStatus(t *testing.T) {
	var env [][]string
	prev := runStatus
	t.Cleanup(func() { runStatus = prev })
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	code := 0
	runStatus = func(_ context.Context, _ string, _, e []string) (int, error) {
		env = append(env, e)
		return code, nil
	}
	if _, ok := powershell(); !ok {
		t.Skip("no powershell.exe")
	}
	if ok, why := BiometricAvailable(); !ok {
		t.Fatalf("unavailable: %s", why)
	}
	if len(env) != 1 || len(env[0]) != 1 || env[0][0] != "AUTHAMA_HELLO_CHECK=1" {
		t.Fatalf("the check did not ask for availability only: %v", env)
	}
	for c, want := range map[int]string{2: "not set up", 3: "policy", 12: "not set up", 6: "cancelled", 5: "too many"} {
		code = c
		if err := VerifyBiometric(context.Background()); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d: %v, want %q", c, err, want)
		}
	}
	t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
	if ok, why := BiometricAvailable(); ok || !strings.Contains(why, "SSH") {
		t.Errorf("over SSH: %v %q", ok, why)
	}
}
