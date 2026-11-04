package web

import (
	"net/http"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/configsvc"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// SSHConfigView is the SSH config page: what `authama ssh config show` prints
// (every file the manifest renders) and what `authama ssh config check` says
// of ~/.ssh against it.
type SSHConfigView struct {
	Rendered string
	Check    string
	InSync   bool
	Problem  string // the manifest could not be read or rendered
}

// sshConfigView reads the page for a home. It is not an Engine method: it
// needs nothing the engine holds but the paths.
func sshConfigView(p paths.Paths) SSHConfigView {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return SSHConfigView{Problem: err.Error()}
	}
	svc := configsvc.New(p.SSHDir, m, platform.EmitUseKeychain())
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
	return v
}

func (h systemPages) handleSSHConfig(w http.ResponseWriter, r *http.Request) {
	h.s.render(w, http.StatusOK, "sshconfig", h.s.newView(r, "sshconfig", "SSH config", sshConfigView(h.s.opts.Engine.Home())))
}
