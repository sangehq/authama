package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/naming"
	"github.com/simtabi/authama/src/internal/services/editor"
)

// generatedAlias names a new host of profile by its naming rules, and returns
// the tags it should carry (its role and environment).
func generatedAlias(m *manifest.Manifest, profile, env, role string) (string, []string, error) {
	if _, ok := m.Profiles[profile]; !ok {
		return "", nil, fmt.Errorf("unknown profile: %q", profile)
	}
	if env == "" && role == "" {
		return "", nil, errors.New("name the host, or give --env and --role to have it named by the naming rules")
	}
	r := manifest.ProfileNamingRules(m, profile)
	code := manifest.ProfileCode(m, profile)
	if code == "" && naming.Required(r.Alias, "code") {
		return "", nil, fmt.Errorf("profile %s has no code to name its servers by; set one (%s ssh profile edit %s --code <code>), "+
			"or let profiles without one use their name (%s ssh naming set --code-fallback profile)",
			profile, brand.Command, profile, brand.Command)
	}
	alias, err := r.Generate(map[string]string{"env": env, "role": role, "code": code}, editor.TakenAliases(m))
	if err != nil {
		return "", nil, err
	}
	var tags []string
	for _, v := range []string{role, env} {
		if v != "" {
			tags = append(tags, v)
		}
	}
	return alias, tags, nil
}

// reportKept lists the hosts whose key name a naming change kept.
func reportKept(c *cobra.Command, kept []string) {
	for _, h := range kept {
		say(c, "  kept its key name: %s", h)
	}
}

func appendMissing(list []string, vals ...string) []string {
	for _, v := range vals {
		found := false
		for _, x := range list {
			found = found || x == v
		}
		if !found {
			list = append(list, v)
		}
	}
	return list
}

// newNamingCmd is `ssh naming`: the rules servers and keys are named by.
func newNamingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "naming",
		Short: "Show, check and set how servers and their keys are named",
		Long: "The naming rules live in the manifest's defaults.naming, a profile's own naming\n" +
			"over them, and each profile's code name in profiles.<name>.code:\n\n" +
			"  alias          the alias template, {code}-{env}-{role}-{nn} by default;\n" +
			"                 {env?} is optional, {nnn} is three digits\n" +
			"  key_name       the key-name template, {profile}_{alias}-{algo} by default\n" +
			"  envs, roles    the values {env} and {role} may take (empty: any)\n" +
			"  enforce        off, warn or error, when a host is added\n" +
			"  start          the first number, 0 (00) or 1 (01)\n" +
			"  code_fallback  a profile without a code: none, or profile (its name)\n\n" +
			"`host add <profile> --env prd --role web` names a server by them. No change to\n" +
			"them, or to a code, renames a key already minted.",
	}
	cmd.AddCommand(newNamingShowCmd(), newNamingCheckCmd(), newNamingSetCmd(), newNamingKeyTemplateCmd())
	return cmd
}

func newNamingShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Short:   "Show the naming rules in effect, and each profile's code and own rules",
		Example: "  " + brand.Command + " ssh naming show",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			printRules(out, manifest.NamingRules(m), "")
			for _, name := range m.ProfileNames() {
				pr := m.Profiles[name]
				code := manifest.ProfileCode(m, name)
				switch {
				case code == "":
					code = "-"
				case pr.Code == "":
					code += " (its name)"
				}
				_, _ = fmt.Fprintf(out, "\n%-20s code %s\n", name, code)
				if pr.Naming != nil {
					printRules(out, manifest.ProfileNamingRules(m, name), "  ")
				}
			}
			return nil
		},
	}
}

func printRules(out io.Writer, r naming.Rules, indent string) {
	_, _ = fmt.Fprintf(out, "%salias          %s\n%skey_name       %s\n%senvs           %s\n%sroles          %s\n"+
		"%senforce        %s\n%sstart          %02d\n%scode_fallback  %s\n",
		indent, r.Alias, indent, r.KeyName, indent, orAny(r.Envs), indent, orAny(r.Roles),
		indent, r.Enforce, indent, r.Start, indent, r.CodeFallback)
}

func orAny(list []string) string {
	if len(list) == 0 {
		return "any"
	}
	return strings.Join(list, ", ")
}

func newNamingCheckCmd() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "List the hosts whose alias does not follow the naming rules",
		Long: "List every server whose alias does not follow the naming rules. Hosts named before\n" +
			"the rules keep their names, so this is a report, not an error, unless --strict.\n" +
			"VM hosts are devboxes, not servers, and are not checked.",
		Example: "  " + brand.Command + " ssh naming check\n" +
			"  " + brand.Command + " ssh naming check --strict",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			n := 0
			for _, name := range m.ProfileNames() {
				for _, h := range m.Profiles[name].Hosts {
					if h.VM != nil {
						continue
					}
					if v := manifest.NamingViolation(m, name, h.Alias); v != "" {
						_, _ = fmt.Fprintf(c.OutOrStdout(), "%s: %s\n", name, v)
						n++
					}
				}
			}
			if n == 0 {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "every server follows the naming rules")
				return nil
			}
			if strict {
				return fmt.Errorf("%d host(s) do not follow the naming rules", n)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "exit non-zero when any host breaks the rules")
	return cmd
}

