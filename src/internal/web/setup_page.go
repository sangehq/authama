package web

import (
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/version"
)

// setupPage is /setup: `authama setup export` and `setup import` for the
// secrets-free setup document. The document downloads from GET
// /setup/export.json; an import is pasted and goes through the change preview
// (action setup-import) like every other change. The encrypted full export -
// private keys and token values - is made and imported at a terminal only, as
// the page says: a private key or a token value never reaches the browser.
type setupPage struct{ s *Server }

// SetupView is the /setup page.
type SetupView struct {
	Profiles  int
	Keys      []string // profile/key the manifest declares; the document carries none
	EnvNames  []string // the variables tokens are read from; never a value
	Tokens    bool     // tokens.json is in the document
	Settings  bool     // authama.json (without its paths) is in the document
	Providers bool
	Download  string
	// The same at a terminal.
	ExportCmd, EncryptCmd, ImportCmd, ImportFullCmd string
}

func (sp setupPage) handleSetup(w http.ResponseWriter, r *http.Request) {
	d, err := setupxfer.Build(sp.s.opts.Engine.Home(), version.Version, time.Now())
	if err != nil {
		sp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	v := SetupView{Profiles: len(d.Profiles), Keys: d.Keys, EnvNames: d.EnvNames,
		Tokens: len(d.Tokens) > 0, Settings: len(d.Settings) > 0, Providers: len(d.Providers) > 0,
		Download:      "/setup/export.json",
		ExportCmd:     brand.Command + " setup export --out setup.json",
		EncryptCmd:    brand.Command + " setup export --encrypt --out setup.age",
		ImportCmd:     brand.Command + " setup import setup.json --dry-run",
		ImportFullCmd: brand.Command + " setup import setup.age --dry-run"}
	sp.s.render(w, http.StatusOK, "setup", sp.s.newView(r, "setup", "Move setup", v))
}

// handleExport downloads the setup document. It holds no secret.
func (sp setupPage) handleExport(w http.ResponseWriter, r *http.Request) {
	d, err := setupxfer.Build(sp.s.opts.Engine.Home(), version.Version, time.Now())
	if err != nil {
		sp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	body, err := d.Marshal()
	if err != nil {
		sp.s.renderError(w, r, http.StatusInternalServerError, "Could not write the setup document", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+brand.Command+`-setup.json"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// setupImportChange is the setup-import action: a pasted setup document,
// checked as `setup import` checks a file, merged under on_conflict. A
// manifest with risky SSH options is refused: allowing one is for a terminal.
func setupImportChange(f url.Values, errs []error) (change.Change, []error) {
	body := f.Get("body")
	policy, err := setupxfer.ParsePolicy(f.Get("on_conflict"))
	if err != nil {
		errs = append(errs, badForm("%s", err.Error()))
	}
	if !utf8.ValidString(body) {
		return change.Change{}, append(errs, badForm("the setup document is not UTF-8 text"))
	}
	d, err := setupxfer.Parse([]byte(body))
	if err != nil {
		return change.Change{}, append(errs, badForm("%s", err.Error()))
	}
	if risky := d.Risky(); len(risky) > 0 {
		var opts []string
		for _, r := range risky {
			opts = append(opts, r.String())
		}
		return change.Change{}, append(errs, badForm("the document's manifest sets SSH options that could redirect or weaken your connections (%s); "+
			"a document does not prove who made it. If you set them yourself, import it at a terminal with `%s setup import FILE --allow-risky-options`",
			strings.Join(opts, "; "), brand.Command))
	}
	return change.SetupImport(d, nil, policy), errs
}
