package web

import (
	"net/http"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/structure"
)

// sshFolderPage is /ssh-folder: ~/.ssh drawn as a tree, as
// `authama ssh structure diagram` prints it. It only reads.
type sshFolderPage struct{ s *Server }

// SSHFolderView is the diagram as text, and its counts.
type SSHFolderView struct {
	Text, Summary, Command string
}

func (sp sshFolderPage) handle(w http.ResponseWriter, r *http.Request) {
	d, err := structure.Diagram(sp.s.opts.Engine.Home())
	if err != nil {
		sp.s.renderError(w, r, http.StatusInternalServerError, "Could not read ~/.ssh", err.Error())
		return
	}
	v := SSHFolderView{Text: d.String(), Summary: d.Summary(), Command: brand.Command + " ssh structure diagram"}
	sp.s.render(w, http.StatusOK, "sshfolder", sp.s.newView(r, "sshfolder", "SSH folder", v))
}