func newNamingSetCmd() *cobra.Command {
	var alias, enforce, fallback, profile string
	var envs, roles []string
	var start int
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the naming rules, for every profile or for one",
		Long: "Set the naming rules: the manifest's (defaults.naming), or with --profile that\n" +
			"profile's own, which override the manifest's field by field. Only the flags given\n" +
			"change; an empty value (--alias '') goes back to inheriting. Every existing host\n" +
			"keeps its key name, written into the manifest when a change would alter it.",
		Example: "  " + brand.Command + " ssh naming set --envs prd,stg,dev --enforce warn\n" +
			"  " + brand.Command + " ssh naming set --profile adelsaiq --alias '{code}-{role}-{nn}' --start 1\n" +
			"  " + brand.Command + " ssh naming set --code-fallback profile",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			changed := false
			for _, f := range []string{"alias", "envs", "roles", "enforce", "start", "code-fallback"} {
				changed = changed || c.Flags().Changed(f)
			}
			if !changed {
				return errors.New("nothing to set: give --alias, --envs, --roles, --enforce, --start or --code-fallback")
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			kept, err := editor.SetNaming(editor.New(p), profile, func(n *manifest.Naming) {
				if c.Flags().Changed("alias") {
					n.Alias = alias
				}
				if c.Flags().Changed("envs") {
					n.Envs = nilIfEmpty(envs)
				}
				if c.Flags().Changed("roles") {
					n.Roles = nilIfEmpty(roles)
				}
				if c.Flags().Changed("enforce") {
					n.Enforce = enforce
				}
				if c.Flags().Changed("start") {
					n.Start = start
				}
				if c.Flags().Changed("code-fallback") {
					n.CodeFallback = fallback
				}
			})
			if err != nil {
				return err
			}
			where := "every profile"
			if profile != "" {
				where = profile
			}
			say(c, "✓ naming rules saved for %s; `%s ssh naming show` lists them", where, brand.Command)
			reportKept(c, kept)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "set this profile's own rules instead of the manifest's")
	cmd.Flags().StringVar(&alias, "alias", "", "the alias template, from {code}, {env}, {role} and a number such as {nn}; {env?} is optional")
	cmd.Flags().StringSliceVar(&envs, "envs", nil, "the environments {env} may be, comma-separated (empty: any)")
	cmd.Flags().StringSliceVar(&roles, "roles", nil, "the roles {role} may be, comma-separated (empty: any)")
	cmd.Flags().StringVar(&enforce, "enforce", "", "off, warn or error, when a host is added")
	cmd.Flags().IntVar(&start, "start", 0, "the first number: 0 (00) or 1 (01)")
	cmd.Flags().StringVar(&fallback, "code-fallback", "", "the {code} of a profile with none of its own: none or profile (its name)")
	return cmd
}

func nilIfEmpty(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return list
}

func newNamingKeyTemplateCmd() *cobra.Command {
	var yes bool
	var profile string
	cmd := &cobra.Command{
		Use:   "key-template <template>",
		Short: "Set how new keys are named, keeping every existing key's name",
		Long: "Set defaults.naming.key_name: how the key of a new host is named, from\n" +
			"{profile}, {alias}, {code}, {env}, {role}, {nn} and {algo}. It starts with\n" +
			"{profile}_ and ends with -{algo}, which is how a key's profile and type are read\n" +
			"back. Every host the new template would rename keeps the key name it has now,\n" +
			"written into the manifest as its key_name, so no key is renamed or left behind;\n" +
			"only hosts added from now on follow the template.",
		Example: "  " + brand.Command + " ssh naming key-template '{profile}_{code}_{env}_{role}_{nn}-{algo}'\n" +
			"  " + brand.Command + " ssh naming key-template '{profile}_{alias}-{algo}' --yes",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if err := naming.CheckKeyName(args[0]); err != nil {
				return err
			}
			if err := confirmChange(c, "New keys will be named "+args[0]+"; existing hosts keep their key names.", yes); err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			kept, err := editor.SetKeyTemplate(editor.New(p), profile, args[0])
			if err != nil {
				return err
			}
			say(c, "✓ new keys are named %s", args[0])
			reportKept(c, kept)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	cmd.Flags().StringVar(&profile, "profile", "", "set this profile's own key template instead of the manifest's")
	return cmd
}
