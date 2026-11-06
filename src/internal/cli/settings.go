package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/services/change"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/util/envnames"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/web"
)

// settingsGroup is the user-settings command group. It is not `config`: that
// word is the SSH config's (`ssh config render|check|show|export`, and its
// hidden top-level spelling), and one word meaning two things is how a
// `config show` ends up printing the wrong file.
const settingsGroup = "settings"

func newSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   settingsGroup,
		Short: "Your settings files (.env and authama.json): where they are, what is in effect, validate, create",
		Long: "authama reads two files from its config home at start-up:\n\n" +
			"  .env           KEY=value lines: tokens, and any AUTHAMA_* setting\n" +
			"  authama.json   settings, checked against a schema (" + settings.SchemaURL + ")\n\n" +
			"Precedence: command-line flags > environment > .env > authama.json > defaults.\n" +
			"A variable already in the environment is never replaced by a file; --no-dotenv\n" +
			"or $" + envnames.NoDotenv + "=1 skips .env for a run.",
		Args: cobra.ArbitraryArgs,
		RunE: groupHelp,
	}
	cmd.AddCommand(newSettingsPathCmd(), newSettingsShowCmd(), newSettingsValidateCmd(), newSettingsInitCmd())
	return cmd
}

type settingsLocation struct {
	ConfigHome   string `json:"config_home"`
	ConfigSource string `json:"config_source"`
	StateHome    string `json:"state_home"`
	Dotenv       string `json:"dotenv"`
	DotenvState  string `json:"dotenv_state"`
	Settings     string `json:"settings"`
	SettingsOK   string `json:"settings_state"`
}

func locate(c *cobra.Command) (settingsLocation, paths.Paths, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return settingsLocation{}, p, err
	}
	return locateAt(p), p, nil
}

// locateAt is locate for a home already resolved: what `settings path`
// prints, and what the web UI's settings page shows.
func locateAt(p paths.Paths) settingsLocation {
	loc := settingsLocation{ConfigHome: p.ConfigDir, StateHome: p.State(), Dotenv: p.EnvFile(), Settings: p.SettingsFile()}
	switch _, from := envnames.Source(nil, envnames.ConfigDir); {
	case p.IsDev():
		loc.ConfigSource = "--dev-root sandbox"
	case from != "":
		loc.ConfigSource = "$" + from
	default:
		loc.ConfigSource = "default"
	}
	st := currentStartup()
	switch {
	case !exists(p.EnvFile()):
		loc.DotenvState = "absent"
	case st != nil && st.DotenvOff != "":
		loc.DotenvState = "present, skipped (" + st.DotenvOff + ")"
	case st != nil && st.Dotenv.Refused != "":
		loc.DotenvState = "present, NOT loaded: readable by other users"
	case st != nil:
		loc.DotenvState = fmt.Sprintf("loaded (%d set, %d already in the environment)", len(st.Dotenv.Set), len(st.Dotenv.Kept))
	default:
		loc.DotenvState = "present"
	}
	if _, err := settings.LoadFile(p.SettingsFile()); err != nil {
		loc.SettingsOK = "INVALID (authama settings validate)"
	} else if exists(p.SettingsFile()) {
		loc.SettingsOK = "valid"
	} else {
		loc.SettingsOK = "absent (authama settings init creates one)"
	}
	return loc
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func newSettingsPathCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print where the config home, state home, .env and authama.json are",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			loc, _, err := locate(c)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if jsonOut {
				return writeJSONTo(out, loc)
			}
			_, _ = fmt.Fprintf(out, "config home:   %s  (%s)\n", loc.ConfigHome, loc.ConfigSource)
			_, _ = fmt.Fprintf(out, "state home:    %s  (history, logs, snapshots, bundles, lock)\n", loc.StateHome)
			_, _ = fmt.Fprintf(out, ".env:          %s  (%s)\n", loc.Dotenv, loc.DotenvState)
			_, _ = fmt.Fprintf(out, "authama.json:  %s  (%s)\n", loc.Settings, loc.SettingsOK)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output (scripting)")
	return cmd
}

type effectiveRow struct {
	Key      string `json:"key"`
	Variable string `json:"variable"`
	Value    string `json:"value"`
	Source   string `json:"source"`
}

func effective() []effectiveRow {
	st := currentStartup()
	in := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	var rows []effectiveRow
	for _, s := range settings.Specs {
		v, from := envnames.Source(nil, s.Env)
		row := effectiveRow{Key: s.Key(), Variable: s.Env, Value: v}
		switch {
		case from == "":
			row.Source = "default"
			row.Value = s.Default
			if s.Env == envnames.StateDir && st != nil {
				row.Value = st.Paths.State()
			}
		case st == nil || st.Preset[from]:
			row.Source = "environment ($" + from + ")"
		case in(st.Dotenv.Set, from):
			row.Source = ".env ($" + from + ")"
		case in(st.Applied.Set, from):
			row.Source = "authama.json"
		default:
			row.Source = "$" + from
		}
		rows = append(rows, row)
	}
	return rows
}

