package web

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/exports"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/snapshots"
	"github.com/simtabi/authama/src/internal/util/csvout"
)

// tokenFilterParam is the history filter for one token, as target/host.
// It is not "token": that is the sign-in link's parameter, and the auth
// check reads it on every request.
const tokenFilterParam = "credential"

// routes registers every page. Each takes names from the path, the query or
// the form, never a filesystem path, and the engine resolves each name against
// what exists. Only the POST routes change anything - the changes and the jobs
// (see jobs.go) - and --read-only registers none of them, nor the job pages.
func (s *Server) routes() http.Handler {
	h, jh := pages{s}, jobPages{s}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/{hash}/{path...}", s.serveAsset)
	mux.HandleFunc("GET /unlock", ownerGate{s}.handleUnlock)
	mux.HandleFunc("POST /unlock", ownerGate{s}.handleUnlock)
	mux.HandleFunc("GET /{$}", h.handleOverview)
	mux.HandleFunc("GET /profiles", h.handleProfiles)
	mux.HandleFunc("GET /naming", namingPage{s}.handleNaming)
	mux.HandleFunc("GET /hosts", h.handleHosts)
	mux.HandleFunc("GET /hosts/{alias}", h.handleHost)
	mux.HandleFunc("GET /net", netPage{s}.handleNet)
	mux.HandleFunc("GET /import", importPage{s}.handleImport)
	mux.HandleFunc("GET /validate", validatePage{s}.handleValidate)
	mux.HandleFunc("GET /keys", h.handleKeys)
	mux.HandleFunc("GET /keys/{profile}/{key}", h.handleKey)
	mux.HandleFunc("GET /snapshots", h.handleSnapshots)
	mux.HandleFunc("GET /snapshots/{id}", h.handleSnapshot)
	mux.HandleFunc("GET /history", h.handleHistory)
	mux.HandleFunc("GET /history/{op}", h.handleOp)
	mux.HandleFunc("GET /doctor", h.handleDoctor)
	mux.HandleFunc("GET /vms", h.handleVMs)
	mux.HandleFunc("GET /audit", h.handleAudit)
	mux.HandleFunc("GET /tokens", h.handleTokens)
	cl := checklistPages{s}
	mux.HandleFunc("GET /checklists", cl.handleList)
	mux.HandleFunc("GET /checklist", cl.handleItem)
	mux.HandleFunc("GET /security", ownerPages{s}.handleSecurity)
	mux.HandleFunc("GET /repair", ownerPages{s}.handleRepair)
	sp := systemPages{s: s}
	mux.HandleFunc("GET /settings", sp.handleSettings)
	mux.HandleFunc("GET /ssh-config", sp.handleSSHConfig)
	mux.HandleFunc("GET /events", sp.handleEvents)
	mux.HandleFunc("GET /about", sp.handleAbout)
	mux.HandleFunc("GET /export", exportAPI{s}.handlePage)
	// Restarting changes nothing in the home, so read-only servers have it too.
	mux.HandleFunc("POST /server/restart", restartAPI{s}.handleRestart)
	mux.HandleFunc("GET /exports/run", exportAPI{s}.handleRun)
	mux.HandleFunc("GET /exports/{name}", h.handleExport)
	mux.HandleFunc("GET /api/dataset", exportAPI{s}.handleDataset)
	mux.HandleFunc("GET /api/export/selections", exportSelectionsAPI{s}.handleList)
	mux.HandleFunc("GET /exports/compare", exportSelectionsAPI{s}.handleCompare)
	mux.HandleFunc("GET /about/licenses", h.handleLicences)
	if !s.opts.ReadOnly {
		mux.HandleFunc("POST /api/export/selections", exportSelectionsAPI{s}.handleSave)
		mux.HandleFunc("POST /api/export/selections/delete", exportSelectionsAPI{s}.handleDelete)
		// A tick, or starting a token's revocation, is kept in the state
		// directory and touches nothing under ~/.ssh: no preview.
		mux.HandleFunc("POST /checklist/tick", checklistPages{s}.handleTick)
		mux.HandleFunc("POST /checklist/revoking", checklistPages{s}.handleRevoking)
		mux.HandleFunc("POST /changes/preview", h.handlePreview)
		mux.HandleFunc("POST /changes/apply", h.handleApply)
		mux.HandleFunc("POST /jobs/review", jh.handleJobReview)
		mux.HandleFunc("POST /jobs", jh.handleJobStart)
		mux.HandleFunc("GET /jobs/{id}", jh.handleJob)
		mux.HandleFunc("GET /jobs/{id}/events", jh.handleJobEvents)
		mux.HandleFunc("POST /jobs/{id}/answer", jh.handleJobAnswer)
		mux.HandleFunc("POST /jobs/{id}/cancel", jh.handleJobCancel)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.renderError(w, r, http.StatusMethodNotAllowed, "Not a change",
				"Changes are made from the forms on these pages, or with the authama command.")
			return
		}
		s.renderError(w, r, http.StatusNotFound, "Not found", "There is no page at this address.")
	})
	return mux
}

