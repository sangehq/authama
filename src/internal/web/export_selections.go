package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/dataset"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/selection"
)

// exportSelectionsAPI is the Export page's saved selections: the list from
// both stores, and saving to or deleting from this account's own. The
// manifest's store changes through /changes like any manifest change
// (export-selection-save, export-selection-delete), so it is previewed,
// confirmed and refused in read-only mode with the rest.
type exportSelectionsAPI struct{ s *Server }

// savedSelections is the answer to GET /api/export/selections.
type savedSelections struct {
	Selections []savedsel.Saved `json:"selections"`
	// SaveTo is the store a save goes to unless the page says otherwise
	// (export.save_to).
	SaveTo string `json:"save_to"`
	// ReadOnly says nothing can be saved or deleted here.
	ReadOnly bool `json:"read_only"`
}

func (x exportSelectionsAPI) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (x exportSelectionsAPI) fail(w http.ResponseWriter, status int, err error) {
	x.reply(w, status, map[string]string{"error": err.Error()})
}

func (x exportSelectionsAPI) handleList(w http.ResponseWriter, _ *http.Request) {
	p := x.s.opts.Engine.Home()
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		m = nil // the account's own store is still listed
	}
	all, err := savedsel.List(p, m)
	if err != nil {
		x.fail(w, http.StatusInternalServerError, err)
		return
	}
	if all == nil {
		all = []savedsel.Saved{}
	}
	x.reply(w, http.StatusOK, savedSelections{Selections: all, SaveTo: savedsel.DefaultStore(nil), ReadOnly: x.s.opts.ReadOnly})
}

// saveRequest is the body of a save: a name and the selection.
type saveRequest struct {
	Name      string          `json:"name"`
	Selection json.RawMessage `json:"selection"`
}

// handleSave keeps a selection in this account's own store. Registered only
// when the server may change things.
func (x exportSelectionsAPI) handleSave(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxSelection+1024)).Decode(&req); err != nil {
		x.fail(w, http.StatusBadRequest, fmt.Errorf("an unreadable request: %w", err))
		return
	}
	sel, err := selection.Parse(req.Selection)
	if err != nil {
		x.fail(w, http.StatusBadRequest, err)
		return
	}
	if err := savedsel.Save(x.s.opts.Engine.Home(), savedsel.StoreUser, strings.TrimSpace(req.Name), sel); err != nil {
		x.fail(w, http.StatusBadRequest, err)
		return
	}
	x.reply(w, http.StatusOK, map[string]string{"saved": req.Name, "store": savedsel.StoreUser})
}

// handleDelete removes a selection from this account's own store.
func (x exportSelectionsAPI) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		x.fail(w, http.StatusBadRequest, fmt.Errorf("an unreadable request: %w", err))
		return
	}
	err := savedsel.Delete(x.s.opts.Engine.Home(), savedsel.StoreUser, strings.TrimSpace(req.Name))
	switch {
	case errors.Is(err, savedsel.ErrNotFound):
		x.fail(w, http.StatusNotFound, err)
	case err != nil:
		x.fail(w, http.StatusBadRequest, err)
	default:
		x.reply(w, http.StatusOK, map[string]string{"deleted": req.Name, "store": savedsel.StoreUser})
	}
}

// savedSelectionForm reads a manifest save's name and selection from a
// change form.
func savedSelectionForm(f url.Values) (string, selection.Selection, error) {
	name := strings.TrimSpace(f.Get("name"))
	if err := savedsel.CheckName(name); err != nil {
		return "", selection.Selection{}, badForm("%s", err.Error())
	}
	raw := f.Get("selection")
	if len(raw) > MaxSelection {
		return "", selection.Selection{}, badForm("the selection is longer than %d bytes", MaxSelection)
	}
	sel, err := selection.Parse([]byte(raw))
	if err != nil {
		return "", selection.Selection{}, badForm("%s", err.Error())
	}
	return name, sel, nil
}

// handleCompare is GET /exports/compare?a=…&b=…: two selections (base64url
// JSON, as /exports/run takes) matched on the columns they share, with the
// rows only one of them holds, at most PreviewRows each.
func (x exportSelectionsAPI) handleCompare(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var res [2]selection.Result
	for i, k := range []string{"a", "b"} {
		sel, _, err := selectionOf("", q.Get(k))
		if err != nil {
			x.fail(w, http.StatusBadRequest, fmt.Errorf("%s: %w", k, err))
			return
		}
		ds, err := dataset.Build(x.s.opts.Engine.Home(), dataset.Options{Parts: selection.Parts(sel.Grain)})
		if err != nil {
			x.fail(w, http.StatusInternalServerError, err)
			return
		}
		if res[i], err = selection.Run(ds, sel); err != nil {
			x.fail(w, http.StatusBadRequest, fmt.Errorf("%s: %w", k, err))
			return
		}
	}
	cmp, err := selection.Compare(res[0], res[1])
	if err != nil {
		x.fail(w, http.StatusBadRequest, err)
		return
	}
	only := func(rows [][]string) ([][]string, int) {
		if len(rows) > PreviewRows {
			return rows[:PreviewRows], len(rows)
		}
		return rows, len(rows)
	}
	var reply struct {
		selection.Comparison
		OnlyACount int `json:"only_a_count"`
		OnlyBCount int `json:"only_b_count"`
	}
	reply.Comparison = cmp
	reply.OnlyA, reply.OnlyACount = only(cmp.OnlyA)
	reply.OnlyB, reply.OnlyBCount = only(cmp.OnlyB)
	x.reply(w, http.StatusOK, reply)
}
