package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/simtabi/ssh-manager/src/v3/internal/brand"
	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/platform"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/mutation"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/tokensvc"
)

// The token group: credentials package managers and git read from their own
// files (Composer's auth.json, .npmrc, Docker's config.json, the git
// credential store, .netrc), built from a config that names environment
// variables and never holds a secret. Ported from auth-token-manager.

const tokenGroup = "token"

// tokenGetenv and tokenLookup read the environment; tests swap them.
var (
	tokenGetenv = os.Getenv
	tokenLookup = os.LookupEnv
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   tokenGroup,
		Short: "Package-manager and git auth tokens: Composer, npm, Docker, git, netrc",
		Long: "Builds the credential files Composer, npm, Docker, git and curl read, from a\n" +
			"config that names environment variables (never secrets), and lists, copies or\n" +
			"removes what is installed. Files are written atomically and owner-only.\n\n" +
			"Config: --config, $" + tokensvc.ConfigEnv + ", ./" + tokensvc.LocalConfigName +
			", or tokens.json in the config home.",
		Args: cobra.ArbitraryArgs,
		RunE: groupHelp,
	}
	cmd.AddCommand(newTokenInventoryCmd(), newTokenStoreCmd(), newTokenForgetCmd(),
		newTokenClipCmd(), newTokenSyncCmd(), newTokenSkeletonCmd(), newTokenScaffoldCmd(),
		newTokenServicesCmd(), newTokenCheckupCmd(), newTokenAdoptLegacyCmd(),
		newTokenShellAliasesCmd())
	return cmd
}

func tokenEnv(c *cobra.Command) (tokensvc.Env, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return tokensvc.Env{}, err
	}
	return tokensvc.NewEnv(p, tokenGetenv, tokenLookup, ""), nil
}

func checkTarget(t string) error {
	if !tokens.IsTarget(t) {
		return fmt.Errorf("unknown target %q (choose from %s)", t, strings.Join(tokens.Targets(), ", "))
	}
	return nil
}

func checkTargets(ts []string) (map[string]bool, error) {
	sel := map[string]bool{}
	for _, t := range ts {
		if err := checkTarget(t); err != nil {
			return nil, err
		}
		sel[t] = true
	}
	return sel, nil
}

func targetsFlag(c *cobra.Command, v *[]string, what string) {
	c.Flags().StringSliceVar(v, "target", nil, what+" (repeatable: "+strings.Join(tokens.Targets(), ", ")+")")
}

func note(c *cobra.Command, format string, a ...any) {
	_, _ = fmt.Fprintf(c.ErrOrStderr(), format+"\n", a...)
}

func say(c *cobra.Command, format string, a ...any) {
	_, _ = fmt.Fprintf(c.OutOrStdout(), format+"\n", a...)
}

// --- inventory -----------------------------------------------------------

