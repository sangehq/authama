package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/svchealth"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage reachability services and run read-only health checks",
		Long: "Services are how a host is reached (Cloudflare Access, WARP, Tailscale, a bastion, …).\n" +
			"They attach to hosts and route variants (`ssh host add --service`, `ssh host edit --service`),\n" +
			"not to key folders. Health checks only read status; they never start a tunnel, change WARP, or open a browser.",
	}

	var listJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List built-in kinds and catalog entries",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			type row struct {
				Name    string `json:"name"`
				Kind    string `json:"kind"`
				Binary  string `json:"binary,omitempty"`
				Label   string `json:"label,omitempty"`
				Catalog bool   `json:"catalog"`
			}
			var rows []row
			seen := map[string]bool{}
			for _, s := range m.Defaults.Services {
				n := s.Ref()
				seen[n] = true
				rows = append(rows, row{Name: n, Kind: s.Kind, Binary: s.Binary, Label: s.Label, Catalog: true})
			}
			for _, k := range manifest.BuiltinServiceKinds() {
				if seen[k] {
					continue
				}
				rows = append(rows, row{Name: k, Kind: k})
			}
			if listJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"version": 1, "services": rows})
			}
			for _, r := range rows {
				src := "built-in"
				if r.Catalog {
					src = "catalog"
				}
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%s  %s  %s\n", r.Name, r.Kind, src)
			}
			return nil
		},
	}
	list.Flags().BoolVar(&listJSON, "json", false, "machine-readable output")
	cmd.AddCommand(list)

	show := &cobra.Command{
		Use:   "show <name>",
		Short: "Show one service and the hosts that attach it",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			name := args[0]
			out := map[string]any{"name": name, "kind": manifest.ServiceKind(m, name), "known": manifest.KnownService(m, name)}
			for _, s := range m.Defaults.Services {
				if s.Ref() == name {
					out["catalog"] = s
				}
			}
			var attached []string
			for _, pname := range m.ProfileNames() {
				for _, h := range m.Profiles[pname].Hosts {
					for _, n := range h.Services {
						if n == name {
							attached = append(attached, pname+"/"+h.Alias)
						}
					}
					for _, v := range h.Variants {
						for _, n := range v.Services {
							if n == name {
								attached = append(attached, pname+"/"+v.Name+"-"+h.Alias)
							}
						}
					}
				}
			}
			out["hosts"] = attached
			return json.NewEncoder(c.OutOrStdout()).Encode(out)
		},
	}
	cmd.AddCommand(show)

	var healthJSON bool
	health := &cobra.Command{
		Use:   "health [name]",
		Short: "Read-only health check (never starts a tunnel or changes WARP)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			var reports []svchealth.Report
			if len(args) == 1 {
				reports = []svchealth.Report{svchealth.Check(m, args[0])}
			} else {
				reports = svchealth.CheckAll(m)
			}
			if healthJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"version": 1, "health": reports})
			}
			for _, r := range reports {
				line := r.Name + "  " + r.Kind + "  " + r.Status
				if r.Version != "" {
					line += "  " + r.Version
				}
				if r.Detail != "" {
					line += "  " + r.Detail
				}
				_, _ = fmt.Fprintln(c.OutOrStdout(), line)
			}
			return nil
		},
	}
	health.Flags().BoolVar(&healthJSON, "json", false, "machine-readable output")
	cmd.AddCommand(health)

	var addKind, addBin, addLabel string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a service to the catalog (defaults.services)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if addKind == "" {
				return fmt.Errorf("--kind is required (one of: %s)", strings.Join(manifest.BuiltinServiceKinds(), ", "))
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			spec := manifest.ServiceSpec{Name: args[0], Kind: addKind, Binary: addBin, Label: addLabel}
			if err := editor.New(p).UpsertService(spec); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "added service %s (%s)\n", args[0], addKind)
			return applyManifestEdit(c, p)
		},
	}
	add.Flags().StringVar(&addKind, "kind", "", "built-in kind (cloudflare-access, cloudflare-warp, tailscale, …)")
	add.Flags().StringVar(&addBin, "binary", "", "override the health-check binary name")
	add.Flags().StringVar(&addLabel, "label", "", "a label shown in the UI")
	cmd.AddCommand(add)

	var editKind, editBin, editLabel string
	edit := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a catalog service",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			var spec manifest.ServiceSpec
			found := false
			for _, s := range m.Defaults.Services {
				if s.Ref() == args[0] {
					spec, found = s, true
					break
				}
			}
			if !found {
				return fmt.Errorf("no catalog service %q (see `%s ssh service list`)", args[0], brand.Command)
			}
			if c.Flags().Changed("kind") {
				spec.Kind = editKind
			}
			if c.Flags().Changed("binary") {
				spec.Binary = editBin
			}
			if c.Flags().Changed("label") {
				spec.Label = editLabel
			}
			if spec.Name == "" {
				spec.Name = args[0]
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			if err := editor.New(p).UpsertService(spec); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "edited service %s\n", args[0])
			return applyManifestEdit(c, p)
		},
	}
	edit.Flags().StringVar(&editKind, "kind", "", "built-in kind")
	edit.Flags().StringVar(&editBin, "binary", "", "override the health-check binary name")
	edit.Flags().StringVar(&editLabel, "label", "", "a label shown in the UI")
	cmd.AddCommand(edit)

	var rmYes bool
	rm := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a catalog service",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if err := confirmChange(c, "Remove catalog service "+args[0]+"? Hosts that still attach it will need a later edit.", rmYes); err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			if err := editor.New(p).RemoveService(args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "removed service %s\n", args[0])
			return applyManifestEdit(c, p)
		},
	}
	rm.Flags().BoolVar(&rmYes, "yes", false, "do not ask")
	cmd.AddCommand(rm)

	var statusJSON bool
	status := &cobra.Command{
		Use:   "status [name]",
		Short: "Alias of health: read-only status of attached services",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// reuse health
			h := health
			if statusJSON {
				_ = h.Flags().Set("json", "true")
			}
			return h.RunE(c, args)
		},
	}
	status.Flags().BoolVar(&statusJSON, "json", false, "machine-readable output")
	cmd.AddCommand(status)
	return cmd
}
