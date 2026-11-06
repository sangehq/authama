package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/naming"
	"github.com/simtabi/authama/src/internal/services/editor"
)

// generatedAlias names a new host of profile by the naming rules, and returns
// the tags it should carry (its role and environment).
func generatedAlias(m *manifest.Manifest, profile, env, role string) (string, []string, error) {
	pr, ok := m.Profiles[profile]
	if !ok {
		return "", nil, fmt.Errorf("unknown profile: %q", profile)
	}
	if env == "" && role == "" {
		return "", nil, errors.New("name the host, or give --env and --role to have it named by the naming rules")
	}
	r := manifest.NamingRules(m)
	parts := map[string]string{"env": env, "role": role, "code": pr.Code}
	for _, t := range naming.Tokens(r.Alias) {
		if t == "code" && pr.Code == "" {
			return "", nil, fmt.Errorf("profile %s has no code to name its servers by; set one: %s ssh profile edit %s --code <code>",
				profile, brand.Command, profile)
		}
	}
	alias, err := r.Generate(parts, editor.TakenAliases(m))
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
		Long: "The naming rules live in the manifest's defaults.naming, and each profile's code\n" +
			"name in profiles.<name>.code:\n\n" +
			"  alias     the alias template, {code}-{env}-{role}-{nn} by default\n" +
			"  key_name  the key-name template, {profile}_{alias}-{algo} by default\n" +
			"  envs      the environments {env} may be (empty: any)\n" +
			"  roles     the roles {role} may be (empty: any)\n" +
			"  enforce   off, warn or error, when a host is added\n\n" +
			"`host add <profile> --env prd --role web` names a server by them.",
	}
	cmd.AddCommand(newNamingShowCmd(), newNamingCheckCmd(), newNamingSetCmd(), newNamingKeyTemplateCmd())
	return cmd
}

func newNamingShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Short:   "Show the naming rules in effect and each profile's code",
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
			r := manifest.NamingRules(m)
			out := c.OutOrStdout()
			_, _ = fmt.Fprintf(out, "alias     %s\nkey_name  %s\nenvs      %s\nroles     %s\nenforce   %s\n\n",
				r.Alias, r.KeyName, orAny(r.Envs), orAny(r.Roles), r.Enforce)
			for _, name := range m.ProfileNames() {
				code := m.Profiles[name].Code
				if code == "" {
					code = "-"
				}
				_, _ = fmt.Fprintf(out, "%-20s code %s\n", name, code)
			}
			return nil
		},
	}
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
	var alias, enforce string
	var envs, roles []string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the alias template, the allowed environments and roles, and how strictly they apply",
		Long: "Set the naming rules in defaults.naming. Only the flags given change. The key-name\n" +
			"template has its own command, `naming key-template`, because changing it needs\n" +
			"the existing hosts' key names kept.",
		Example: "  " + brand.Command + " ssh naming set --envs prd,stg,dev --enforce warn\n" +
			"  " + brand.Command + " ssh naming set --alias '{code}-{env}-{role}-{nn}' --roles web,db,cache",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if !c.Flags().Changed("alias") && !c.Flags().Changed("envs") && !c.Flags().Changed("roles") && !c.Flags().Changed("enforce") {
				return errors.New("nothing to set: give --alias, --envs, --roles or --enforce")
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			err = editor.SetNaming(editor.New(p), func(n *manifest.Naming) {
				if c.Flags().Changed("alias") {
					n.Alias = alias
				}
				if c.Flags().Changed("envs") {
					n.Envs = envs
				}
				if c.Flags().Changed("roles") {
					n.Roles = roles
				}
				if c.Flags().Changed("enforce") {
					n.Enforce = enforce
				}
			})
			if err != nil {
				return err
			}
			say(c, "✓ naming rules saved; `%s ssh naming show` lists them", brand.Command)
			return nil
		},
	}
	cmd.Flags().StringVar(&alias, "alias", "", "the alias template, from {code}, {env}, {role} and {nn}")
	cmd.Flags().StringSliceVar(&envs, "envs", nil, "the environments {env} may be, comma-separated (empty: any)")
	cmd.Flags().StringSliceVar(&roles, "roles", nil, "the roles {role} may be, comma-separated (empty: any)")
	cmd.Flags().StringVar(&enforce, "enforce", "", "off, warn or error, when a host is added")
	return cmd
}

func newNamingKeyTemplateCmd() *cobra.Command {
	var yes bool
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
			pinned, err := editor.SetKeyTemplate(editor.New(p), args[0])
			if err != nil {
				return err
			}
			say(c, "✓ new keys are named %s", args[0])
			for _, h := range pinned {
				say(c, "  kept its key name: %s", h)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
