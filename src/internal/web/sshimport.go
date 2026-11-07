package web

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// ImportSource is one ssh config the web offers to import, chosen by Key. A
// form names the key, never the path, so no path arrives from a browser.
type ImportSource struct {
	Key   string
	Label string
	Path  string
}

// ImportView is /import: where an import could read from, and whether it
// would replace a manifest that has profiles.
type ImportView struct {
	Sources  []ImportSource
	Profiles int
}

// importSources are the ssh configs on this machine that import can read:
// the managed ~/.ssh's config, and a config home's ssh/config (as
// `ssh import ~/.config` reads it). Only those that exist are offered.
func importSources(p paths.Paths) []ImportSource {
	var out []ImportSource
	if c := filepath.Join(p.SSHDir, "config"); isRegular(c) {
		out = append(out, ImportSource{Key: "ssh", Label: "This machine's ssh config", Path: c})
	}
	if dir := configHome(); dir != "" && isRegular(filepath.Join(dir, "ssh", "config")) {
		out = append(out, ImportSource{Key: "config-home", Label: "The config home's ssh/config", Path: dir})
	}
	return out
}

// importSource is the source a form named, or false for one this machine
// does not have.
func importSource(p paths.Paths, key string) (ImportSource, bool) {
	for _, s := range importSources(p) {
		if s.Key == key {
			return s, true
		}
	}
	return ImportSource{}, false
}

// configHome is the base config directory as paths resolves it everywhere
// else: an absolute $XDG_CONFIG_HOME, else ~/.config.
func configHome() string {
	return paths.XDGConfigHome(nil)
}

func isRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// importPage serves /import, kept off pages, which the layering test holds
// to its ceiling of methods.
type importPage struct{ s *Server }

func (ip importPage) handleImport(w http.ResponseWriter, r *http.Request) {
	h := pages(ip)
	p := h.s.opts.Engine.Home()
	v := ImportView{Sources: importSources(p)}
	if m, err := manifest.Load(p.Manifest()); err == nil {
		v.Profiles = len(m.Profiles)
	}
	h.s.render(w, http.StatusOK, "import", h.s.newView(r, "profiles", "Import an ssh config", v))
}
