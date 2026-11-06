package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/simtabi/authama/src/internal/brand"
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
	// Defaults are the grain's default columns, field and heading, so a page
	// can start its column list from them.
	Defaults []selection.Column `json:"defaults"`
}

type exportFormat struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Extension string `json:"extension"`
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
			Defaults: selection.Defaults[sel.Grain]}
		for i, row := range res.Rows {
			if i == PreviewRows {
				break
			}
			p.Rows = append(p.Rows, selection.Cells(row))
		}
		for _, n := range encode.Names() {
			ef, _ := encode.Get(n)
			p.Formats = append(p.Formats, exportFormat{Name: ef.Name, Label: ef.Label, Extension: ef.Extension})
		}
		x.json(w, p)
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