// snapshotsPage is the snapshots and the encrypted bundles beside them.
type snapshotsPage struct {
	Snapshots []SnapshotView
	Bundles   []BundleView
}

// pages holds the page handlers, kept off Server so neither type grows into
// the facade cmd/authama's layering test guards against.
type pages struct{ s *Server }

// fail renders an engine error: a name that matched nothing is a 404, and
// anything else is a 500 with the message, which is the CLI's own wording.
func (h pages) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		h.s.renderError(w, r, http.StatusNotFound, "Not found", "Nothing by that name is in this authama home.")
		return
	}
	h.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
}

// dashboard is the overview page: the menu's rows and counts, and a card each
// for keys, tokens, snapshots, profiles and the newest file events.
type dashboard struct {
	Overview
	KeysOnDisk, Reserved, Loose int
	Tokens, TokenFiles          int
	TokenTools                  int // tools authama writes a credential file for
	TokensFailed, TokenConfig   bool
	Snapshots, SnapshotRetain   int
	SnapshotsWithKeys           int // from before key exclusion: they hold private keys
	FirstSnapshot, LastSnapshot time.Time
	ProfileRows                 []ProfileView
	MaxHosts                    int
	Events                      []EventRow
}

func (h pages) handleOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := h.s.opts.Engine.Overview(h.s.now())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.scanEvents()
	d := dashboard{Overview: ov}
	if kv, err := h.s.opts.Engine.Keys(); err == nil {
		for _, k := range kv.Rows {
			if k.PrivateOnDisk {
				d.KeysOnDisk++
			}
		}
		d.Reserved, d.Loose = len(kv.Reserved), len(kv.Loose)
	}
	if tv, err := h.s.opts.Engine.Tokens(); err == nil {
		d.Tokens, d.TokensFailed, d.TokenConfig = len(tv.Items), tv.Failed, tv.HasConfig
		d.TokenTools = len(tv.Status.Targets)
		for _, t := range tv.Status.Targets {
			if t.Exists {
				d.TokenFiles++
			}
		}
	}
	if snaps, err := h.s.opts.Engine.Snapshots(); err == nil {
		d.Snapshots, d.SnapshotRetain = len(snaps), h.s.opts.Retain
		for _, s := range snaps {
			if s.KeyBearing {
				d.SnapshotsWithKeys++
			}
			if d.FirstSnapshot.IsZero() || s.Taken.Before(d.FirstSnapshot) {
				d.FirstSnapshot = s.Taken
			}
			if s.Taken.After(d.LastSnapshot) {
				d.LastSnapshot = s.Taken
			}
		}
	}
	if ps, err := h.s.opts.Engine.Profiles(); err == nil {
		d.ProfileRows = ps
		for _, p := range ps {
			d.MaxHosts = max(d.MaxHosts, p.Hosts)
		}
	}
	d.Events = h.s.recentEvents(events.Filter{Limit: 6})
	h.s.render(w, http.StatusOK, "overview", h.s.newView(r, "overview", "Overview", d))
}

