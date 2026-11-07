package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/renderer"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/dataset"
	"github.com/simtabi/authama/src/internal/services/selection"
	"github.com/simtabi/authama/src/internal/util/encode"
)

// exportAPI serves the export pipeline: the dataset, and any selection in
// any format. Its own type, so the pages type keeps to its size.
//
//	GET /api/dataset?parts=tokens,ops         the dataset as JSON
//	GET /exports/run?preset=hosts&format=json  a built-in selection, as a file
//	GET /exports/run?s=<base64url JSON>&format=yaml
//	GET /exports/run?…&preview=1               JSON: first rows, stages, fields, facets
//
// All are reads, so they stay in read-only mode. A selection is small and
// holds no secret, so it travels in the URL: a download link must be a GET.
type exportAPI struct{ s *Server }

// MaxSelection caps the encoded selection a URL may carry.
const MaxSelection = 16 << 10

// exportPage is what the Export page starts from: the grains it offers.
type exportPage struct {
	Grains []exportGrain
}

type exportGrain struct {
	Name, Label, Icon, Preset string
}

// exportGrains are the Export page's row kinds, in its order; each one's
// preset is the built-in selection of that grain.
var exportGrains = func() []exportGrain {
	out := []exportGrain{
		{selection.GrainHost, "Hosts", "server", ""},
		{selection.GrainKey, "Keys", "key", ""},
		{selection.GrainProfile, "Profiles", "users", ""},
		{selection.GrainToken, "Auth tokens", "cloud-lock", ""},
		{selection.GrainOp, "History", "history", ""},
		{selection.GrainEvent, "Events", "activity", ""},
		{selection.GrainSnapshot, "Snapshots", "archive", ""},
		{selection.GrainDeployment, "Deployments", "route", ""},
	}
	for i := range out {
		for name, sel := range selection.Presets {
			if sel.Grain == out[i].Name {
				out[i].Preset = name
			}
		}
	}
	return out
}()

// handlePage is the Export page: the rows, filters, columns and format of a
// file, previewed as they are chosen. The page's script drives it through
// /exports/run; without the script it lists each preset's download.
func (x exportAPI) handlePage(w http.ResponseWriter, r *http.Request) {
	x.s.render(w, http.StatusOK, "export", x.s.newView(r, "export", "Export", exportPage{Grains: exportGrains}))
}

func (x exportAPI) handleDataset(w http.ResponseWriter, r *http.Request) {
	var parts []string
	if p := strings.TrimSpace(r.URL.Query().Get("parts")); p != "" {
		parts = splitList(p)
	}
	ds, err := dataset.Build(x.s.opts.Engine.Home(), dataset.Options{Parts: parts})
	if err != nil {
		x.fail(w, http.StatusInternalServerError, err)
		return
	}
	x.json(w, ds)
}

// exportSteps names the transforms, the formats a format step takes and the
// aggregates a group computes, from the engine's own lists.
type exportSteps struct {
	Kinds      []string `json:"kinds"`
	Formats    []string `json:"formats"`
	Aggregates []string `json:"aggregates"`
}

// exportPreview is the answer to ?preview=1.
type exportPreview struct {
	Grain   string                            `json:"grain"`
	Columns []string                          `json:"columns"`
	Rows    [][]string                        `json:"rows"`
	Total   int                               `json:"total"` // rows before any filter
	Kept    int                               `json:"kept"`  // rows the file holds
	Stages  []selection.Stage                 `json:"stages"`
	Fields  []string                          `json:"fields"`
	Facets  map[string][]selection.FacetValue `json:"facets"`
	Formats []exportFormat                    `json:"formats"`
	// Defaults are the columns the file has when none are chosen: the grain's,
	// then the fields any transforms wrote. A page starts its list from them.
	Defaults []selection.Column `json:"defaults"`
	// Steps are the transforms a page can add, and their choices.
	Steps exportSteps `json:"steps"`
	// Skipped are the hosts the format asked for (?format=, a host format)
	// leaves out of the file, each with the reason: the preview shows the
	// selection's rows, the file only those the client can take.
	Skipped []string `json:"skipped,omitempty"`
}

type exportFormat struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Extension string `json:"extension"`
	// Hosts marks a host format, which writes host rows only and ignores
	// the columns.
	Hosts bool `json:"hosts,omitempty"`
}

