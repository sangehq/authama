package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/editor"
)

const proxyFlagHelp = "how ssh reaches the host: cloudflared (Cloudflare Access: ssh runs " +
	"'<cloudflared> access ssh --hostname %h'), none (ProxyCommand none: connect directly, " +
	"e.g. over WARP), or off to remove the proxy"

// routeFlags are the flags a host and a route variant share: the proxy,
// ProxyJump, ConnectTimeout, any other ssh option, and the comments
// rendered around the block.
type routeFlags struct {
	proxy, proxyCommand, proxyJump, section, comment string
	connectTimeout                                   int
	options, unset, services, removeServices         []string
}

func (r *routeFlags) register(f *pflag.FlagSet, editing bool) {
	f.StringVar(&r.proxy, "proxy", "", proxyFlagHelp)
	f.StringVar(&r.proxyCommand, "proxy-command", "",
		"a ProxyCommand of your own, e.g. '/usr/bin/nc -X 5 -x proxy:1080 %h %p' (each argument is "+
			"quoted for the shell; none and the cloudflared command become those proxies; '' removes it)")
	f.StringVar(&r.proxyJump, "proxy-jump", "", "connect through this jump host (ProxyJump); '' removes it")
	f.IntVar(&r.connectTimeout, "connect-timeout", 0, "seconds ssh waits to connect (ConnectTimeout); 0 removes it")
	f.StringArrayVar(&r.options, "option", nil,
		"any other ssh_config option as KEY=VALUE, e.g. ServerAliveInterval=30 (repeatable; checked "+
			"against the keywords OpenSSH knows; KEY= removes it)")
	if editing {
		f.StringArrayVar(&r.unset, "unset-option", nil, "remove this ssh option (repeatable)")
	}
	f.StringVar(&r.section, "section", "",
		"a heading rendered as a comment above the block; hosts after it without one sit under it ('' removes it)")
	f.StringVar(&r.comment, "comment", "", "a comment rendered inside the block, under its Host line ('' removes it)")
	f.StringArrayVar(&r.services, "service", nil, "attach this reachability service (a built-in kind or a defaults.services name; repeatable)")
	if editing {
		f.StringArrayVar(&r.removeServices, "remove-service", nil, "detach this reachability service (repeatable)")
	}
}

func (r *routeFlags) fields(c *cobra.Command) (editor.RouteFields, error) {
	f := editor.RouteFields{
		Proxy:          strPtrIf(c, "proxy", r.proxy),
		ProxyCommand:   strPtrIf(c, "proxy-command", r.proxyCommand),
		Section:        strPtrIf(c, "section", r.section),
		Comment:        strPtrIf(c, "comment", r.comment),
		UnsetOptions:   r.unset,
		Services:       r.services,
		RemoveServices: r.removeServices,
	}
	if c.Flags().Changed("proxy-jump") {
		f.SetOptions = append(f.SetOptions, [2]string{"ProxyJump", r.proxyJump})
	}
	if c.Flags().Changed("connect-timeout") {
		if r.connectTimeout < 0 {
			return f, fmt.Errorf("--connect-timeout is a number of seconds (0 removes it)")
		}
		v := ""
		if r.connectTimeout > 0 {
			v = strconv.Itoa(r.connectTimeout)
		}
		f.SetOptions = append(f.SetOptions, [2]string{"ConnectTimeout", v})
	}
	for _, o := range r.options {
		kv, err := editor.ParseOption(o)
		if err != nil {
			return f, err
		}
		if strings.EqualFold(kv[0], "ProxyCommand") {
			return f, fmt.Errorf("a ProxyCommand is set with --proxy or --proxy-command")
		}
		f.SetOptions = append(f.SetOptions, kv)
	}
	return f, nil
}

const variantLong = "A route variant is a second way to reach the same server: one host reached through Cloudflare\n" +
	"Access everywhere, and its WARP twin on the private address, say. `set` creates or updates one and\n" +
	"renders it as its own Host block named <name>-<alias> (warp-simbi-prd-web-00), with the host's user\n" +
	"and key - rotating the key or changing the user moves both routes. The variant sets only what\n" +
	"differs: --hostname, --port, the proxy and options. Its blocks follow the profile's hosts, every\n" +
	"variant of one name together under its --section heading."

func newHostVariantCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "variant", Short: "Add, change or remove a host's route variants (warp-<alias> ...)",
		Long: variantLong}

	var hostname string
	var port int
	var rf routeFlags
	set := &cobra.Command{
		Use:   "set <profile> <alias> <name>",
		Short: "Create or update a route variant of a host",
		Long:  variantLong,
		Args:  cobra.ExactArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			route, err := rf.fields(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			created, err := editor.RoutesOf(editor.New(p)).SetVariant(args[0], args[1], args[2], editor.VariantFields{
				Hostname: strPtrIf(c, "hostname", hostname), Port: intPtrIf(c, "port", port), RouteFields: route})
			if err != nil {
				return err
			}
			verb := "updated"
			if created {
				verb = "added"
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%s variant %s of %s: Host %s\n", verb, args[2], args[1],
				manifest.VariantAlias(args[2], args[1]))
			return applyManifestEdit(c, p)
		},
	}
	set.Flags().StringVarP(&hostname, "hostname", "H", "", "the address this route connects to (the host's when unset)")
	set.Flags().IntVarP(&port, "port", "p", 22, "the port this route connects to (the host's when unset)")
	rf.register(set.Flags(), true)
	cmd.AddCommand(set)

	remove := &cobra.Command{
		Use:   "remove <profile> <alias> <name>",
		Short: "Remove a route variant of a host",
		Args:  cobra.ExactArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			if err := editor.RoutesOf(editor.New(p)).RemoveVariant(args[0], args[1], args[2]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "removed variant %s of %s\n", args[2], args[1])
			return applyManifestEdit(c, p)
		},
	}
	cmd.AddCommand(remove)
	return cmd
}

// newConfigKeepEditsCmd is `ssh config keep-edits`: take what was typed into
// the managed config by hand into the manifest, so a render keeps it.
func newConfigKeepEditsCmd() *cobra.Command {
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "keep-edits",
		Short: "Take hand edits in the managed config into the manifest, so a render keeps them",
		Long: "A render refuses to replace what was typed into a managed config file by hand. This takes it in:\n" +
			"a block for a managed host updates that host (address, extra names, proxy, options, heading,\n" +
			"comment); a block named <name>-<alias> with the user and key of host <alias> becomes its route\n" +
			"variant <name>; any other block becomes a new host of the profile whose banner it sits under,\n" +
			"using the key its IdentityFile names in that profile's directory. A block it cannot take in whole\n" +
			"is left as it is, with the reason. The config is rendered again afterwards.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			ed := editor.New(p)
			preview, err := editor.RoutesOf(ed).KeepEdits(platform.EmitUseKeychain(), true)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if dryRun || len(preview.Adopted) == 0 {
				_, _ = fmt.Fprintln(out, preview.Format(true))
				return nil
			}
			if err := confirmChange(c, preview.Format(true), yes); err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			res, err := editor.RoutesOf(ed).KeepEdits(platform.EmitUseKeychain(), false)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, res.Format(false))
			if len(res.Left) > 0 {
				_, _ = fmt.Fprintln(out, "the config was not rendered: the blocks left above are still hand edits "+
					"(fix them and run this again, or discard them with `authama ssh config render --force`)")
				return nil
			}
			return applyManifestEdit(c, p)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be adopted without changing anything")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "adopt without confirming (implied when there is no terminal)")
	return cmd
}
