package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/exportsched"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/log"
)

// exportSchedules is the Schedules card on /export: `authama export schedule
// add|list|remove|run`. Each change asks for the owner PIN within the step-up
// window, since a schedule writes files outside the home. Registering the
// daily job with the system's scheduler (`schedule install`, `uninstall`)
// stays at a terminal: it changes launchd, systemd, cron or schtasks, not
// authama's home.
type exportSchedules struct {
	Rows       []scheduleRow
	Selections []string
	Formats    []string
	Done       string
	Results    []string
	Problem    string
	InstallCmd string
}

type scheduleRow struct {
	Selection, Format, File string
}

func (x exportAPI) schedules() exportSchedules {
	p := x.s.opts.Engine.Home()
	v := exportSchedules{Formats: encode.TableNames(), InstallCmd: brand.Command + " export schedule install"}
	all, err := savedsel.Schedules(p)
	if err != nil {
		v.Problem = err.Error()
	}
	for _, s := range all {
		file, _ := s.File()
		v.Rows = append(v.Rows, scheduleRow{Selection: s.Selection, Format: s.Format, File: file})
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		m = nil
	}
	if saved, err := savedsel.List(p, m); err == nil {
		for _, s := range saved {
			if s.Err == "" && !s.Shadowed {
				v.Selections = append(v.Selections, s.Name)
			}
		}
	}
	return v
}

func (x exportAPI) renderPage(w http.ResponseWriter, r *http.Request, status int, sc exportSchedules) {
	x.s.render(w, status, "export", x.s.newView(r, "export", "Export", exportPage{Grains: exportGrains, Schedules: sc}))
}

// handleSchedule is POST /export/schedules/{act}: add, remove or run.
func (x exportAPI) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if !ownerGate(x).stepUp(w, r) {
		return
	}
	p := x.s.opts.Engine.Home()
	act := r.PathValue("act")
	sel := strings.TrimSpace(r.PostFormValue("selection"))
	format := strings.TrimSpace(r.PostFormValue("format"))
	var done string
	var results []string
	var err error
	switch act {
	case "add":
		folder := strings.TrimSpace(r.PostFormValue("folder"))
		switch {
		case folder == "":
			err = badForm("give the folder to write to")
		case !strings.HasPrefix(folder, "/") && !strings.HasPrefix(folder, "~/") && (len(folder) < 3 || folder[1] != ':'):
			err = badForm("%q: give the folder's full path (or ~/…); the server has no current directory of yours", folder)
		case strings.ContainsFunc(folder, func(r rune) bool { return r < 0x20 || r == 0x7f }):
			err = badForm("the folder holds a control character")
		}
		if err != nil {
			break
		}
		var abs string
		if abs, err = exportsched.Folder(p, expandHome(folder)); err != nil {
			break
		}
		m, lerr := manifest.Load(p.Manifest())
		if lerr != nil {
			m = nil
		}
		if _, err = savedsel.Get(p, m, sel); err != nil {
			break
		}
		if format == "" {
			format = "csv"
		}
		s := savedsel.Schedule{Selection: sel, Format: format, Folder: abs}
		if err = savedsel.AddSchedule(p, s); err == nil {
			file, _ := s.File()
			done = "Scheduled " + sel + " daily to " + file + ". The daily job writes it once `" + brand.Command + " export schedule install` has registered one."
		}
	case "remove":
		var n int
		if n, err = savedsel.RemoveSchedules(p, sel, format); err == nil {
			done = "Removed " + strconv.Itoa(n) + " schedule(s) of " + sel + ". The files written stay."
		}
	case "run":
		var res []exportsched.Result
		if res, err = exportsched.RunAll(p); err == nil {
			failed := 0
			for _, rr := range res {
				if rr.Err != nil {
					failed++
					results = append(results, rr.Schedule.Selection+" ("+rr.Schedule.Format+"): "+rr.Err.Error())
					continue
				}
				results = append(results, "wrote "+strconv.Itoa(rr.Rows)+" row(s) to "+rr.File)
			}
			done = "Wrote every scheduled export."
			if len(res) == 0 {
				done = "There is no schedule to run."
			}
			if failed > 0 {
				done = strconv.Itoa(failed) + " of " + strconv.Itoa(len(res)) + " scheduled export(s) failed."
			}
		}
	default:
		x.s.renderError(w, r, http.StatusNotFound, "Not found", "There is no schedule action "+act+".")
		return
	}
	sc := x.schedules()
	if err != nil {
		sc.Problem = err.Error()
		x.renderPage(w, r, http.StatusBadRequest, sc)
		return
	}
	log.Audit(p.AuditLog(), "export.schedule."+act, log.Field{Key: "selection", Value: sel}, log.Field{Key: "source", Value: "web"})
	sc.Done, sc.Results = done, results
	x.renderPage(w, r, http.StatusOK, sc)
}
