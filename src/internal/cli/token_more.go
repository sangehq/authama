package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/simtabi/ssh-manager/src/v3/internal/brand"
	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/mutation"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/tokensvc"
)

// The token commands that read one entry, the catalog or the state of the
// whole setup, and the one that imports a config: show, status, providers
// and import.

// --- show ----------------------------------------------------------------

func newTokenShowCmd() *cobra.Command {
	var project string
	var asJSON, reveal, yes bool
	cmd := &cobra.Command{
		Use:   "show TARGET HOST",
		Short: "Everything about one installed token: tool, host, kind, user and file (secret masked)",
		Long: "Prints one entry of `token list` in full. The secret stays masked unless\n" +
			"--reveal is given, which asks first at a terminal and needs --yes without one.",
		Example: "  " + brand.Command + " token show npm registry.npmjs.org\n" +
			"  " + brand.Command + " token show git github.com --json",
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			target := args[0]
			if err := checkTarget(target); err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			host := tokens.CanonicalHost(target, args[1])
			it, ok := env.Find(target, host, project)
			if !ok {
				return fmt.Errorf("no token found for %s: %s (see `%s token list`)", target, host, brand.Command)
			}
			reveal = mayReveal(c, reveal, yes)
			if !reveal {
				it.Secret = ""
			}
			if asJSON {
				b, _ := json.MarshalIndent(it, "", "  ")
				say(c, "%s", b)
				return nil
			}
			secret := "—"
			switch {
			case reveal:
				secret = emDash(it.Secret)
			case it.SecretPresent:
				secret = "•••••••• (set; --reveal to print it)"
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, row := range [][2]string{
				{"tool", it.Target}, {"host", it.Host}, {"what", it.Description}, {"kind", it.Kind},
				{"user", emDash(it.Username)}, {"secret", secret}, {"file", it.File},
			} {
				_, _ = fmt.Fprintf(tw, "%s:\t%s\n", row[0], row[1])
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "read the project's auth.json or .npmrc instead of the user one")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the entry as JSON (the same fields as token list --json)")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show the secret value (asks first; --yes when not at a terminal)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "reveal without asking")
	return cmd
}

// --- status --------------------------------------------------------------

// tokenStatus is `token status --json`; its field names are part of the
// machine-readable surface and do not change.
type tokenStatus struct {
	Config   tokenStatusConfig   `json:"config"`
	Targets  []tokenStatusTarget `json:"targets"`
	Warnings []string            `json:"warnings"`
}

type tokenStatusConfig struct {
	Path    string `json:"path"`
	Builtin bool   `json:"builtin"`
	Legacy  bool   `json:"legacy"`
}

type tokenStatusTarget struct {
	Target     string `json:"target"`
	File       string `json:"file"`
	Exists     bool   `json:"exists"`
	Mode       string `json:"mode"`
	Installed  int    `json:"installed"`
	Configured int    `json:"configured"`
	Resolved   int    `json:"resolved"`
}

func newTokenStatusCmd() *cobra.Command {
	var config, project string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Which config is in use, where each tool's file is, and which variables are missing; write nothing",
		Long: "A read-only overview: the token config that would be used (and why), each tool's\n" +
			"credential file with its mode, how many tokens it holds, how many entries the\n" +
			"config has for it and how many of those resolve from the environment now. The\n" +
			"warnings name the entries `token apply` would skip, and why (usually an unset\n" +
			"variable). It always exits 0; `token audit` is the check that fails.",
		Example: "  " + brand.Command + " token status\n" +
			"  " + brand.Command + " token status --project . --json",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			st, err := buildTokenStatus(env, config, project)
			if err != nil {
				return err
			}
			if asJSON {
				b, _ := json.MarshalIndent(st, "", "  ")
				say(c, "%s", b)
				return nil
			}
			return printTokenStatus(c.OutOrStdout(), st)
		},
	}
	cmd.Flags().StringVar(&config, "config", "", "the token config file (default: discovered)")
	cmd.Flags().StringVar(&project, "project", "", "report the project's auth.json and .npmrc instead of the user ones")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output (scripting)")
	return cmd
}

// buildTokenStatus gathers `token status`: the config that would be used,
// and per tool its file, mode, installed count and config entries.
func buildTokenStatus(env tokensvc.Env, config, project string) (tokenStatus, error) {
	cfg, src, err := env.ResolveConfig(config)
	if err != nil {
		return tokenStatus{}, err
	}
	st := tokenStatus{
		Config:   tokenStatusConfig{Path: src.Path, Builtin: src.Path == "", Legacy: src.Legacy},
		Warnings: []string{},
	}
	set := cfg.Resolve(env.Lookup)
	st.Warnings = append(st.Warnings, set.Warnings...)
	configured := map[string]int{}
	for _, sp := range cfg.Specs() {
		configured[sp.Target]++
	}
	installed := map[string]int{}
	for _, it := range env.Inventory(project, false) {
		installed[it.Target]++
	}
	for _, t := range tokens.Targets() {
		row := tokenStatusTarget{Target: t, File: env.TargetPath(t, project), Mode: "-",
			Installed: installed[t], Configured: configured[t], Resolved: set.Len(t)}
		if fi, err := os.Stat(row.File); err == nil {
			row.Exists = true
			if !env.Windows {
				row.Mode = fmt.Sprintf("%04o", fi.Mode().Perm())
			}
		}
		st.Targets = append(st.Targets, row)
	}
	return st, nil
}

