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

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/exports"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/util/csvout"
	"github.com/simtabi/authama/src/internal/util/envnames"
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
		Use:     tokenGroup,
		Aliases: []string{"auth"},
		Short:   "Package-manager and git auth tokens: Composer, npm, Docker, git, netrc",
		Long: "Builds the credential files Composer, npm, Docker, git and curl read, from a\n" +
			"config that names environment variables (never secrets), and lists, copies or\n" +
			"removes what is installed. Files are written atomically and owner-only.\n\n" +
			"Config: --config, $" + tokensvc.ConfigEnv + ", ./" + tokensvc.LocalConfigName +
			", or tokens.json in the config home.",
		Args:              cobra.ArbitraryArgs,
		RunE:              groupHelp,
		PersistentPreRunE: useTokenCatalog,
	}
	cmd.AddCommand(newTokenListCmd(), newTokenShowCmd(), newTokenStatusCmd(), newTokenSetCmd(),
		newTokenDeleteCmd(), newTokenCopyCmd(), newTokenApplyCmd(), newTokenExamplesCmd(),
		newTokenInitCmd(), newTokenProvidersCmd(), newTokenAuditCmd(), newTokenImportCmd(),
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

// useTokenCatalog runs before every token command: it installs the catalog
// $AUTHAMA_TOKENS_CATALOG (or its older names $SSH_MANAGER_TOKENS_CATALOG and
// auth-token-manager's
// $AUTH_TOKEN_MANAGER_CATALOG) names, says so when the variable is the old one
// or names no file, and refuses to go on with a catalog that does not load.
func useTokenCatalog(c *cobra.Command, _ []string) error {
	env, err := tokenEnv(c)
	if err != nil {
		return nil // the command itself reports a layout it cannot resolve
	}
	if err := env.Catalog.Err; err != nil {
		return err
	}
	errw := c.ErrOrStderr()
	if w := env.Catalog.Warning; w != "" {
		_, _ = fmt.Fprintf(errw, "warning: %s.\n", w)
	}
	if env.Catalog.Legacy && env.Catalog.Path != "" {
		_, _ = fmt.Fprintf(errw, "deprecated: $%s; set $%s instead.\n", env.Catalog.Var, tokensvc.CatalogEnv)
	}
	return nil
}

func checkTarget(t string) error {
	if !tokens.IsTarget(t) {
		return fmt.Errorf("unknown target %q (choose from %s)", t, strings.Join(tokens.Targets(), ", "))
	}
	return nil
}

// checkTargets validates --target values. No targets is nil, which every
// consumer (RedactedJSON included) reads as "all of them"; an empty map would
// select nothing.
func checkTargets(ts []string) (map[string]bool, error) {
	if len(ts) == 0 {
		return nil, nil
	}
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

// --- list --------------------------------------------------------------

func newTokenListCmd() *cobra.Command {
	var asJSON, asCSV, reveal, yes bool
	var targets []string
	var project string
	cmd := &cobra.Command{
		Use: "list",
		Example: "  " + brand.Command + " token list" + "\n" +
			"  " + brand.Command + " token list --target npm --target git" + "\n" +
			"  " + brand.Command + " token list --project . --json",
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
			reveal = mayReveal(c, reveal, yes)
			if reveal {
				if err := requireOwner(c, env.Paths, "revealing tokens"); err != nil {
					return err
				}
			}
			var items []tokens.Installed
			for _, it := range env.Inventory(project, reveal) {
				if len(sel) == 0 || sel[it.Target] {
					items = append(items, it)
				}
			}
			if asCSV {
				if reveal {
					return errors.New("--csv never holds a secret: drop --reveal")
				}
				return csvout.Write(c.OutOrStdout(), exports.Tokens(items))
			}
			if reveal {
				recordTokenAccess(env, eventTokenRevealed, items...)
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
			return writeTokenTable(c.OutOrStdout(), items, reveal)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON")
	cmd.Flags().BoolVar(&asCSV, "csv", false, "print the list as CSV, header first; never the secrets")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show the secret values (asks first; --yes when not at a terminal)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "reveal without asking")
	targetsFlag(cmd, &targets, "only these tools")
	cmd.Flags().StringVar(&project, "project", "", "read the project's auth.json and .npmrc instead of the user ones")
	return cmd
}

// mayReveal settles --reveal: --yes, or a yes at a terminal; never without
// one, so a pipe or a log never receives a secret by accident.
func mayReveal(c *cobra.Command, reveal, yes bool) bool {
	if !reveal || yes {
		return reveal
	}
	if !stdinIsTerminal() {
		note(c, "refusing to print secrets non-interactively — re-run with --yes.")
		return false
	}
	if confirm(c, "Reveal the actual secret values to your terminal?") {
		return true
	}
	note(c, "keeping secrets masked.")
	return false
}

// writeTokenTable prints installed tokens, secrets masked unless revealed.
func writeTokenTable(w io.Writer, items []tokens.Installed, reveal bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tHOST\tWHAT\tUSER\tSECRET\tFILE")
	for _, it := range items {
		secret := "—"
		switch {
		case reveal:
			secret = emDash(it.Secret)
		case it.SecretPresent:
			secret = "••••••••"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", it.Target, it.Host, it.Description, emDash(it.Username), secret, it.File)
	}
	return tw.Flush()
}

func emDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// --- set -----------------------------------------------------------------

func newTokenSetCmd() *cobra.Command {
	var target, service, server, username, scope, project, mode string
	cmd := &cobra.Command{
		Use: "set",
		Example: "  printf %s \"$GH_TOKEN\" | " + brand.Command + " token set --service github --server github.com" + "\n" +
			"  " + brand.Command + " token set --target npm --server npm.pkg.github.com --scope @acme" + "\n" +
			"  " + brand.Command + " token set --target git --server github.com --username octocat",
		Short: "Add or replace one token in its tool's credential file (the secret is read from stdin or a hidden prompt)",
		Long: "Adds one credential, or replaces the one already saved for that host (so it is\n" +
			"also how a token is rotated or edited). --service picks a Composer service (see\n" +
			"`token providers`); otherwise give --target. Token tools read a token; http-basic,\n" +
			"docker, git and netrc also need --username and read a password or token. The secret is read\n" +
			"from standard input when it is piped, else from a prompt that does not echo; it\n" +
			"is never a flag, so it stays out of shell history and the process list.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			plan, err := tokensvc.SetSpec{Target: target, Service: service, Server: server, Username: username, Scope: scope}.Check()
			if err != nil {
				return err
			}
			if !tokensvc.ValidDockerMode(mode) {
				return fmt.Errorf("--creds-store-mode must be one of %s", strings.Join(tokensvc.DockerModes(), ", "))
			}
			secret, err := readTokenSecret(c, plan.SecretLabel())
			if err != nil {
				return err
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			set, err := plan.Credential(secret)
			if err != nil {
				return err
			}
			target = plan.Target
			mutation.LockForMutation(env.Paths)
			return writeTokenTargets(cliTokenIO(c), env, set, []string{target}, project, mode, nil)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "the tool: npm, docker, git, netrc, pypi, cargo, gem or gradle (Composer uses --service)")
	cmd.Flags().StringVar(&service, "service", "", "a Composer service, e.g. github or gitlab (see token providers)")
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

// tokenIO is where a token write reports, and who answers its one question
// (whether to hand credential-store registries to docker login). The CLI and
// the TUI each build one, so the write itself is shared.
type tokenIO struct {
	out, errw io.Writer
	// ask is nil when nobody can answer: the docker question then refuses.
	ask func(prompt string) bool
}

func cliTokenIO(c *cobra.Command) tokenIO {
	tio := tokenIO{out: c.OutOrStdout(), errw: c.ErrOrStderr()}
	if stdinIsTerminal() {
		tio.ask = func(prompt string) bool { return confirm(c, prompt) }
	}
	return tio
}

// writeTokenTargets writes each target's entries and says where. Docker's
// credential-store question goes to whoever tio can ask.
func writeTokenTargets(tio tokenIO, env tokensvc.Env, set *tokens.Set, targets []string, project, mode string, cfg *tokens.Config) error {
	ask := func(dest string) (string, error) { return dockerMode(tio, set, dest, mode, cfg) }
	return env.WriteAll(set, targets, project, ask, tio.out, tio.errw)
}

// dockerMode settles what to do with registries Docker keeps in a credential
// store: the flag, then $AUTHAMA_DOCKER_CREDS_STORE (which .env or
// authama.json's tokens.docker_creds_store may supply), then the token
// config's _settings.docker_creds_store, then a question when someone can
// answer; without one, refuse and warn.
func dockerMode(tio tokenIO, set *tokens.Set, dest, flag string, cfg *tokens.Config) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if m := strings.TrimSpace(envnames.Get(nil, envnames.DockerCredsStore)); m != "" {
		if !tokensvc.ValidDockerMode(m) {
			return "", fmt.Errorf("$%s must be one of %s, not %q", envnames.DockerCredsStore,
				strings.Join(tokensvc.DockerModes(), ", "), m)
		}
		return m, nil
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
	if tio.ask == nil || !tokensvc.HasDockerDelegation(set, dest) {
		return tokensvc.DockerRefuse, nil
	}
	if tio.ask("Docker keeps these registries in a credential store, so an inline auth would be ignored.\n" +
		"Store them with `docker login --password-stdin` instead? (no skips them)") {
		return tokensvc.DockerLogin, nil
	}
	return tokensvc.DockerRefuse, nil
}

// --- delete / copy -------------------------------------------------------

func newTokenDeleteCmd() *cobra.Command {
	var project string
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete TARGET HOST",
		Short: "Delete one installed token from its tool's credential file",
		Long: "Removes the entry for HOST from TARGET's credential file; entries for other\n" +
			"hosts are kept. At a terminal it asks first (--yes skips the question); without\n" +
			"one it goes ahead, as every other delete does.",
		Example: "  " + brand.Command + " token delete npm registry.npmjs.org\n" +
			"  " + brand.Command + " token delete composer github.com --project . --yes",
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
			if _, found := env.Find(target, host, project); !found {
				return fmt.Errorf("nothing removed — %s: %s not found", target, host)
			}
			if !yes && stdinIsTerminal() &&
				!confirm(c, fmt.Sprintf("Delete the %s token for %s from %s?", target, host, env.TargetPath(target, project))) {
				say(c, "Kept it.")
				return nil
			}
			mutation.LockForMutation(env.Paths)
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
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newTokenCopyCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:     "copy TARGET HOST",
		Example: "  " + brand.Command + " token copy npm registry.npmjs.org",
		Short:   "Copy one installed token to the clipboard without printing it",
		Long: "Puts HOST's secret from TARGET's credential file on the clipboard, through\n" +
			"pbcopy, wl-copy, xclip, xsel or clip, and prints only what it copied. The\n" +
			"secret is written nowhere else; the history records that it was copied, and\n" +
			"from which file, never the value.",
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
			if !ok || it.Secret == "" {
				return fmt.Errorf("no token found for %s: %s", target, host)
			}
			if err := requireOwner(c, env.Paths, "copying a token"); err != nil {
				return err
			}
			if err := clipboardWrite(it.Secret); err != nil {
				return fmt.Errorf("couldn't reach a clipboard tool (pbcopy/wl-copy/xclip/xsel/clip): %w", err)
			}
			recordTokenAccess(env, eventTokenCopied, it)
			say(c, "✓ copied %s: %s token to the clipboard", target, host)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "read the project's auth.json or .npmrc instead of the user one")
	return cmd
}

// --- apply ---------------------------------------------------------------

// loadTokenConfig finds and reads the token config, saying where it came
// from when that is a legacy location or the built-in default.
func loadTokenConfig(errw io.Writer, env tokensvc.Env, explicit string) (*tokens.Config, error) {
	cfg, src, err := env.ResolveConfig(explicit)
	if err != nil {
		return nil, err
	}
	if src.Path == "" {
		_, _ = fmt.Fprintf(errw, "note: no token config found — using the built-in default (GitHub for Composer "+
			"from $GH_TOKEN); `%s token init` writes one.\n", brand.Command)
	} else if src.Legacy {
		_, _ = fmt.Fprintf(errw, "deprecated: reading the auth-token-manager config %s; run `%s token import` "+
			"to move it to %s.\n", src.Path, brand.Command, env.Paths.Tokens())
	}
	return cfg, nil
}

func newTokenApplyCmd() *cobra.Command {
	var config, project, source, mode string
	var targets []string
	var exports, dryRun, noHint bool
	cmd := &cobra.Command{
		Use: "apply",
		Example: "  " + brand.Command + " token apply" + "\n" +
			"  " + brand.Command + " token apply --dry-run --target composer" + "\n" +
			"  " + brand.Command + " token apply --source .env --project ." + "\n" +
			"  eval \"$(" + brand.Command + " token apply --exports)\"",
		Short: "Resolve the token config from the environment and write each tool's credential file",
		Long: "Reads the token config, fills each entry from the environment variables it names,\n" +
			"and merges the result into each tool's file - entries for other hosts are kept.\n" +
			"--dry-run prints what would be written, secrets masked; --exports prints\n" +
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
			cfg, err := loadTokenConfig(c.ErrOrStderr(), env, config)
			if err != nil {
				return err
			}
			set := cfg.Resolve(env.Lookup)
			for _, w := range tokens.RelevantWarnings(set.Warnings, sel) {
				note(c, "  %s", w)
			}
			if set.Len("") == 0 {
				return errors.New("no credentials resolved — set the env vars named in your config " +
					"(or use --source), or add one with `" + brand.Command + " token set`")
			}
			if dryRun {
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
			if err := writeTokenTargets(cliTokenIO(c), env, set, chosen, project, mode, cfg); err != nil {
				return err
			}
			if sel[tokens.Composer] || len(sel) == 0 && set.Len(tokens.Composer) > 0 {
				if !noHint {
					note(c, "\nFor CI, `%s token apply --exports` prints COMPOSER_AUTH as a shell export.", brand.Command)
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the resolved credentials as JSON, secrets masked; write nothing")
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

// --- examples ------------------------------------------------------------

func newTokenExamplesCmd() *cobra.Command {
	var project string
	var targets []string
	var asJSON bool
	cmd := &cobra.Command{
		Use: "examples",
		Example: "  " + brand.Command + " token examples --target docker" + "\n" +
			"  " + brand.Command + " token examples --json" + "\n" +
			"  " + brand.Command + " token examples --project ./demo",
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
			if asJSON {
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
			if err := writeTokenTargets(cliTokenIO(c), env, set, write, project, tokensvc.DockerRefuse, nil); err != nil {
				return err
			}
			note(c, "NOTE: placeholders are not valid credentials — replace them.")
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "write the placeholder auth.json and .npmrc into this directory")
	targetsFlag(cmd, &targets, "only these tools")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the placeholders as JSON, masked, instead of as files")
	return cmd
}

// --- init / providers / import -----------------------------------------

func newTokenInitCmd() *cobra.Command {
	var targets []string
	cmd := &cobra.Command{
		Use: "init [FILE]",
		Example: "  " + brand.Command + " token init" + "\n" +
			"  " + brand.Command + " token init --target composer --target npm ./authama-tokens.json",
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
			say(c, "Edit it, then run `%s token apply`.", brand.Command)
			return nil
		},
	}
	targetsFlag(cmd, &targets, "start from these tools' templates")
	return cmd
}

// --- audit ---------------------------------------------------------------

func newTokenAuditCmd() *cobra.Command {
	var config, project string
	var targets []string
	cmd := &cobra.Command{
		Use: "audit",
		Example: "  " + brand.Command + " token audit" + "\n" +
			"  " + brand.Command + " token audit --project . --target composer",
		Short: "Check the credential files are owner-only and .gitignore covers them; write nothing",
		Long: "For each tool (all five, the --target ones, or those a --config uses): is its\n" +
			"file there, and is it owner-only? With --project, the project's auth.json and\n" +
			".npmrc, and whether the project's .gitignore covers every secret file. Exits\n" +
			"non-zero when anything needs fixing; `" + brand.Command + " doctor --fix` tightens the modes.\n" +
			"`token status` is the read-only overview that never fails.",
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
			set := tokens.NewSet()
			for t := range sel {
				set.Add(tokens.Credential{Target: t, Host: "-", Kind: tokens.KindToken, Token: "-"})
			}
			if len(sel) == 0 && config != "" {
				cfg, err := loadTokenConfig(c.ErrOrStderr(), env, config)
				if err != nil {
					return err
				}
				set = cfg.Resolve(env.Lookup)
			}
			findings, failed := env.Checkup(orderedSet(set), project)
			printFindings(c.OutOrStdout(), findings)
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

func printFindings(w io.Writer, fs []tokensvc.Finding) {
	line := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }
	for _, f := range fs {
		switch f.Level {
		case "ok":
			line("✓ %s", f.Text)
		case "absent":
			line("· %s", f.Text)
		case "note":
			line("    note: %s", f.Text)
		default:
			if f.Target != "" && !strings.HasPrefix(f.Text, f.Target+":") {
				line("    WARN: %s", f.Text)
			} else if f.Target != "" {
				line("✗ %s", f.Text)
			} else {
				line("WARN: %s", f.Text)
			}
		}
	}
}

// --- shell-aliases -------------------------------------------------------

func newTokenShellAliasesCmd() *cobra.Command {
	var install, uninstall, print, login, noBackup, quiet bool
	var file string
	cmd := &cobra.Command{
		Use: "shell-aliases",
		Example: "  " + brand.Command + " token shell-aliases" + "\n" +
			"  " + brand.Command + " token shell-aliases --install" + "\n" +
			"  eval \"$(" + brand.Command + " token shell-aliases --print)\"",
		Short: "Show, install or remove the atm shell aliases (atm, atm-ui, atm-doctor, atm-init)",
		Long: "The aliases auth-token-manager installed, pointed at this tool: atm runs\n" +
			"`" + brand.Command + " token`, atm-ui the menu, atm-doctor `token audit`,\n" +
			"atm-init `token init`. They live in a marked block in your shell's rc file,\n" +
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