func (h pages) handleProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := h.s.opts.Engine.Profiles()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "profiles", h.s.newView(r, "profiles", "Profiles", ps))
}

func (h pages) handleHosts(w http.ResponseWriter, r *http.Request) {
	groups, err := h.s.opts.Engine.Hosts()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "hosts", h.s.newView(r, "hosts", "Hosts", groups))
}

func (h pages) handleHost(w http.ResponseWriter, r *http.Request) {
	host, err := h.s.opts.Engine.Host(r.PathValue("alias"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "host", h.s.newView(r, "hosts", host.Detail.Alias, host))
}

func (h pages) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.s.opts.Engine.Keys()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "keys", h.s.newView(r, "keys", "Keys", keys))
}

func (h pages) handleKey(w http.ResponseWriter, r *http.Request) {
	ref, ok := splitRef(r.PathValue("profile") + "/" + r.PathValue("key"))
	if !ok {
		h.fail(w, r, ErrNotFound)
		return
	}
	k, err := h.s.opts.Engine.Key(ref)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "key", h.s.newView(r, "keys", ref, k))
}

func (h pages) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	list, err := h.s.opts.Engine.Snapshots()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	bundles, err := h.s.opts.Engine.Bundles()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "snapshots", h.s.newView(r, "snapshots", "Snapshots",
		snapshotsPage{Snapshots: list, Bundles: bundles}))
}

type snapshotPage struct {
	SnapshotDiff
	Others []SnapshotView
}

func (h pages) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	d, err := h.s.opts.Engine.SnapshotDiff(r.PathValue("id"), r.URL.Query().Get("against"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	others, _ := h.s.opts.Engine.Snapshots()
	h.s.render(w, http.StatusOK, "snapshot", h.s.newView(r, "snapshots", d.ID, snapshotPage{SnapshotDiff: d, Others: others}))
}

type historyPage struct {
	Filter history.Filter
	Ops    []history.Op
	More   string // the next page's query, "" on the last page
	Query  url.Values
}

const historyPageSize = 50

func (h pages) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := historyFilter(q)
	limit := historyPageSize
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	f.Limit = limit + 1 // one more than shown says whether there is a next page
	ops, err := h.s.opts.Engine.History(f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := historyPage{Filter: f, Query: q}
	if len(ops) > limit {
		ops = ops[:limit]
		next := url.Values{}
		for k, v := range q {
			next[k] = v
		}
		next.Set("limit", strconv.Itoa(limit+historyPageSize))
		page.More = "/history?" + next.Encode()
	}
	page.Ops = ops
	h.s.render(w, http.StatusOK, "history", h.s.newView(r, "history", "History", page))
}

func (h pages) handleOp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("op")
	if id == "" || strings.Trim(id, "0123456789abcdef") != "" {
		h.fail(w, r, ErrNotFound)
		return
	}
	v, err := h.s.opts.Engine.Op(id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "op", h.s.newView(r, "history", v.Op.Command, v))
}

func (h pages) handleDoctor(w http.ResponseWriter, r *http.Request) {
	rep, err := h.s.opts.Engine.Doctor()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "doctor", h.s.newView(r, "doctor", "Doctor", struct {
		OK     bool
		Report string
	}{rep.OK(), rep.Format()}))
}

func (h pages) handleVMs(w http.ResponseWriter, r *http.Request) {
	// ?check=docker asks the engine behind each running VM's Docker context;
	// without it the page connects to no guest.
	v, err := h.s.opts.Engine.VMs(r.URL.Query().Get("check") == "docker")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "vms", h.s.newView(r, "vms", "VMs", v))
}

