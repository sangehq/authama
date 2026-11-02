package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/hypervisor"
	"github.com/simtabi/authama/src/internal/services/migratesvc"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/util/envnames"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// noDotenvFlag skips <config home>/.env for one run, like $AUTHAMA_NO_DOTENV.
const noDotenvFlag = "no-dotenv"

// runStartup turns the start-up work off for the package tests, which build
// command trees by the hundred and must never read - or migrate - the real
// user's home. The tests that are about start-up turn it back on.
var runStartup = true

// startupState is what start-up found, for `settings` and doctor to report.
type startupState struct {
	Paths paths.Paths
	// Preset lists the setting variables (any spelling) already in the
	// environment before the files were read.
	Preset      map[string]bool
	Migrated    string
	Relocated   []string
	Conflicts   []string
	DotenvOff   string // why .env was skipped, if it was
	Dotenv      settings.DotenvResult
	Settings    *settings.File
	SettingsErr error
	Applied     settings.ApplyResult
	Deprecated  []string
}

var (
	startupMu sync.Mutex
	lastStart *startupState
)

func currentStartup() *startupState {
	startupMu.Lock()
	defer startupMu.Unlock()
	return lastStart
}

// startup runs once per invocation, before any command: migrate a legacy home
// on first run, read .env and authama.json into the environment, move the
// tool's state out of the config home, and name any deprecated variable.
//
// Commands that only describe the binary (help, version, completion) skip it.
// An invalid authama.json stops every command except the ones that diagnose
// it (settings, doctor, init), with the problems listed.
func startup(c *cobra.Command, _ []string) error {
	if !runStartup || skipsStartup(c) {
		return nil
	}
	st := &startupState{Preset: map[string]bool{}}
	for _, n := range envnames.AllNames() {
		if _, ok := os.LookupEnv(n); ok {
			st.Preset[n] = true
		}
	}
	errw := c.ErrOrStderr()
	p, err := quietPaths(c)
	if err != nil {
		return nil // the command resolves the paths itself and reports this
	}
	// A home named explicitly is the user's choice of folder, not the default
	// one migration fills: it is never filled from a legacy home on its own.
	// (`authama migrate` is there if they want it.)
	if !envnames.IsSet(os.LookupEnv, envnames.ConfigDir) {
		st.Migrated = autoMigrate(errw, p)
	}
	if why := dotenvOff(c); why != "" {
		st.DotenvOff = why
	} else {
		st.Dotenv = settings.LoadDotenv(p.EnvFile(), settings.Env{})
		if st.Dotenv.Refused != "" {
			notice(errw, "%s", st.Dotenv.Refused)
		}
		for _, w := range st.Dotenv.Warnings {
			notice(errw, "warning: %s", w)
		}
	}
	st.Settings, st.SettingsErr = settings.LoadFile(p.SettingsFile())
	if st.SettingsErr == nil {
		st.Applied = st.Settings.Apply(settings.Env{})
		if st.Settings != nil {
			hypervisor.Register(st.Settings.VM.Hypervisors)
		}
	}
	// The state home may have come from one of the files.
	if p2, err := quietPaths(c); err == nil {
		p = p2
	}
	st.Paths = p
	if moved, conflicts, err := migratesvc.RelocateState(p); err != nil {
		notice(errw, "could not move state to %s: %v", p.State(), err)
	} else {
		st.Relocated, st.Conflicts = moved, conflicts
		if len(moved) > 0 {
			notice(errw, "moved %s from %s to the state home %s", strings.Join(moved, ", "), p.ConfigDir, p.State())
		}
		for _, c := range conflicts {
			notice(errw, "warning: %s was left in place: the state home already has one (compare and remove the older)", c)
		}
	}
	if st.Deprecated = envnames.Deprecated(nil); len(st.Deprecated) > 0 {
		notice(errw, "deprecated variable names, still honored: %s - rename them (docs/configuration.md)",
			strings.Join(st.Deprecated, ", "))
	}
	startupMu.Lock()
	lastStart = st
	startupMu.Unlock()
	if st.SettingsErr != nil && !toleratesBadSettings(c) {
		return st.SettingsErr
	}
	return nil
}

func autoMigrate(errw io.Writer, p paths.Paths) string {
	res, did, err := migratesvc.AutoMigrate(p)
	switch {
	case err != nil:
		notice(errw, "could not migrate your old settings: %v", err)
	case did:
		notice(errw, "first run under the new name: %s", res.Message)
		return res.Message
	}
	return ""
}

func notice(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, brand.Command+": "+format+"\n", a...)
}

// dotenvOff says why .env is skipped this run, or "".
func dotenvOff(c *cobra.Command) string {
	if v, err := c.Root().PersistentFlags().GetBool(noDotenvFlag); err == nil && v {
		return "--" + noDotenvFlag
	}
	if settings.Truthy(os.Getenv(envnames.NoDotenv)) {
		return "$" + envnames.NoDotenv
	}
	return ""
}

func skipsStartup(c *cobra.Command) bool {
	for x := c; x != nil; x = x.Parent() {
		switch x.Name() {
		case "help", "version", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		}
	}
	return false
}

func toleratesBadSettings(c *cobra.Command) bool {
	for x := c; x != nil; x = x.Parent() {
		switch x.Name() {
		case settingsGroup, "doctor", "init":
			return x.Parent() != nil && !x.Parent().HasParent()
		}
	}
	return false
}