func (x exportAPI) handleRun(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sel, name, err := selectionOf(q.Get("preset"), q.Get("s"))
	if err != nil {
		x.fail(w, http.StatusBadRequest, err)
		return
	}
	if q.Get("preset") != "" {
		sel = shapedPreset(sel, q)
	}
	x.run(w, r, sel, name)
}

// runPreset answers a list's old CSV URL (/exports/hosts.csv) through the
// pipeline: its preset, shaped by the link's options, as CSV, under the
// file name that URL always had.
func (x exportAPI) runPreset(w http.ResponseWriter, r *http.Request, preset string) {
	x.run(w, r, shapedPreset(selection.Presets[preset], r.URL.Query()), preset)
}

// enginePresetParams are the CSV links' options the pipeline answers byte for
// byte as the old lists did, per list, proven by
// TestTheOldCSVURLsAreTheirListsBytes. Every list took the download dialog's
// shape (csvShape); events and history also took their own page's filters,
// and ignored the rest, so another list's filter keeps the old path rather
// than start filtering where it never did.
var enginePresetParams = func() map[string]map[string]bool {
	page := map[string][]string{"events": {"area", "kind"}, "history": {"since", "until", "command", "source"}}
	out := map[string]map[string]bool{}
	for name, sel := range selection.Presets {
		allowed := map[string]bool{"exclude": true, "drop": true}
		// csvShape filters by profile, or by key, only where the list has
		// that column, and ignores the option elsewhere.
		for _, c := range selection.Defaults[sel.Grain] {
			switch h := c.Heading(); {
			case slices.Contains(profileColumns, h):
				allowed["profile"] = true
			case slices.Contains(keyColumns, h), slices.Contains(keyListColumns, h):
				allowed["key"], allowed["narrow"] = true, true
			}
		}
		for _, p := range page[name] {
			allowed[p] = true
		}
		out[name] = allowed
	}
	return out
}()

// enginePreset is the preset an old CSV URL maps onto, when the pipeline
// serves it: one of csvLists, no preview (the download dialog reads the old
// preview's shape), no format (the old URL is CSV), and only options the
// pipeline matches for that list. History's profile and key filter its
// operations by what they touched, which the pipeline's op rows do not carry,
// so those stay on the old path.
func enginePreset(file string, q url.Values) (string, bool) {
	name, isCSV := strings.CutSuffix(file, ".csv")
	if !isCSV || !slices.Contains(csvLists, file) {
		return "", false
	}
	allowed, ok := enginePresetParams[name]
	if !ok {
		return "", false
	}
	for k := range q {
		if !allowed[k] || (name == "history" && (k == "profile" || k == "key")) {
			return "", false
		}
	}
	return name, true
}