func (h pages) handleTokens(w http.ResponseWriter, r *http.Request) {
	v, err := h.s.opts.Engine.Tokens()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "tokens", h.s.newView(r, "tokens", "Auth tokens", v))
}

// systemPages are the pages about the tool and its files rather than the
// home's SSH and token contents: settings, the rendered SSH config, the file
// events and the about page. A type of their own keeps pages under the
// layering guard's method ceiling.
type systemPages struct{ s *Server }

func (h systemPages) handleSettings(w http.ResponseWriter, r *http.Request) {
	p := h.s.opts.Engine.Home()
	var v SettingsView
	if h.s.opts.Settings != nil {
		v = h.s.opts.Settings(p)
	} else {
		_, err := os.Stat(p.SettingsFile())
		v = SettingsView{ConfigHome: p.ConfigDir, StateHome: p.State(), Dotenv: p.EnvFile(),
			Settings: p.SettingsFile(), HasSettings: err == nil}
	}
	h.s.render(w, http.StatusOK, "settings", h.s.newView(r, "settings", "Settings", v))
}

func (h pages) handleAudit(w http.ResponseWriter, r *http.Request) {
	a, err := h.s.opts.Engine.Audit(h.s.now())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.s.render(w, http.StatusOK, "audit", h.s.newView(r, "audit", "Audit", a))
}