func newSettingsShowCmd() *cobra.Command {
	var eff, jsonOut bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print authama.json, or with --effective every setting's value and where it came from",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if eff {
				rows := effective()
				if jsonOut {
					return writeJSONTo(out, rows)
				}
				w := 0
				for _, r := range rows {
					w = max(w, len(r.Key))
				}
				for _, r := range rows {
					val := r.Value
					if val == "" {
						val = "(unset)"
					}
					_, _ = fmt.Fprintf(out, "%-*s  %s  [%s]\n", w, r.Key, val, r.Source)
				}
				return nil
			}
			b, err := os.ReadFile(p.SettingsFile())
			if errors.Is(err, os.ErrNotExist) {
				_, _ = fmt.Fprintf(out, "no %s (every setting is at its default unless the environment or .env sets it;\n"+
					"`%s settings show --effective` lists them, `%s settings init` creates the file)\n",
					p.SettingsFile(), brand.Command, brand.Command)
				return nil
			}
			if err != nil {
				return err
			}
			_, _ = out.Write(b)
			return nil
		},
	}
	cmd.Flags().BoolVar(&eff, "effective", false, "every setting with its value in effect and its source")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output (with --effective)")
	return cmd
}

func newSettingsValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [FILE]",
		Short: "Check authama.json (or FILE) against the schema, and .env's syntax and permissions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			out := c.OutOrStdout()
			var problems []string
			if len(args) == 1 {
				problems = fileProblems(args[0], true)
			} else {
				p, err := resolvePaths(c)
				if err != nil {
					return err
				}
				problems = settingsProblems(p)
			}
			if len(problems) == 0 {
				_, _ = fmt.Fprintln(out, "settings: ok")
				return nil
			}
			for _, pr := range problems {
				_, _ = fmt.Fprintln(out, pr)
			}
			return errNotClean
		},
	}
	return cmd
}

// settingsProblems is `settings validate` with no FILE: .env's syntax and
// permissions, then authama.json against the schema. An absent authama.json
// is not a problem.
func settingsProblems(p paths.Paths) []string {
	return append(dotenvProblems(p.EnvFile()), fileProblems(p.SettingsFile(), false)...)
}

// fileProblems checks one settings file against the schema. named is true
// when the file was given on the command line, where a missing file is an
// error rather than "every setting at its default".
func fileProblems(target string, named bool) []string {
	var problems []string
	if b, err := os.ReadFile(target); err == nil {
		if _, err := settings.Parse(target, b); err != nil {
			var ve *settings.ValidationError
			if errors.As(err, &ve) {
				for _, pr := range ve.Problems {
					problems = append(problems, filepath.Base(target)+": "+pr)
				}
			} else {
				problems = append(problems, err.Error())
			}
		}
	} else if named || !errors.Is(err, os.ErrNotExist) {
		problems = append(problems, err.Error())
	}
	return problems
}

// dotenvProblems checks a .env without loading it.
func dotenvProblems(path string) []string {
	var out []string
	r := settings.LoadDotenv(path, settings.Env{
		Lookup: func(string) (string, bool) { return "", true }, // everything "set": load nothing
		Setenv: func(string, string) error { return nil },
	})
	if r.Refused != "" {
		out = append(out, r.Refused)
	}
	out = append(out, r.Warnings...)
	return out
}

func newSettingsInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter authama.json (schema reference and version; every setting at its default)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			return change.SettingsInit(force).Run(p, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "replace an existing authama.json")
	return cmd
}

func writeJSONTo(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// settingsSummary is doctor's view of the settings files: lines to print and
// whether any is a failure.
func settingsSummary(p paths.Paths) (lines []string, failed bool) {
	lines = append(lines, "config home: "+p.ConfigDir, "state home:  "+p.State())
	st := currentStartup()
	switch {
	case !exists(p.EnvFile()):
		lines = append(lines, ".env: absent")
	case st != nil && st.DotenvOff != "":
		lines = append(lines, ".env: skipped ("+st.DotenvOff+")")
	case st != nil && st.Dotenv.Refused != "":
		lines = append(lines, ".env: NOT loaded - readable by other users (doctor --fix tightens it)")
	case st != nil:
		lines = append(lines, fmt.Sprintf(".env: loaded (%d set, %d already in the environment)", len(st.Dotenv.Set), len(st.Dotenv.Kept)))
	default:
		lines = append(lines, ".env: present")
	}
	if _, err := settings.LoadFile(p.SettingsFile()); err != nil {
		failed = true
		lines = append(lines, "authama.json: INVALID - "+strings.ReplaceAll(err.Error(), "\n", "\n  "))
	} else if exists(p.SettingsFile()) {
		lines = append(lines, "authama.json: valid")
	} else {
		lines = append(lines, "authama.json: absent")
	}
	if l := p.FirstLegacyHome(); l != "" && exists(p.ConfigDir) {
		lines = append(lines, "legacy home not migrated: "+l+" (`"+brand.Command+" migrate --force` copies it over the current one)")
	}
	if st != nil && len(st.Deprecated) > 0 {
		lines = append(lines, "deprecated variable names in use: "+strings.Join(st.Deprecated, ", "))
	}
	return lines, failed
}

// settingsView is the web UI's settings page: `settings path`, `show
// --effective` and `validate`, from the same functions those commands use.
func settingsView(p paths.Paths) web.SettingsView {
	loc := locateAt(p)
	v := web.SettingsView{
		ConfigHome: loc.ConfigHome, ConfigSource: loc.ConfigSource, StateHome: loc.StateHome,
		Dotenv: loc.Dotenv, DotenvState: loc.DotenvState, Settings: loc.Settings, SettingsState: loc.SettingsOK,
		HasSettings: exists(p.SettingsFile()), Problems: settingsProblems(p),
	}
	for _, r := range effective() {
		v.Effective = append(v.Effective, web.SettingRow{Key: r.Key, Variable: r.Variable, Value: r.Value, Source: r.Source})
	}
	return v
}
