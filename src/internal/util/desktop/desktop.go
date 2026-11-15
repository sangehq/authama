// Package desktop posts a best-effort desktop notification through
// github.com/simtabi/go-notifykit, so a banner says "authama" with authama's
// icon instead of "Script Editor", and nothing reaches the screen under
// `go test` or in CI. Notify keeps its old contract: false means nothing was
// delivered.
package desktop

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	notifykit "github.com/simtabi/go-notifykit/src"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/util/envnames"
)

// AppID is the identifier macOS lists authama's notifications under
// (System Settings > Notifications), with ".notifier" appended by notifykit.
const AppID = "com.simtabi.authama"

// NewNotifier returns the notifier every authama surface shares: the one-shot
// alert, `authama notify send|ping|doctor`, and the listener `authama serve`
// runs. AUTHAMA_NOTIFICATIONS=off turns it off, as it always has. go-notifykit
// reads its own switches too (the EnvPrefix below, then NOTIFYKIT_DESKTOP); they
// are documented in docs/tools/expiry.md rather than in envnames, which lists
// only what authama itself reads.
func NewNotifier() (*notifykit.Notifier, error) {
	return notifykit.New(notifykit.Config{
		AppName:     brand.Command,
		AppID:       AppID,
		Icon:        IconPNG(),
		EnvPrefix:   "AUTHAMA",
		Disabled:    off(envnames.Get(nil, envnames.Notifications)),
		MinInterval: 30 * time.Second,
	})
}

func off(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// Notify posts a desktop notification. Returns true if it was delivered, or
// will be once the user answers macOS's first-run permission prompt.
func Notify(title, message string) bool {
	n, err := NewNotifier()
	if err != nil {
		return legacyNotify(title, message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := n.Send(ctx, notification(title, message))
	return err == nil && res.Backend != ""
}

// notification is what Notify sends. Its Group is the title, so the same
// event posted again replaces its banner instead of stacking another (one
// banner per event, as go-notifykit's standard asks).
func notification(title, message string) notifykit.Notification {
	return notifykit.Notification{Title: title, Body: message, Group: title}
}

// legacyNotify is the notifier authama shipped before go-notifykit, kept as the
// fallback for a notifier that cannot be configured. Its banners on macOS say
// "Script Editor".
//
// Deprecated: superseded by NewNotifier. Removable once a release has shipped
// with go-notifykit and no fallback has been reported, no earlier than 0.2.0.
func legacyNotify(title, message string) bool {
	switch runtime.GOOS {
	case "darwin":
		if has("terminal-notifier") {
			_ = exec.Command("terminal-notifier", "-title", title, "-message", message).Run()
			return true
		}
		if has("osascript") {
			script := "display notification " + appleQuote(message) + " with title " + appleQuote(title)
			_ = exec.Command("osascript", "-e", script).Run()
			return true
		}
		return false
	case "linux":
		if !has("notify-send") {
			return false
		}
		body := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(message)
		_ = exec.Command("notify-send", "--", title, body).Run()
		return true
	case "windows":
		if !has("powershell") {
			return false
		}
		script := "Add-Type -AssemblyName System.Windows.Forms;" +
			"$n=New-Object System.Windows.Forms.NotifyIcon;" +
			"$n.Icon=[System.Drawing.SystemIcons]::Information;$n.Visible=$true;" +
			"$n.ShowBalloonTip(10000," + psQuote(title) + "," + psQuote(message) +
			",[System.Windows.Forms.ToolTipIcon]::Info)"
		_ = exec.Command("powershell", "-NoProfile", "-Command", script).Run()
		return true
	default:
		return false
	}
}

func has(name string) bool { _, err := exec.LookPath(name); return err == nil }

// appleQuote quotes a string as an AppleScript double-quoted literal.
func appleQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// psQuote quotes a string as a PowerShell single-quoted literal (doubling quotes).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
