package web

import (
	"net/http"

	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/repair"
	"github.com/sangehq/authama/src/internal/services/tokensvc"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// repairHome is what the web repairs: the server's home, its token files, and
// the platform's rendered-config setting, as the CLI's repair uses.
func repairHome(p paths.Paths) repair.Home {
	env := tokensvc.NewEnv(p, nil, nil, "")
	return repair.Home{P: p, Tokens: &env, EmitUseKeychain: platform.EmitUseKeychain()}
}

type repairPage struct {
	Findings []repair.Finding
	Fixable  int
	Reset    repair.ResetPlan
}

func (h ownerPages) handleRepair(w http.ResponseWriter, r *http.Request) {
	p := h.s.opts.Engine.Home()
	d := repairPage{Findings: repairHome(p).Check(), Reset: repair.PlanFactoryReset(p, h.s.now())}
	for _, f := range d.Findings {
		if f.Fixable {
			d.Fixable++
		}
	}
	h.s.render(w, http.StatusOK, "repair", h.s.newView(r, "repair", "Repair", d))
}
