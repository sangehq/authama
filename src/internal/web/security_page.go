package web

import (
	"net/http"
	"sort"

	"github.com/simtabi/authama/src/internal/services/owner"
)

// securityPage is the owner binding as the Owner & PIN page shows it. It is
// read-only: binding a home, setting or resetting a PIN, and accepting an
// outside change all prove the OS account at a terminal, which a browser
// cannot.
type securityPage struct {
	State     owner.State
	Biometric bool
	Why       string
	Files     []string
}

func (h pages) handleSecurity(w http.ResponseWriter, r *http.Request) {
	p := h.s.opts.Engine.Home()
	bio, why := owner.BiometricAvailable()
	var files []string
	for name := range owner.SealedFiles(p) {
		files = append(files, name)
	}
	sort.Strings(files)
	h.s.render(w, http.StatusOK, "security", h.s.newView(r, "security", "Owner & PIN",
		securityPage{State: owner.Verify(p), Biometric: bio, Why: why, Files: files}))
}
