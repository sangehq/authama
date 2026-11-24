package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
)

// toolRow is one tool key as `ssh tool list --json` prints it.
type toolRow struct {
	Profile string `json:"profile"`
	Name    string `json:"name"`
	Tool    string `json:"tool"`
	Kind    string `json:"tool_kind,omitempty"`
	Path    string `json:"path"`
	Present bool   `json:"present"`
}

func newToolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "List and inspect tool keys (git signing, docker, …)",
		Long: "Tool keys live in the shared folder ~/.ssh/profiles/tools/<tool>/, apart from host\n" +
			"keys in each profile's folder; each stays declared under the profile that owns it. Moving a\n" +
			"key there is `ssh structure classify` and approve; this command lists what the manifest declares.",
	}
	var jsonOut bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List declared tool keys",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			rows, err := listToolRows(p.Manifest(), p.SSHDir)
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"version": 1, "tools": rows})
			}
			if len(rows) == 0 {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "no tool keys declared (see `"+brand.Command+" ssh structure classify`)")
				return nil
			}
			for _, r := range rows {
				mark := "missing"
				if r.Present {
					mark = "ok"
				}
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%s  %s/%s  tools/%s  %s\n", mark, r.Profile, r.Name, r.Tool, r.Path)
			}
			return nil
		},
	}
	list.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output")
	cmd.AddCommand(list)

	show := &cobra.Command{
		Use:   "show <profile>/<key>",
		Short: "Show one tool key",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			rows, err := listToolRows(p.Manifest(), p.SSHDir)
			if err != nil {
				return err
			}
			want := strings.TrimSpace(args[0])
			for _, r := range rows {
				if r.Profile+"/"+r.Name == want || r.Name == want {
					return json.NewEncoder(c.OutOrStdout()).Encode(r)
				}
			}
			return fmt.Errorf("no tool key %q (see `%s ssh tool list`)", want, brand.Command)
		},
	}
	cmd.AddCommand(show)

	var statusJSON bool
	status := &cobra.Command{
		Use:   "status",
		Short: "Show whether each declared tool key is present on disk",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			rows, err := listToolRows(p.Manifest(), p.SSHDir)
			if err != nil {
				return err
			}
			if statusJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"version": 1, "tools": rows})
			}
			ok, missing := 0, 0
			for _, r := range rows {
				if r.Present {
					ok++
				} else {
					missing++
				}
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%d tool key(s) declared, %d present, %d missing\n", len(rows), ok, missing)
			for _, r := range rows {
				if !r.Present {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "  missing  %s/%s  %s\n", r.Profile, r.Name, r.Path)
				}
			}
			return nil
		},
	}
	status.Flags().BoolVar(&statusJSON, "json", false, "machine-readable output")
	cmd.AddCommand(status)
	return cmd
}

func listToolRows(manifestPath, sshDir string) ([]toolRow, error) {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, err
	}
	var rows []toolRow
	for _, pname := range m.ProfileNames() {
		for _, k := range m.Profiles[pname].Keys {
			if k.Role != manifest.RoleTool && k.Tool == "" {
				continue
			}
			ref := manifest.KeyRef{Profile: pname, KeyName: k.Name}
			path := m.KeyPath(sshDir, ref)
			_, err := os.Stat(path)
			rel, _ := filepath.Rel(sshDir, path)
			rows = append(rows, toolRow{
				Profile: pname, Name: k.Name, Tool: k.Tool, Kind: k.ToolKind,
				Path: "~/.ssh/" + filepath.ToSlash(rel), Present: err == nil,
			})
		}
	}
	return rows, nil
}