// handleExport downloads an export. It writes nothing, so --read-only keeps
// it: it is a page in another format.
func (h pages) handleExport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if preset, ok := enginePreset(name, r.URL.Query()); ok {
		exportAPI(h).runPreset(w, r, preset)
		return
	}
	if h.legacyCSV(w, r, name) {
		return
	}
	if name != ExportFlat && name != ExportTermius {
		h.fail(w, r, ErrNotFound)
		return
	}
	body, err := h.s.opts.Engine.Export(name, r.URL.Query().Get("include_git") == "1")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// The Termius file is a CSV too: shaped like the lists' when asked to be,
	// and written exactly as rendered when not.
	if name == ExportTermius && shapeOf(r.URL.Query()).any() {
		t, err := csvout.Parse(body)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.writeCSV(w, r, name, t, nil)
		return
	}
	ctype := "text/plain; charset=utf-8"
	if name == ExportTermius {
		ctype = "text/csv; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}

// legacyCSV answers one of csvLists the way it was built before the export
// pipeline: from the list's own table, shaped by csvShape. It still serves
// what the pipeline does not take over (a preview for the download dialog,
// and history's host, token, profile and key filters), and the parity test
// holds the pipeline to its bytes. It reports whether name was such a list.
func (h pages) legacyCSV(w http.ResponseWriter, r *http.Request, name string) bool {
	t, ok, err := h.listCSV(name, r.URL.Query())
	if !ok {
		return false
	}
	if err != nil {
		h.fail(w, r, err)
		return true
	}
	h.writeCSV(w, r, brand.Command+"-"+name, t, csvKeyHosts(h.s.opts.Engine, name))
	return true
}

type licence struct {
	Name, Text string
}

func (h pages) handleLicences(w http.ResponseWriter, r *http.Request) {
	h.s.render(w, http.StatusOK, "licenses", h.s.newView(r, "about", "Third-party licences", h.s.licences()))
}

// licences are the bundled front-end assets' licence texts, as embedded.
func (s *Server) licences() []licence {
	var ls []licence
	for _, name := range []string{"tabler", "tabler-icons", "bootstrap", "popper"} {
		if body, ok := s.assets.files["vendor/LICENSES/"+name+".txt"]; ok {
			ls = append(ls, licence{Name: name, Text: string(body)})
		}
	}
	return ls
}

// csvLists are the lists the pages show that download as CSV, by file name:
// the same columns as the CLI's --csv on each.
var csvLists = []string{"hosts.csv", "keys.csv", "tokens.csv", "history.csv", "events.csv",
	"profiles.csv", "snapshots.csv", "deployments.csv"}

// historyFilter reads the history page's filters from its query; the page
// and its CSV both use it, so a download is what the page shows.
func historyFilter(q url.Values) history.Filter {
	return history.Filter{
		Host: q.Get("host"), Profile: q.Get("profile"), Key: q.Get("key"), Token: q.Get(tokenFilterParam),
		Since: q.Get("since"), Until: q.Get("until"), Command: q.Get("command"), Source: q.Get("source"),
	}
}

// eventsFilter reads the events page's filters, keeping only the areas and
// kinds there are; anything else is no filter at all.
func eventsFilter(q url.Values) events.Filter {
	f := events.Filter{Area: strings.TrimSpace(q.Get("area")), Kind: strings.TrimSpace(q.Get("kind"))}
	switch f.Area {
	case "", events.AreaSSH, events.AreaConfig, events.AreaAuth:
	default:
		f.Area = ""
	}
	switch f.Kind {
	case "", events.KindAdded, events.KindChanged, events.KindRemoved, events.KindMode, events.KindStarted:
	default:
		f.Kind = ""
	}
	return f
}

// profileKeys is each profile's key names, in the keys list's order, and
// how many hosts use each key: what profiles.csv needs to keep, and narrow,
// the profiles holding some keys.
func profileKeys(e Engine) (names map[string][]string, hosts map[string]int, err error) {
	kv, err := e.Keys()
	if err != nil {
		return nil, nil, err
	}
	names, hosts = map[string][]string{}, map[string]int{}
	for _, r := range kv.Rows {
		names[r.Ref.Profile] = append(names[r.Ref.Profile], r.Ref.KeyName)
		hosts[r.Ref.KeyName] = len(r.Hosts)
	}
	return names, hosts, nil
}

// csvKeyHosts is how many hosts use each key, for the list that counts them
// per profile (profiles.csv); nil for every other list.
func csvKeyHosts(e Engine, name string) map[string]int {
	if name != "profiles.csv" {
		return nil
	}
	_, hosts, err := profileKeys(e)
	if err != nil {
		return nil
	}
	return hosts
}

// listCSV builds one of csvLists, filtered by q as its page is; ok is false
// for any other name. It is a pages helper, not a handler, so it adds no
// route of its own.
func (h pages) listCSV(name string, q url.Values) (t csvout.Table, ok bool, err error) {
	e := h.s.opts.Engine
	switch name {
	case "hosts.csv":
		groups, err := e.Hosts()
		return exports.Hosts(groups), true, err
	case "keys.csv":
		kv, err := e.Keys()
		return exports.Keys(kv.Rows), true, err
	case "tokens.csv":
		tv, err := e.Tokens()
		return exports.Tokens(tv.Items), true, err
	case "history.csv":
		ops, err := e.History(historyFilter(q))
		return exports.History(ops), true, err
	case "events.csv":
		evs, err := events.List(e.Home(), eventsFilter(q))
		return exports.Events(evs), true, err
	case "profiles.csv":
		pv, err := e.Profiles()
		if err != nil {
			return csvout.Table{}, true, err
		}
		names, _, err := profileKeys(e)
		rows := make([]exports.Profile, 0, len(pv))
		for _, p := range pv {
			rows = append(rows, exports.Profile{Name: p.Name, Category: p.Category, KeyScope: p.KeyScope,
				KeyStore: p.KeyStore, KeyComment: p.KeyComment, Hosts: p.Hosts, Keys: p.Keys, KeyNames: names[p.Name]})
		}
		return exports.Profiles(rows), true, err
	case "snapshots.csv":
		return exports.Snapshots(snapshots.List(e.Home().SnapshotsDir())), true, nil
	case "deployments.csv":
		inv, err := inventory.Load(e.Home().Inventory())
		return exports.Deployments(inv), true, err
	}
	return csvout.Table{}, false, nil
}
