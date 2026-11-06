//go:build darwin

package owner

import (
	"context"
	"strings"
	"testing"
)

// fakeTouchID stands in for the helper: it records the arguments and answers
// with code.
func fakeTouchID(t *testing.T, found bool, code int) *[][]string {
	t.Helper()
	var calls [][]string
	prevPath, prevRun := touchIDHelperPath, runStatus
	touchIDHelperPath = func() (string, bool) { return "/opt/authama/authama-touchid", found }
	runStatus = func(_ context.Context, name string, args, _ []string) (int, error) {
		calls = append(calls, append([]string{name}, args...))
		return code, nil
	}
	t.Cleanup(func() { touchIDHelperPath, runStatus = prevPath, prevRun })
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	return &calls
}

func TestTouchIDIsOfferedOnlyWhenTheHelperSaysSo(t *testing.T) {
	calls := fakeTouchID(t, true, 0)
	if ok, why := BiometricAvailable(); !ok {
		t.Fatalf("unavailable: %s", why)
	}
	if got := strings.Join((*calls)[0], " "); got != "/opt/authama/authama-touchid --check" {
		t.Fatalf("availability ran %q", got)
	}
	if err := VerifyBiometric(context.Background()); err != nil {
		t.Fatal(err)
	}

	for code, want := range map[int]string{2: "no Touch ID", 3: "no finger is enrolled", 4: "cancelled", 5: "locked"} {
		fakeTouchID(t, true, code)
		if ok, why := BiometricAvailable(); ok || !strings.Contains(why, want) {
			t.Errorf("status %d: ok=%v %q, want %q", code, ok, why, want)
		}
		if err := VerifyBiometric(context.Background()); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d: verify %v", code, err)
		}
	}

	fakeTouchID(t, false, 0)
	if ok, why := BiometricAvailable(); ok || !strings.Contains(why, "authama-touchid is not installed") {
		t.Errorf("no helper: %v %q", ok, why)
	}

	calls = fakeTouchID(t, true, 0)
	t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
	if ok, why := BiometricAvailable(); ok || !strings.Contains(why, "SSH") || len(*calls) != 0 {
		t.Errorf("over SSH: %v %q, %d helper runs", ok, why, len(*calls))
	}
}