func (x exportAPI) run(w http.ResponseWriter, r *http.Request, sel selection.Selection, name string) {
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "csv"
	}
	f, err := encode.Get(format)
	if err != nil {
		x.fail(w, http.StatusBadRequest, err)
		return
	}
	ds, err := dataset.Build(x.s.opts.Engine.Home(), dataset.Options{Parts: selection.Parts(sel.Grain)})
	if err != nil {
		x.fail(w, http.StatusInternalServerError, err)
		return
	}
	res, err := selection.Run(ds, sel)
	if err != nil {
		x.fail(w, http.StatusBadRequest, err)
		return
	}
	if q.Get("preview") == "1" {
		facets, err := selection.Facets(ds, sel.Grain)
		if err != nil {
			x.fail(w, http.StatusBadRequest, err)
			return
		}
		p := exportPreview{Grain: sel.Grain, Columns: res.Columns, Rows: [][]string{}, Stages: res.Stages,
			Fields: res.Fields, Facets: facets, Total: res.Stages[0].Rows, Kept: len(res.Rows),
			Defaults: res.Defaults,
			Steps:    exportSteps{selection.Transforms, selection.Formats, selection.Aggregates}}
		for i, row := range res.Rows {
			if i == PreviewRows {
				break
			}
			p.Rows = append(p.Rows, selection.Cells(row))
		}
		for _, n := range append(encode.TableNames(), encode.HostNames()...) {
			ef, _ := encode.Get(n)
			// A host format writes the hosts themselves: hosts only, untransformed.
			if ef.IsHosts() && (sel.Grain != selection.GrainHost || len(sel.Transforms) > 0) {
				continue
			}
			p.Formats = append(p.Formats, exportFormat{Name: ef.Name, Label: ef.Label, Extension: ef.Extension, Hosts: ef.IsHosts()})
		}
		if f.IsHosts() && sel.Grain == selection.GrainHost && len(sel.Transforms) == 0 {
			exp, err := x.hostExport(ds, sel, f, q)
			if err != nil {
				x.fail(w, http.StatusBadRequest, err)
				return
			}
			p.Skipped = exp.Skipped
		}
		x.json(w, p)
		return
	}
	if f.IsHosts() {
		exp, err := x.hostExport(ds, sel, f, q)
		if err != nil {
			x.fail(w, http.StatusBadRequest, err)
			return
		}
		data, err := encode.Bundle(f, exp)
		if err != nil {
			x.fail(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", f.MediaType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+encode.FileName(f, exp, brand.Command+"-"+name)+`"`)
		_, _ = w.Write(data)
		return
	}
	opts := encode.Options{NoHeader: q.Get("header") == "0", Compact: q.Get("compact") == "1", Shape: q.Get("shape"),
		Title: brand.Command + " " + name}
	var buf bytes.Buffer
	if err := f.Write(&buf, encode.Table{Columns: res.Columns, Rows: res.Rows}, opts); err != nil {
		x.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", f.MediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+brand.Command+"-"+name+"."+f.Extension+`"`)
	_, _ = w.Write(buf.Bytes())
}

// hostExport writes the hosts sel keeps in the host format f, from this
// home's manifest. ?include_git=1 keeps git hosts, as --include-git does.
func (x exportAPI) hostExport(ds *dataset.Dataset, sel selection.Selection, f encode.Format, q url.Values) (*encode.ClientExport, error) {
	home := x.s.opts.Engine.Home()
	m, err := manifest.Load(home.Manifest())
	if err != nil {
		return nil, err
	}
	src := renderer.Source{M: m, P: renderer.ExportPathsFor(home.SSHDir), EmitUseKeychain: platform.EmitUseKeychain()}
	exp, _, err := selection.HostExport(ds, sel, f, src, encode.Options{IncludeGit: q.Get("include_git") == "1"})
	return exp, err
}

// shapedPreset applies to a preset the options a list's CSV link carries, so
// the download dialog can ask for its file in any format with the choice it
// already holds: ?profile= ?key= ?narrow=1 ?exclude= keep and narrow rows,
// ?drop= leaves columns out by their heading, and the events and history
// pages' own filters (?area= ?kind= ?since= ?until= ?command= ?source=)
// keep what the page shows.
func shapedPreset(sel selection.Selection, q url.Values) selection.Selection {
	shape := shapeOf(q)
	sel.Where.Profile, sel.Where.Key, sel.Narrow = shape.Profiles, shape.Keys, shape.Narrow
	sel.Where.Exclude = shape.Exclude
	for param, to := range map[string]*[]string{"area": &sel.Where.Area, "kind": &sel.Where.Kind, "source": &sel.Where.Source, "command": &sel.Where.Command} {
		if v := strings.TrimSpace(q.Get(param)); v != "" {
			*to = []string{v}
		}
	}
	sel.Where.Since, sel.Where.Until = strings.TrimSpace(q.Get("since")), strings.TrimSpace(q.Get("until"))
	if len(shape.Drop) > 0 {
		drop := map[string]bool{}
		for _, d := range shape.Drop {
			drop[d] = true
		}
		for _, c := range selection.Defaults[sel.Grain] {
			if !drop[c.Heading()] {
				sel.Columns = append(sel.Columns, c)
			}
		}
	}
	return sel
}

// selectionOf reads the selection a request names: a built-in preset by
// name, or a selection as base64url JSON. name is what the file is called.
func selectionOf(preset, encoded string) (selection.Selection, string, error) {
	switch {
	case preset != "" && encoded != "":
		return selection.Selection{}, "", errors.New("give a preset or a selection, not both")
	case preset != "":
		s, ok := selection.Presets[preset]
		if !ok {
			return selection.Selection{}, "", errors.New("no preset named " + preset)
		}
		return s, preset, nil
	case encoded != "":
		if len(encoded) > MaxSelection {
			return selection.Selection{}, "", errors.New("the selection is too long")
		}
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(encoded, "="))
		if err != nil {
			return selection.Selection{}, "", errors.New("the selection is not base64url")
		}
		s, err := selection.Parse(b)
		if err != nil {
			return selection.Selection{}, "", err
		}
		return s, s.Grain + "s", nil
	}
	return selection.Selection{}, "", errors.New("give ?preset= or ?s=")
}

func (x exportAPI) json(w http.ResponseWriter, v any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (x exportAPI) fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
