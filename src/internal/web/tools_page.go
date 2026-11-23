package web

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
)

// toolsPage is /tools: declared tool keys under tools/<tool>/.
type toolsPage struct{ s *Server }

// ToolsView is the /tools page.
type ToolsView struct {
	Rows    []ToolRow
	ListCmd string
}

// ToolRow is one declared tool key.
type ToolRow struct {
	Profile, Name, Tool, Kind, Path string
	Present                         bool
}

func (tp toolsPage) handleTools(w http.ResponseWriter, r *http.Request) {
	p := tp.s.opts.Engine.Home()
	v := ToolsView{ListCmd: brand.Command + " ssh tool list"}
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
				v.Rows = append(v.Rows, ToolRow{
					Profile: pname, Name: k.Name, Tool: k.Tool, Kind: k.ToolKind,
					Path: "~/.ssh/" + filepath.ToSlash(rel), Present: e == nil,
				})
			}
		}
	}
	tp.s.render(w, http.StatusOK, "tools", tp.s.newView(r, "tools", "Tools", v))
}
