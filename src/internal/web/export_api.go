package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
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
			Fields: res.Fields, Facets: facets, Total: res.Stages[0].Rows, Kept: len(res.Rows)}
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
