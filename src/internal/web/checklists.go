package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/simtabi/authama/src/internal/services/checklist"
)

// checklistPages are the revocation checklists: every key's and token's on
// one page, and one item's on its own (opened in the side panel from a row).
// A tick, and starting a token's revocation, are kept in the state directory
// and change nothing under ~/.ssh, so they are made straight away rather than
// through a preview.
type checklistPages struct{ s *Server }

// checklistsView is the list page.
type checklistsView struct {
	Items  []checklist.Checklist
	Show   string // "" (all), "open", "keys" or "tokens"
	Open   int    // items with steps left
	Total  int
	Counts map[string]int // per Show filter, for the segmented control
}

// checklistView is one item's page.
type checklistView struct {
	checklist.Checklist
	Done, Total int
}

func (h checklistPages) service() *checklist.Service {
	return checklist.New(h.s.opts.Engine.Home())
}

func (h checklistPages) handleList(w http.ResponseWriter, r *http.Request) {
	all, err := h.service().All()
	if err != nil {
		h.s.renderError(w, r, http.StatusInternalServerError, "Could not read the checklists", err.Error())
		return
	}
	show := r.URL.Query().Get("show")
	v := checklistsView{Show: show, Total: len(all), Counts: map[string]int{"": len(all)}}
	for _, c := range all {
		open := c.Open() > 0
		if open {
			v.Open++
			v.Counts["open"]++
		}
		v.Counts[c.Kind+"s"]++
		switch {
		case show == "open" && !open, show == "keys" && c.Kind != checklist.KindKey, show == "tokens" && c.Kind != checklist.KindToken:
			continue
		}
		v.Items = append(v.Items, c)
	}
	h.s.render(w, http.StatusOK, "checklists", h.s.newView(r, "checklists", "Checklists", v))
}

func (h checklistPages) handleItem(w http.ResponseWriter, r *http.Request) {
	c, err := h.service().Get(r.URL.Query().Get("item"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	done, total := c.Counts()
	h.s.render(w, http.StatusOK, "checklist", h.s.newView(r, "checklists", c.Name, checklistView{Checklist: c, Done: done, Total: total}))
}

// handleTick ticks or unticks a step authama cannot check, then shows the
// item again.
func (h checklistPages) handleTick(w http.ResponseWriter, r *http.Request) {
	item, step := strings.TrimSpace(r.PostFormValue("item")), strings.TrimSpace(r.PostFormValue("step"))
	c, err := h.service().Tick(item, step, r.PostFormValue("on") == "1")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.back(w, r, c.ID)
}

// handleRevoking starts or stops a token's revocation checklist.
func (h checklistPages) handleRevoking(w http.ResponseWriter, r *http.Request) {
	c, err := h.service().SetRevoking(strings.TrimSpace(r.PostFormValue("item")), r.PostFormValue("on") == "1")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.back(w, r, c.ID)
}

// back answers a post with the item's page, by redirect, so a reload never
// posts the tick again. The side panel follows the redirect in place.
func (h checklistPages) back(w http.ResponseWriter, r *http.Request, id string) {
	http.Redirect(w, r, h.s.opts.BasePath+"/checklist?item="+url.QueryEscape(id), http.StatusSeeOther)
}

func (h checklistPages) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, checklist.ErrNotFound) {
		h.s.renderError(w, r, http.StatusNotFound, "Not found", "No key or token by that name is in this authama home.")
		return
	}
	h.s.renderError(w, r, http.StatusBadRequest, "Could not change the checklist", err.Error())
}
