package web

import (
	"net/http"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/sshcfg"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// SSHConfigView is the SSH config page: what `authama ssh config show` prints
// (every file the manifest renders) and what `authama ssh config check` says
// of ~/.ssh against it.
// ConfigHost is one Host in the Include tree, for the source-file table.
type ConfigHost struct {
	Name    string
	File    string
	Line    int
	Managed bool
}

type SSHConfigView struct {
	Rendered string
	Check    string
	InSync   bool
	// HandEdits are the lines typed into authama's files by hand since it
	// last wrote them (HAND-EDIT in config check).
	HandEdits []string
	// Tree is `authama ssh config tree`: the Include graph and each Host's
	// source file. Warnings are Include problems that did not stop the read.
	Tree     string
	Hosts    []ConfigHost
	Warnings []string
	Problem  string // the manifest could not be read or rendered
}

// sshConfigView reads the page for a home. It is not an Engine method: it
// needs nothing the engine holds but the paths.
func sshConfigView(p paths.Paths) SSHConfigView {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return SSHConfigView{Problem: err.Error()}
	}
	svc := configsvc.New(p.SSHDir, m, platform.EmitUseKeychain()).WithState(p.StateDir())
	v := SSHConfigView{}
	if v.Rendered, err = svc.Show(""); err != nil {
		return SSHConfigView{Problem: err.Error()}
	}
	res, err := svc.Check(true)
	if err != nil {
		v.Problem = err.Error()
		return v
	}
	v.Check, v.InSync = res.Format(), res.InSync()
	for _, h := range res.HandEdits {
		v.HandEdits = append(v.HandEdits, h.String())
	}
	if tree, err := sshcfg.LoadUser(p.SSHDir); err == nil && tree != nil {
		v.Tree = tree.Format()
		for _, w := range tree.Warnings {
			v.Warnings = append(v.Warnings, w.Message)
		}
		for _, h := range tree.Hosts {
			name := h.Alias
			if name == "" {
				if len(h.Patterns) == 0 {
					continue
				}
				name = strings.Join(h.Patterns, " ")
			} else if len(h.Patterns) > 1 {
				name = strings.Join(h.Patterns, " ")
			}
			v.Hosts = append(v.Hosts, ConfigHost{Name: name, File: sshcfg.Display(h.File, p.SSHDir), Line: h.Line, Managed: h.Managed})
		}
	}
	return v
}

func (h systemPages) handleSSHConfig(w http.ResponseWriter, r *http.Request) {
	h.s.render(w, http.StatusOK, "sshconfig", h.s.newView(r, "sshconfig", "SSH config", sshConfigView(h.s.opts.Engine.Home())))
}