func newTokenInventoryCmd() *cobra.Command {
	var asJSON, reveal, yes bool
	var targets []string
	var project string
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "List the tokens installed in the credential files (secrets masked)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			sel, err := checkTargets(targets)
			if err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			if reveal && !yes {
				if stdinIsTerminal() {
					reveal = confirm(c, "Reveal the actual secret values to your terminal?")
					if !reveal {
						note(c, "keeping secrets masked.")
					}
				} else {
					note(c, "refusing to print secrets non-interactively — re-run with --yes.")
					reveal = false
				}
			}
			var items []tokens.Installed
			for _, it := range env.Inventory(project, reveal) {
				if len(sel) == 0 || sel[it.Target] {
					items = append(items, it)
				}
			}
			if asJSON {
				if items == nil {
					items = []tokens.Installed{}
				}
				b, _ := json.MarshalIndent(items, "", "  ")
				say(c, "%s", b)
				return nil
			}
			if len(items) == 0 {
				say(c, "No tokens installed.")
				return nil
			}
			if reveal {
				note(c, "revealing secrets — they will print to your terminal.")
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "TOOL\tHOST\tWHAT\tUSER\tSECRET\tFILE")
			for _, it := range items {
				user := emDash(it.Username)
				secret := "—"
				switch {
				case reveal:
					secret = emDash(it.Secret)
				case it.SecretPresent:
					secret = "••••••••"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", it.Target, it.Host, it.Description, user, secret, it.File)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show the secret values (asks first; --yes when not at a terminal)")
	cmd.Flags().BoolVar(&yes, "yes", false, "reveal without asking")
	targetsFlag(cmd, &targets, "only these tools")
	cmd.Flags().StringVar(&project, "project", "", "read the project's auth.json and .npmrc instead of the user ones")
	return cmd
}

func emDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// --- store ---------------------------------------------------------------

func newTokenStoreCmd() *cobra.Command {
	var target, service, server, username, scope, project, mode string
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Save one token into its tool's credential file (the secret is read from stdin or a hidden prompt)",
		Long: "Saves one credential. --service picks a Composer service (see `token services`);\n" +
			"otherwise give --target. Token tools read a token; http-basic, docker, git and\n" +
			"netrc also need --username and read a password or token. The secret is read\n" +
			"from standard input when it is piped, else from a prompt that does not echo; it\n" +
			"is never a flag, so it stays out of shell history and the process list.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			mech := ""
			if service != "" {
				m, ok := tokens.LoadCatalogServices()[service]
				if !ok {
					return fmt.Errorf("unknown service %q (see `%s token services`)", service, brand.Command)
				}
				target, mech = tokens.Composer, m
			} else {
				if target == "" {
					return errors.New("give --service (Composer) or --target")
				}
				if err := checkTarget(target); err != nil {
					return err
				}
				if target == tokens.Composer {
					return errors.New("a Composer token needs --service (see `" + brand.Command + " token services`)")
				}
			}
			if strings.TrimSpace(server) == "" {
				return errors.New("--server is required")
			}
			if !tokensvc.ValidDockerMode(mode) {
				return fmt.Errorf("--creds-store-mode must be one of %s", strings.Join(tokensvc.DockerModes(), ", "))
			}
			var cr tokens.Credential
			switch {
			case target == tokens.NPM || target == tokens.Composer && tokens.IsTokenMechanism(mech):
				s, err := readTokenSecret(c, "Token")
				if err != nil {
					return err
				}
				cr = tokens.Credential{Token: s, Scope: scope}
			case target == tokens.Composer && mech != "http-basic":
				return fmt.Errorf("%s (%s) takes more than one secret; add it to the token config instead", service, mech)
			default:
				if username == "" {
					return errors.New("--username is required for " + target)
				}
				s, err := readTokenSecret(c, "Password / token")
				if err != nil {
					return err
				}
				cr = tokens.Credential{Username: username, Password: s}
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			set := tokens.NewSet()
			if !set.Apply(target, mech, strings.TrimSpace(server), cr) {
				return errors.New("nothing to save: the credential is incomplete")
			}
			mutation.LockForMutation(env.Paths)
			return writeTokenTargets(c, env, set, []string{target}, project, mode, nil)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "the tool: npm, docker, git or netrc (Composer uses --service)")
	cmd.Flags().StringVar(&service, "service", "", "a Composer service, e.g. github or gitlab (see token services)")
	cmd.Flags().StringVar(&server, "server", "", "the registry or git server, e.g. github.com or registry.npmjs.org")
	cmd.Flags().StringVar(&username, "username", "", "the user name, for http-basic, docker, git and netrc")
	cmd.Flags().StringVar(&scope, "scope", "", "npm only: bind an @org scope to this registry")
	cmd.Flags().StringVar(&project, "project", "", "write the project's auth.json or .npmrc instead of the user one")
	dockerModeFlag(cmd, &mode)
	return cmd
}

func dockerModeFlag(cmd *cobra.Command, mode *string) {
	cmd.Flags().StringVar(mode, "creds-store-mode", "",
		"when Docker keeps a registry in a credential store: login (docker login --password-stdin) or refuse")
}

// readTokenSecret reads one secret: a line from piped stdin, or a prompt that
// does not echo at a terminal.
func readTokenSecret(c *cobra.Command, label string) (string, error) {
	var s string
	var err error
	if stdinIsTerminal() {
		s, err = platform.ReadSecret(label + ": ")
	} else {
		s, err = platform.ReadLine(c.InOrStdin())
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New(strings.ToLower(label) + " is empty; nothing saved")
	}
	return s, nil
}

// writeTokenTargets writes each target's entries and says where.
func writeTokenTargets(c *cobra.Command, env tokensvc.Env, set *tokens.Set, targets []string, project, mode string, cfg *tokens.Config) error {
	for _, t := range targets {
		dest := env.TargetPath(t, project)
		if t == tokens.Docker {
			m, err := dockerMode(c, set, dest, mode, cfg)
			if err != nil {
				return err
			}
			res, err := env.SyncDocker(set, dest, m)
			if err != nil {
				return err
			}
			for _, w := range res.Warnings {
				note(c, "  %s", w)
			}
			for _, h := range res.LoggedIn {
				say(c, "✓ stored docker: %s with docker login", h)
			}
			if len(res.Written) > 0 {
				say(c, "✓ wrote docker → %s", dest)
			}
			continue
		}
		warns, err := env.WriteTarget(set, t, dest)
		if err != nil {
			return err
		}
		say(c, "✓ wrote %s → %s", t, dest)
		for _, w := range warns {
			note(c, "  %s", w)
		}
	}
	return nil
}

// dockerMode settles what to do with registries Docker keeps in a credential
// store: the flag, then the config's _settings.docker_creds_store, then a
// question at a terminal; without one, refuse and warn.
func dockerMode(c *cobra.Command, set *tokens.Set, dest, flag string, cfg *tokens.Config) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if cfg != nil {
		if m := cfg.Setting("docker_creds_store"); m != "" {
			if !tokensvc.ValidDockerMode(m) {
				return "", fmt.Errorf("_settings.docker_creds_store must be one of %s, not %q",
					strings.Join(tokensvc.DockerModes(), ", "), m)
			}
			return m, nil
		}
	}
	if !tokensvc.HasDockerDelegation(set, dest) || !stdinIsTerminal() {
		return tokensvc.DockerRefuse, nil
	}
	if confirm(c, "Docker keeps these registries in a credential store, so an inline auth would be ignored.\n"+
		"Store them with `docker login --password-stdin` instead? (no skips them)") {
		return tokensvc.DockerLogin, nil
	}
	return tokensvc.DockerRefuse, nil
}

