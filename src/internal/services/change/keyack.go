package change

import (
	"fmt"
	"io"
	"strings"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// KeyAck is `authama ssh key ack <selection>`: each selected key loses its
// "new" label until a later event (a deploy, say) makes it new again. Only
// key-events.json changes. A key with no inventory record has no events to
// acknowledge and is skipped.
func KeyAck(sel keysvc.Selection) Change {
	args := append([]string{}, sel.Selectors...)
	if sel.Category != "" {
		args = append(args, "--category="+sel.Category)
	}
	if sel.All {
		args = append(args, "--all")
	}
	var subjects []history.Subject
	for _, s := range sel.Selectors {
		if profile, key, ok := strings.Cut(s, "/"); ok {
			subjects = append(subjects, history.Subject{Kind: history.KindProfile, Name: profile},
				history.Subject{Kind: history.KindKey, Name: key})
		}
	}
	return Change{
		Command: "key ack", Args: args, Subjects: subjects,
		Run: func(p paths.Paths, out io.Writer) error {
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			inv, err := inventory.Load(p.Inventory())
			if err != nil {
				return err
			}
			rows, err := keysvc.New(m, inv, p.SSHDir).Select(sel)
			if err != nil {
				return err
			}
			now := inventory.Stamp()
			var done []string
			for _, r := range rows {
				if r.Fingerprint == "" {
					continue
				}
				inv.Note(r.Fingerprint, inventory.EventAcknowledged, now)
				done = append(done, r.Ref.String())
			}
			if len(done) == 0 {
				_, _ = fmt.Fprintln(out, "nothing to acknowledge: none of those keys has been minted")
				return nil
			}
			if err := inv.Save(p.Inventory()); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), "key.ack", log.Field{Key: "keys", Value: strings.Join(done, ",")})
			_, _ = fmt.Fprintf(out, "acknowledged %s; each is new again only after its next event\n", strings.Join(done, ", "))
			return nil
		},
	}
}
