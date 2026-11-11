package web

import "net/http"

// resetOption is one card on the Reset page: one way to start something
// again, what it resets and keeps, how to undo it, the command it is the
// same as, the page and form that do it, and Tip, what its button opens.
type resetOption struct {
	Title, Icon, What          string
	Resets, Keeps, Undo        string
	Command, Href, Button, Tip string
	Danger                     bool
}

// resetPage gathers every reset authama offers in one place, grouped by how
// much each one resets. The forms themselves stay on their own pages; the
// two that have no other page (settings and the home's files) are on the
// Reset page itself.
type resetPage struct {
	Keys, Config, Everything []resetOption
}

func resetOptions(readOnly, devRoot bool) resetPage {
	var d resetPage
	href := func(h string) string {
		if readOnly {
			return ""
		}
		return h
	}
	if !devRoot {
		d.Keys = append(d.Keys,
			resetOption{Title: "Rotate every key", Icon: "recycle", What: "Replaces every key a host uses with a new one, deploying each before the old one goes.",
				Resets: "Each key a host uses.", Keeps: "Profiles and hosts; each old key under old/, for rollback.", Undo: "Roll a key back from its page.",
				Command: "authama ssh rotate --all", Href: href("/keys?pick=all#rotate-keys"), Button: "Choose keys to rotate", Tip: "Opens Rotate keys with every key chosen; untick any to keep. Nothing runs until you review the job."},
			resetOption{Title: "Revoke keys", Icon: "ban", What: "Removes keys from every server they were deployed to, and records them as revoked.",
				Resets: "Where the chosen keys are trusted.", Keeps: "The declarations, hosts and key files; each key's checklist lists what is left.", Undo: "Mint and deploy a successor.",
				Command: "authama ssh key revoke --all", Href: href("/keys#revoke-keys"), Button: "Choose keys to revoke", Tip: "Opens Revoke keys to choose which to revoke. Nothing runs until you review the job.", Danger: true})
	}
	d.Config = append(d.Config,
		resetOption{Title: "Old snapshots", Icon: "archive", What: "Deletes the oldest snapshots of ~/.ssh, keeping the newest you choose.",
			Resets: "Snapshots older than the newest N.", Keeps: "The newest N snapshots, encrypted bundles, and ~/.ssh.", Undo: "None: a pruned snapshot is gone.",
			Command: "authama ssh snapshots prune", Href: href("/snapshots#prune"), Button: "Choose how many to keep", Tip: "Opens Prune old snapshots on the Snapshots page, to choose how many to keep.", Danger: true})
	d.Everything = append(d.Everything,
		resetOption{Title: "Unbind this home", Icon: "lock-open", What: "Removes the owner binding: the sealing key, the seal and the PIN.",
			Resets: "The owner, seal and PIN.", Keeps: "Everything else: keys, hosts, tokens, settings.", Undo: "Bind it again on Owner & PIN.",
			Command: "authama owner forget", Href: href("/security#forget"), Button: "Review the unbinding", Tip: "Opens Unbind this home on Owner & PIN, which asks you to type a word before it runs.", Danger: true},
		resetOption{Title: "Factory reset", Icon: "refresh", What: "Puts authama back to how it was before it was set up, by moving its folders aside.",
			Resets: "The config and state homes: manifest, settings, tokens' records, history and owner.", Keeps: "The moved folders, renamed aside; ~/.ssh, its keys and config.", Undo: "Rename the folders back.",
			Command: "authama reset", Href: href("/repair#factory-reset"), Button: "Review the factory reset", Tip: "Opens Factory reset on Repair, which lists what moves aside and asks you to type reset.", Danger: true})
	return d
}

func (h ownerPages) handleReset(w http.ResponseWriter, r *http.Request) {
	v := h.s.newView(r, "reset", "Reset", nil)
	v.Data = resetOptions(v.ReadOnly, v.DevRoot != "")
	h.s.render(w, http.StatusOK, "reset", v)
}