// printTokenStatus is `token status` for a person; the TUI shows the same.
func printTokenStatus(w io.Writer, st tokenStatus) error {
	line := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }
	switch {
	case st.Config.Builtin:
		line("config:  none found — the built-in default (GitHub for Composer from $GH_TOKEN); `%s token init` writes one", brand.Command)
	case st.Config.Legacy:
		line("config:  %s (an auth-token-manager location — `%s token import` moves it)", st.Config.Path, brand.Command)
	default:
		line("config:  %s", st.Config.Path)
	}
	line("")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tFILE\tMODE\tINSTALLED\tIN CONFIG\tRESOLVED")
	for _, r := range st.Targets {
		mode := r.Mode
		if !r.Exists {
			mode = "absent"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\n", r.Target, r.File, mode, r.Installed, r.Configured, r.Resolved)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(st.Warnings) > 0 {
		line("\n`%s token apply` would skip:", brand.Command)
		for _, wn := range st.Warnings {
			line("  %s", wn)
		}
	}
	return nil
}

// --- providers -----------------------------------------------------------

func newTokenProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List the Composer services and the known registries and git hosts a token can name",
		Long: "The token catalog: every Composer service `token set --service` and the\n" +
			"config's \"service\" key accept, with the auth mechanism each maps to and the\n" +
			"fields it needs; then the well-known hosts for npm, Docker and git (any other\n" +
			"host works too: these are shortcuts, not a whitelist).",
		Example: "  " + brand.Command + " token providers",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			needs := map[string]string{
				"http-basic":      "user_env + pass_env",
				"bitbucket-oauth": "key_env + secret_env",
				"custom-headers":  "headers (with ${VAR})",
			}
			svcs := tokens.LoadCatalogServices()
			names := make([]string, 0, len(svcs))
			for n := range svcs {
				names = append(names, n)
			}
			sort.Strings(names)
			say(c, "Composer services (--service, or \"service\" in the config):")
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "SERVICE\tMECHANISM\tNEEDS")
			for _, n := range names {
				need := needs[svcs[n]]
				if need == "" {
					need = "env"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", n, svcs[n], need)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			cat, err := tokens.LoadCatalog()
			if err != nil {
				return err
			}
			say(c, "\nKnown hosts for the other tools (any host works; these are shortcuts):")
			tw = tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "TOOL\tPROVIDER\tHOST\tSELF-HOSTED")
			for _, t := range cat.ProviderTargets() {
				for _, p := range cat.Providers[t] {
					host := p.Cloud
					if host == "" {
						host = "(your server)"
					}
					self := "no"
					if p.Hostable {
						self = "yes"
					}
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t, p.Label, host, self)
				}
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			say(c, "\nnetrc takes any machine name; npm, docker, git and netrc need no service key.")
			return nil
		},
	}
}

// --- import --------------------------------------------------------------

func newTokenImportCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import [FILE]",
		Short: "Copy an auth-token-manager (or any) token config to tokens.json, so the old location stops being read",
		Long: "Without FILE it finds auth-token-manager's config where that tool looked\n" +
			"($AUTH_TOKEN_MANAGER_CONFIG, $COMPOSER_AUTH_CONFIG, ./auth-token-manager.json,\n" +
			"~/.config/auth-token-manager.json). The file is parsed first, never copied\n" +
			"over an existing tokens.json, and left in place: delete it when you are happy.\n" +
			"The format is unchanged, so nothing is converted.",
		Example: "  " + brand.Command + " token import\n" +
			"  " + brand.Command + " token import ./team-tokens.json --dry-run",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			from := ""
			if len(args) == 1 {
				from = args[0]
			}
			if !dryRun {
				mutation.LockForMutation(env.Paths)
			}
			src, to, err := env.Import(from, dryRun)
			if errors.Is(err, tokensvc.ErrNoLegacyConfig) {
				say(c, "No auth-token-manager config found; nothing to import.")
				return nil
			}
			if err != nil {
				return err
			}
			if dryRun {
				say(c, "would copy %s → %s (the config parses; nothing written)", src, to)
				return nil
			}
			say(c, "✓ copied %s → %s", src, to)
			say(c, "The old file is left in place; delete it once you are happy with the new one.")
			say(c, "Next: `%s token status` shows what it resolves to, `%s token apply` writes the files.",
				brand.Command, brand.Command)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "check the file parses and say where it would go; write nothing")
	return cmd
}