// --- forget / clip -------------------------------------------------------

func newTokenForgetCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "forget TARGET HOST",
		Short: "Delete one installed token from its tool's credential file",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			target := args[0]
			if err := checkTarget(target); err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			mutation.LockForMutation(env.Paths)
			host := tokens.CanonicalHost(target, args[1])
			ok, err := env.Forget(target, host, project)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("nothing removed — %s: %s not found", target, host)
			}
			say(c, "✓ removed %s: %s", target, host)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "edit the project's auth.json or .npmrc instead of the user one")
	return cmd
}

func newTokenClipCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "clip TARGET HOST",
		Short: "Copy one installed token to the clipboard without printing it",
		Args:  cobra.ExactArgs(2),
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
			if !ok || it.Secret == "" {
				return fmt.Errorf("no token found for %s: %s", target, host)
			}
			if err := clipboardWrite(it.Secret); err != nil {
				return fmt.Errorf("couldn't reach a clipboard tool (pbcopy/wl-copy/xclip/xsel/clip): %w", err)
			}
			say(c, "✓ copied %s: %s token to the clipboard", target, host)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "read the project's auth.json or .npmrc instead of the user one")
	return cmd
}

// --- sync ----------------------------------------------------------------

// loadTokenConfig finds and reads the token config, saying where it came
// from when that is a legacy location or the built-in default.
func loadTokenConfig(c *cobra.Command, env tokensvc.Env, explicit string) (*tokens.Config, error) {
	src, err := env.FindConfig(explicit)
	if err != nil {
		return nil, err
	}
	if src.Path == "" {
		note(c, "note: no token config found — using the built-in default (GitHub for Composer from $GH_TOKEN); "+
			"`%s token scaffold` writes one.", brand.Command)
		return tokensvc.Builtin(), nil
	}
	if src.Legacy {
		note(c, "deprecated: reading the auth-token-manager config %s; run `%s token adopt-legacy` to move it to %s.",
			src.Path, brand.Command, env.Paths.Tokens())
	}
	return tokensvc.LoadConfig(src.Path)
}

