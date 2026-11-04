package web

import (
	"net/http"
	"runtime"
	"runtime/debug"

	"github.com/simtabi/authama/src/internal/brand"
)

// AboutView is the about page: what this build is and where it keeps things.
type AboutView struct {
	Name, Summary, Command, Version string
	GoVersion, OS, Arch             string
	Revision, BuiltAt               string
	Modified                        bool
	ConfigHome, StateHome, SSHDir   string
	DevRoot                         string
	Repository, Docs, Issues        string
	User, Host                      string
	Licences                        []licence
}

func (h systemPages) handleAbout(w http.ResponseWriter, r *http.Request) {
	p := h.s.opts.Engine.Home()
	v := AboutView{Name: brand.Name, Summary: brand.Summary, Command: brand.Command, Version: h.s.opts.Version,
		GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH,
		ConfigHome: p.ConfigDir, StateHome: p.State(), SSHDir: p.SSHDir, DevRoot: p.DevRoot,
		Repository: "https://github.com/simtabi/authama",
		Docs:       "https://opensource.simtabi.com/documentation/simtabi/authama/",
		Issues:     "https://github.com/simtabi/authama/issues",
		User:       h.s.who.user, Host: h.s.who.host, Licences: h.s.licences()}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				v.Revision = s.Value
			case "vcs.time":
				v.BuiltAt = s.Value
			case "vcs.modified":
				v.Modified = s.Value == "true"
			}
		}
	}
	h.s.render(w, http.StatusOK, "about", h.s.newView(r, "about", "About", v))
}
