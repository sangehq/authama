//go:build windows

package owner

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"time"
	"unicode/utf16"

	"github.com/simtabi/authama/src/internal/util/envnames"
)

// helloScript asks Windows Hello through WinRT's UserConsentVerifier, from
// Windows PowerShell 5.1 (PowerShell 7 has no built-in WinRT projection), so
// authama needs no compiled helper. With AUTHAMA_HELLO_CHECK set it only
// checks availability. Exit status: the UserConsentVerifierAvailability
// value for a check (0 Available), otherwise 10 + that value when it is not
// available, or the UserConsentVerificationResult (0 Verified).
const helloScript = `$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$V = [Windows.Security.Credentials.UI.UserConsentVerifier,Windows.Security.Credentials.UI,ContentType=WindowsRuntime]
$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' } | Select-Object -First 1
function Await($op, [Type]$t) { $task = $asTask.MakeGenericMethod($t).Invoke($null, @($op)); $null = $task.Wait(-1); $task.Result }
$a = Await ($V::CheckAvailabilityAsync()) ([Windows.Security.Credentials.UI.UserConsentVerifierAvailability])
if ($env:AUTHAMA_HELLO_CHECK) { exit [int]$a }
if ([int]$a -ne 0) { exit (10 + [int]$a) }
$r = Await ($V::RequestVerificationAsync('Unlock authama')) ([Windows.Security.Credentials.UI.UserConsentVerificationResult])
exit [int]$r
`

// encodedCommand is script as powershell.exe's -EncodedCommand takes it:
// base64 of its UTF-16LE bytes.
func encodedCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// helloReason maps an availability value (0-4) or a verification result
// (0-6) to why Windows Hello did not unlock.
func helloReason(code int, check bool) string {
	if code >= 10 {
		code, check = code-10, true
	}
	switch {
	case code == 1:
		return "no Windows Hello device is present"
	case code == 2:
		return "Windows Hello is not set up for this account; set it up in Settings > Accounts > Sign-in options"
	case code == 3:
		return "Windows Hello is turned off by policy"
	case code == 4:
		return "the Windows Hello device is busy"
	case code == 5 && !check:
		return "Windows Hello gave up after too many tries"
	case code == 6 && !check:
		return "Windows Hello was cancelled"
	}
	return fmt.Sprintf("Windows Hello did not verify (status %d)", code)
}

func powershell() (string, bool) {
	p, err := exec.LookPath("powershell.exe")
	return p, err == nil
}

func helloArgs() []string {
	return []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodedCommand(helloScript)}
}

// BiometricAvailable reports whether Windows Hello can stand in for the PIN:
// not an SSH session, Windows PowerShell is there, and Hello is set up.
func BiometricAvailable() (bool, string) {
	if remoteSession() {
		return false, "this is an SSH session, where Windows Hello cannot be asked for"
	}
	ps, ok := powershell()
	if !ok {
		return false, "Windows PowerShell (powershell.exe) was not found"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	code, err := runStatus(ctx, ps, helloArgs(), []string{envnames.HelloCheck + "=1"})
	if err != nil {
		return false, "Windows Hello could not be checked: " + err.Error()
	}
	if code != 0 {
		return false, helloReason(code, true)
	}
	return true, ""
}

// VerifyBiometric asks Windows Hello, for up to a minute. The prompt can open
// behind the terminal; it is in the taskbar when it does.
func VerifyBiometric(ctx context.Context) error {
	ps, ok := powershell()
	if !ok {
		return errors.New("powershell.exe (Windows PowerShell) was not found")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	code, err := runStatus(ctx, ps, helloArgs(), nil)
	if err != nil {
		return fmt.Errorf("no answer from Windows Hello: %w", err)
	}
	if code != 0 {
		return errors.New(helloReason(code, false))
	}
	return nil
}