func newTokenSyncCmd() *cobra.Command {
	var config, project, source, mode string
	var targets []string
	var exports, preview, noHint bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Resolve the token config from the environment and write each tool's credential file",
		Long: "Reads the token config, fills each entry from the environment variables it names,\n" +
			"and merges the result into each tool's file - entries for other hosts are kept.\n" +
			"--preview prints what would be written, secrets masked; --exports prints\n" +
			"COMPOSER_AUTH as a shell export for CI instead of writing anything.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			sel, err := checkTargets(targets)
			if err != nil {
				return err
			}
			if !tokensvc.ValidDockerMode(mode) {
				return fmt.Errorf("--creds-store-mode must be one of %s", strings.Join(tokensvc.DockerModes(), ", "))
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			if source != "" {
				if err := env.LoadEnvFile(source); err != nil {
					return err
				}
			}
			cfg, err := loadTokenConfig(c, env, config)
			if err != nil {
				return err
			}
			set := cfg.Resolve(env.Lookup)
			for _, w := range tokens.RelevantWarnings(set.Warnings, sel) {
				note(c, "  %s", w)
			}
			if set.Len("") == 0 {
				return errors.New("no credentials resolved — set the env vars named in your config " +
					"(or use --source), or add one with `" + brand.Command + " token store`")
			}
			if preview {
				say(c, "%s", set.RedactedJSON(sel))
				return nil
			}
			chosen := selectTargets(set, sel)
			if len(chosen) == 0 {
				return nothingToWrite(sel)
			}
			if exports {
				shell := tokens.DetectShell(env.Getenv, env.Windows)
				for _, t := range chosen {
					if t == tokens.Composer {
						say(c, "%s", set.ComposerEnv(shell))
					} else {
						note(c, "note: %s has no env channel — render it to a file (no --exports).", t)
					}
				}
				return nil
			}
			mutation.LockForMutation(env.Paths)
			if err := writeTokenTargets(c, env, set, chosen, project, mode, cfg); err != nil {
				return err
			}
			if sel[tokens.Composer] || len(sel) == 0 && set.Len(tokens.Composer) > 0 {
				if !noHint {
					note(c, "\nFor CI, `%s token sync --exports` prints COMPOSER_AUTH as a shell export.", brand.Command)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&config, "config", "", "the token config file (default: discovered)")
	targetsFlag(cmd, &targets, "only these tools")
	cmd.Flags().StringVar(&project, "project", "", "write the project's auth.json and .npmrc instead of the user ones")
	cmd.Flags().StringVar(&source, "source", "", "load VAR=value lines from this env file first (set variables win)")
	cmd.Flags().BoolVar(&exports, "exports", false, "print COMPOSER_AUTH as a shell export instead of writing files")
	cmd.Flags().BoolVar(&preview, "preview", false, "print the resolved credentials as JSON, secrets masked; write nothing")
	cmd.Flags().BoolVar(&noHint, "no-env-hint", false, "do not print the CI hint after writing Composer's file")
	dockerModeFlag(cmd, &mode)
	return cmd
}

func selectTargets(set *tokens.Set, sel map[string]bool) []string {
	var out []string
	for _, t := range set.Targets() {
		if len(sel) == 0 || sel[t] {
			out = append(out, t)
		}
	}
	return out
}

func nothingToWrite(sel map[string]bool) error {
	scope := ""
	if len(sel) > 0 {
		var ts []string
		for t := range sel {
			ts = append(ts, t)
		}
		sort.Strings(ts)
		scope = " for " + strings.Join(ts, ", ")
	}
	return fmt.Errorf("nothing to write%s — no credentials resolved for it", scope)
}

// --- skeleton ------------------------------------------------------------

func newTokenSkeletonCmd() *cobra.Command {
	var project string
	var targets []string
	var preview bool
	cmd := &cobra.Command{
		Use:   "skeleton",
		Short: "Show what each tool's credential file looks like, with placeholder values",
		Long: "Prints every tool's file filled with placeholders, and writes nothing. With\n" +
			"--project it writes the project-scoped ones (auth.json, .npmrc) into that\n" +
			"directory; the others live in your home directory, where placeholders would\n" +
			"replace real credentials, so they are never written.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			sel, err := checkTargets(targets)
			if err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			set := tokens.Template()
			if preview {
				say(c, "%s", set.RedactedJSON(sel))
				return nil
			}
			chosen := selectTargets(set, sel)
			if project == "" {
				for _, t := range chosen {
					say(c, "# %s → %s", t, env.TargetPath(t, ""))
					text, err := set.Render(t, "", false)
					if err != nil {
						return err
					}
					say(c, "%s", text)
				}
				note(c, "NOTE: placeholders are not valid credentials — replace them.")
				return nil
			}
			var write, skipped []string
			for _, t := range chosen {
				if t == tokens.Composer || t == tokens.NPM {
					write = append(write, t)
				} else {
					skipped = append(skipped, t)
				}
			}
			if len(skipped) > 0 {
				note(c, "note: --project only writes the project-scoped targets (composer, npm); skipped %s — "+
					"their placeholders would land in your home-directory credential files.", strings.Join(skipped, ", "))
			}
			if len(write) == 0 {
				return nothingToWrite(sel)
			}
			mutation.LockForMutation(env.Paths)
			if err := writeTokenTargets(c, env, set, write, project, tokensvc.DockerRefuse, nil); err != nil {
				return err
			}
			note(c, "NOTE: placeholders are not valid credentials — replace them.")
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "write the placeholder auth.json and .npmrc into this directory")
	targetsFlag(cmd, &targets, "only these tools")
	cmd.Flags().BoolVar(&preview, "preview", false, "print the placeholders as JSON, masked, instead of as files")
	return cmd
}

// --- scaffold / services / adopt-legacy ----------------------------------

func newTokenScaffoldCmd() *cobra.Command {
	var targets []string
	cmd := &cobra.Command{
		Use:   "scaffold [FILE]",
		Short: "Write a starter token config (never over an existing file)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if _, err := checkTargets(targets); err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			dest := ""
			if len(args) == 1 {
				dest = args[0]
			}
			mutation.LockForMutation(env.Paths)
			msg, err := env.Scaffold(dest, targets)
			if err != nil {
				return err
			}
			say(c, "%s", msg)
			say(c, "Edit it, then run `%s token sync`.", brand.Command)
			return nil
		},
	}
	targetsFlag(cmd, &targets, "start from these tools' templates")
	return cmd
}

func newTokenServicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "services",
		Short: "List the Composer services a token config can name, and what each needs",
		Args:  cobra.NoArgs,
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
			say(c, "\nOther targets (npm/docker/git/netrc) take host + env refs directly — no service catalog needed.")
			return nil
		},
	}
}

func newTokenAdoptLegacyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt-legacy",
		Short: "Copy an auth-token-manager config to tokens.json, so the old locations stop being read",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			mutation.LockForMutation(env.Paths)
			from, to, err := env.AdoptLegacy()
			if errors.Is(err, tokensvc.ErrNoLegacyConfig) {
				say(c, "No auth-token-manager config found; nothing to adopt.")
				return nil
			}
			if err != nil {
				return err
			}
			say(c, "✓ copied %s → %s", from, to)
			say(c, "The old file is left in place; delete it once you are happy with the new one.")
			return nil
		},
	}
}

// --- checkup -------------------------------------------------------------

func newTokenCheckupCmd() *cobra.Command {
	var config, project string
	var targets []string
	cmd := &cobra.Command{
		Use:   "checkup",
		Short: "Check the credential files are owner-only and .gitignore covers them; write nothing",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			sel, err := checkTargets(targets)
			if err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			set := tokens.NewSet()
			for t := range sel {
				set.Add(tokens.Credential{Target: t, Host: "-", Kind: tokens.KindToken, Token: "-"})
			}
			if len(sel) == 0 && config != "" {
				cfg, err := loadTokenConfig(c, env, config)
				if err != nil {
					return err
				}
				set = cfg.Resolve(env.Lookup)
			}
			findings, failed := env.Checkup(orderedSet(set), project)
			printFindings(c, findings)
			if failed {
				return errNotClean
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&config, "config", "", "check the tools this token config uses")
	targetsFlag(cmd, &targets, "only these tools")
	cmd.Flags().StringVar(&project, "project", "", "check the project's files and .gitignore")
	return cmd
}

// orderedSet puts a set's targets in catalog order, as the Python tool listed them.
func orderedSet(s *tokens.Set) *tokens.Set {
	out := tokens.NewSet()
	for _, t := range tokens.Targets() {
		for _, c := range s.Entries(t) {
			out.Add(c)
		}
	}
	return out
}

func printFindings(c *cobra.Command, fs []tokensvc.Finding) {
	for _, f := range fs {
		switch f.Level {
		case "ok":
			say(c, "✓ %s", f.Text)
		case "absent":
			say(c, "· %s", f.Text)
		case "note":
			say(c, "    note: %s", f.Text)
		default:
			if f.Target != "" && !strings.HasPrefix(f.Text, f.Target+":") {
				say(c, "    WARN: %s", f.Text)
			} else if f.Target != "" {
				say(c, "✗ %s", f.Text)
			} else {
				say(c, "WARN: %s", f.Text)
			}
		}
	}
}

// --- shell-aliases -------------------------------------------------------

func newTokenShellAliasesCmd() *cobra.Command {
	var install, uninstall, print, login, noBackup, quiet bool
	var file string
	cmd := &cobra.Command{
		Use:   "shell-aliases",
		Short: "Show, install or remove the atm shell aliases (atm, atm-ui, atm-doctor, atm-init)",
		Long: "The aliases auth-token-manager installed, pointed at this tool: atm runs\n" +
			"`" + brand.Command + " token`, atm-ui the menu, atm-doctor `token checkup`,\n" +
			"atm-init `token scaffold`. They live in a marked block in your shell's rc file,\n" +
			"which --install replaces (an auth-token-manager block included) and --uninstall\n" +
			"removes. Without a flag it reports what the file holds.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			n := 0
			for _, b := range []bool{install, uninstall, print} {
				if b {
					n++
				}
			}
			if n > 1 {
				return errors.New("--install, --uninstall and --print are exclusive")
			}
			if print {
				for _, l := range tokens.AliasLines(tokens.DefaultAliases(brand.Command)) {
					say(c, "%s", l)
				}
				return nil
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			pr := env.Profile(file, login)
			quietly := func(format string, a ...any) {
				if !quiet {
					say(c, format, a...)
				}
			}
			switch {
			case install:
				mutation.LockForMutation(env.Paths)
				updated, warns, err := pr.Install(brand.Command, !noBackup)
				if err != nil {
					return err
				}
				for _, w := range warns {
					note(c, "  %s", w)
				}
				verb := "installed"
				if updated {
					verb = "updated"
				}
				quietly("✓ %s aliases in %s — open a new shell or `source %s`.", verb, pr.File, pr.File)
			case uninstall:
				mutation.LockForMutation(env.Paths)
				removed, err := pr.Remove(!noBackup)
				if err != nil {
					return err
				}
				if removed {
					quietly("✓ removed aliases from %s", pr.File)
				} else {
					quietly("no aliases installed in %s", pr.File)
				}
			default:
				st := pr.Status()
				state := "absent"
				if st.Exists {
					state = "exists"
				}
				say(c, "profile: %s (%s)", st.File, state)
				if st.Present {
					say(c, "status:  installed")
				} else {
					say(c, "status:  not installed")
				}
				if st.Legacy {
					say(c, "note:    an auth-token-manager alias block is present; --install replaces it")
				}
				say(c, "command: %s token", brand.Command)
				for _, l := range st.Lines {
					say(c, "  %s", l)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "add or update the alias block")
	cmd.Flags().BoolVar(&uninstall, "uninstall", false, "remove the alias block")
	cmd.Flags().BoolVar(&print, "print", false, "print the alias lines, for eval")
	cmd.Flags().StringVar(&file, "file", "", "the startup file (default: your shell's rc file)")
	cmd.Flags().BoolVar(&login, "login", false, "use the login profile (.zprofile, .bash_profile) instead of the rc file")
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "do not copy the file to <file>"+tokensvc.AliasBackupSuffix+" first")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "print nothing on success")
	return cmd
}
