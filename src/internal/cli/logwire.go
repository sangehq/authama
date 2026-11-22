package cli

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sangehq/authama/src/internal/services/browsers"
	"github.com/sangehq/authama/src/internal/util/applog"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// tzFlag shows one command's times in another zone than display.timezone.
const tzFlag = "tz"

// applyDisplayZone sets the zone times are shown in: --tz, else
// display.timezone (or $AUTHAMA_TIMEZONE), else this system's. A bad --tz is
// an error; a bad setting is named and this system's zone is used.
func applyDisplayZone(c *cobra.Command) error {
	if f := c.Flags().Lookup(tzFlag); f != nil && f.Changed {
		return tz.Pin(f.Value.String())
	}
	if err := tz.Set(envnames.Get(nil, envnames.Timezone)); err != nil {
		notice(c.ErrOrStderr(), "display.timezone: %v; showing this system's zone", err)
		_ = tz.Set("")
	}
	return nil
}

func settingInt(name string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(envnames.Get(nil, name)))
	if err != nil || n < 1 {
		return def
	}
	return n
}

// configureLog points authama.log at the home's log directory, at the level
// and retention its settings name.
func configureLog(p paths.Paths) {
	level := envnames.Get(nil, envnames.LogLevel)
	if level == "" {
		level = applog.DefaultLevel
	}
	applog.Configure(applog.Config{
		Dir:           p.LogDir(),
		Level:         applog.ParseLevel(level),
		RetentionDays: settingInt(envnames.LogRetentionDays, applog.DefaultRetentionDays),
		MaxSizeMB:     settingInt(envnames.LogMaxSizeMB, applog.DefaultMaxSizeMB),
	})
}

// commandPath is a command's words under the root: "ssh host add".
func commandPath(c *cobra.Command) string {
	if c == nil {
		return ""
	}
	path := strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), c.Root().Name()))
	if path == "" {
		return "(bare)"
	}
	return path
}

// holdsSecrets reports whether a command's arguments may be secret values
// (a token's value, a PIN), so none of them is logged.
func holdsSecrets(path string) bool {
	for _, w := range []string{"token", "auth", "owner", "secret", "setup import", "api", "webhook", "askpass", "pin"} {
		if strings.Contains(path, w) {
			return true
		}
	}
	return false
}

// logCommand records a finished command in authama.log: its words, the
// flags it was given (names only), its arguments unless they may hold a
// secret, how long it took and how it ended.
func logCommand(c *cobra.Command, args, raw []string, took time.Duration, err error) {
	if c == nil {
		return
	}
	path := commandPath(c)
	var flags []string
	c.Flags().Visit(func(f *pflag.Flag) { flags = append(flags, f.Name) })
	attrs := []any{"area", "cli", "command", path, "duration_ms", took.Milliseconds(), "pid", os.Getpid()}
	if len(flags) > 0 {
		attrs = append(attrs, "flags", flags)
	}
	if len(args) > 0 && !holdsSecrets(path) {
		attrs = append(attrs, "args", applog.Args(args))
	}
	if err != nil {
		msg := err.Error()
		if holdsSecrets(path) {
			msg = withoutArgs(msg, path, raw)
		}
		applog.Warn("command failed", append(attrs, "error", msg)...)
		return
	}
	applog.Info("command", attrs...)
}

// rememberFor is how long a browser stays signed in after a sign-in link:
// web.remember_days, 30 by default; 0 turns remembering off.
func rememberFor() time.Duration {
	days := browsers.DefaultDays
	if v := strings.TrimSpace(envnames.Get(nil, envnames.WebRememberDays)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			days = n
		}
	}
	return time.Duration(days) * 24 * time.Hour
}

// executeLogged runs root on args and records the command it ran in
// authama.log.
func executeLogged(root *cobra.Command, args []string) error {
	root.SetArgs(args)
	start := time.Now()
	ran, err := root.ExecuteC()
	if ran != nil && !skipsStartup(ran) {
		logCommand(ran, ran.Flags().Args(), args, time.Since(start), err)
	}
	return err
}

// withoutArgs blanks, in a command's error, every word it was given that is
// not one of its command words: cobra quotes an argument back ("unknown
// command \"<it>\""), and for a token or a PIN that argument is the secret.
func withoutArgs(msg, path string, raw []string) string {
	words := map[string]bool{}
	for _, w := range strings.Fields(path) {
		words[w] = true
	}
	for _, a := range raw {
		_, v, hasValue := strings.Cut(a, "=")
		if strings.HasPrefix(a, "-") {
			if !hasValue {
				continue
			}
			a = v
		}
		if len(a) < 3 || words[a] {
			continue
		}
		msg = strings.ReplaceAll(msg, a, applog.Redacted)
	}
	return msg
}
