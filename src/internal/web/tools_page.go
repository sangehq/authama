package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
)

// toolsPage is /tools: declared tool keys under tools/<tool>/.
type toolsPage struct{ s *Server }

// ToolsView is the /tools page: list, status, and optionally one key
// as `ssh tool show` prints it.
type ToolsView struct {
	Rows      []ToolRow
	Show      *ToolRow
	ShowOf    string
	Declared  int
	Present   int
	Missing   int
	ListCmd   string
	ShowCmd   string
	StatusCmd string
}

// ToolRow is one declared tool key.
type ToolRow struct {
	Profile, Name, Tool, Kind, Path, Ref string
	Present                              bool
}

func (tp toolsPage) handleTools(w http.ResponseWriter, r *http.Request) {
	p := tp.s.opts.Engine.Home()
	v := ToolsView{
		ListCmd:   brand.Command + " ssh tool list",
		StatusCmd: brand.Command + " ssh tool status",
		ShowOf:    strings.TrimSpace(r.URL.Query().Get("show")),
	}
	if v.ShowOf != "" {
		v.ShowCmd = brand.Command + " ssh tool show " + v.ShowOf
	} else {
		v.ShowCmd = brand.Command + " ssh tool show <profile>/<key>"
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil && !os.IsNotExist(err) {
		tp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the manifest", err.Error())
		return
	}
	if err == nil {
		for _, pname := range m.ProfileNames() {
			for _, k := range m.Profiles[pname].Keys {
				if k.Role != manifest.RoleTool && k.Tool == "" {
					continue
				}
				ref := manifest.KeyRef{Profile: pname, KeyName: k.Name}
				path := m.KeyPath(p.SSHDir, ref)
				_, e := os.Stat(path)
				rel, _ := filepath.Rel(p.SSHDir, path)
				row := ToolRow{
					Profile: pname, Name: k.Name, Tool: k.Tool, Kind: k.ToolKind,
					Ref:  pname + "/" + k.Name,
					Path: "~/.ssh/" + filepath.ToSlash(rel), Present: e == nil,
				}
				v.Rows = append(v.Rows, row)
				v.Declared++
				if row.Present {
					v.Present++
				} else {
					v.Missing++
				}
				if v.ShowOf != "" && (row.Ref == v.ShowOf || row.Name == v.ShowOf) && v.Show == nil {
					r := row
					v.Show = &r
				}
			}
		}
	}
	tp.s.render(w, http.StatusOK, "tools", tp.s.newView(r, "tools", "Tools", v))
}
