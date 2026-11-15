package web

import (
	"net/http"

	"github.com/sangehq/authama/src/internal/services/repair"
)

// resetOption is one row on the Reset page: one way to start something
// again, what it resets and keeps, how to undo it, the command it is the
// same as, and how it is started - a link to the form on its own page (Href),
// a preview straight from the row (Action), or a panel on this page that asks
// for a word to be typed (Panel). Tip is what its button opens.
type resetOption struct {
	Title, Icon, What   string
	Resets, Keeps, Undo string
	Command, Button     string
	Tip                 string
	Href, Action, Panel string
	// Undoable is false when authama cannot put it back.
	Undoable bool
}

// resetGroup is a card of rows on the Reset page.
type resetGroup struct {
	Title, Summary string
	Options        []resetOption
}

// resetPage gathers every reset authama offers in one place, grouped by how
// much each one resets.
type resetPage struct {
	Groups []resetGroup
	// HistoryPermanent is history.permanent: the history is never set aside,
	// so its row says so instead of offering the reset.
	HistoryPermanent bool
}

func resetOptions(devRoot, historyPermanent bool) resetPage {
	var keys []resetOption
	if !devRoot {
		keys = append(keys,
			resetOption{Title: "Rotate every key", Icon: "recycle", Undoable: true,
				What:   "Replaces every key a host uses with a new one, deploying each before the old one goes.",
				Resets: "Each key a host uses.", Keeps: "Profiles and hosts; each old key under old/, for rollback.", Undo: "Roll a key back from its page.",
				Command: "authama ssh rotate --all", Href: "/jobs/review?action=rotate-keys&all=1", Button: "Choose keys",
				Tip: "Opens the rotation's review with every key chosen; untick it to choose fewer. Nothing runs until you start it."},
			resetOption{Title: "Revoke keys", Icon: "ban",
				What:   "Removes keys from every server they were deployed to, and records them as revoked.",
				Resets: "Where the chosen keys are trusted.", Keeps: "The declarations, hosts and key files; each key's checklist lists what is left.", Undo: "None: mint and deploy a successor instead.",
				Command: "authama ssh key revoke --all", Href: "/jobs/review?action=revoke-keys", Button: "Choose keys",
				Tip: "Opens the revocation's review to choose which keys. Nothing runs until you start it."})
	}
	history := resetOption{Title: "Start the history again", Icon: "history", Undoable: true,
		What:   "Moves the record of every change, and the audit log it is rebuilt from, aside; the next change starts a new one.",
		Resets: "history.db and audit.log.", Keeps: "Both, renamed to <name>.reset-<time>; everything else.", Undo: "Rename the two files back.",
		Command: "authama history reset", Panel: "history-reset", Button: "Review…",
		Tip: "Opens a panel that lists what moves aside and asks you to type history."}
	if historyPermanent {
		history.What = "Kept for good: history.permanent is on, so the history is never set aside."
		history.Panel, history.Href, history.Button = "", "/settings", "Settings"
		history.Tip = "Opens Settings, where history.permanent can be turned off."
	}
	return resetPage{HistoryPermanent: historyPermanent, Groups: []resetGroup{
		{Title: "Keys", Summary: "Replace or retire keys. These reach the servers that trust them.", Options: keys},
		{Title: "Configuration", Summary: "Put a file back to its defaults, or clear old copies.", Options: []resetOption{
			{Title: "Settings to their defaults", Icon: "settings", Undoable: true,
				What:   "Writes authama.json again with every setting at its default.",
				Resets: "authama.json: theme, defaults, retention and the rest.", Keeps: "The old file, beside it as authama.json.before-reset-<time>; the manifest, keys and tokens.", Undo: "Rename the kept file back.",
				Command: "authama settings init --force", Action: "settings-reset", Button: "Preview",
				Tip: "Shows the new authama.json and the copy of the old one before anything is written."},
			{Title: "Initialise the home again", Icon: "refresh", Undoable: true,
				What:   "Writes the manifest, the inventory and .env again with fresh defaults.",
				Resets: "Every profile, host and key declaration, the key records, and .env.", Keeps: "A copy of the old files in the state home's .state/init-backup-<time>/; ~/.ssh, its keys and config.", Undo: "Copy the backed-up files back.",
				Command: "authama init --force --backup", Panel: "home-init", Button: "Review…",
				Tip: "Opens a panel that lists what is rewritten and asks you to type init."},
			{Title: "Old snapshots", Icon: "archive",
				What:   "Deletes the oldest snapshots of ~/.ssh, keeping the newest you choose.",
				Resets: "Snapshots older than the newest N.", Keeps: "The newest N snapshots, encrypted bundles, and ~/.ssh.", Undo: "None: a pruned snapshot is gone.",
				Command: "authama ssh snapshots prune", Href: "/snapshots#prune", Button: "Choose",
				Tip: "Opens Prune old snapshots on the Snapshots page, to choose how many to keep."},
		}},
		{Title: "History", Summary: "The record of every change authama made.", Options: []resetOption{history}},
		{Title: "Everything", Summary: "Start the whole home again, or only its binding to you.", Options: []resetOption{
			{Title: "Unbind this home", Icon: "lock-open", Undoable: true,
				What:   "Removes the owner binding: the sealing key, the seal and the PIN.",
				Resets: "The owner, seal and PIN.", Keeps: "Everything else: keys, hosts, tokens, settings.", Undo: "Bind it again on Owner & PIN.",
				Command: "authama owner forget", Href: "/security#forget", Button: "Review…",
				Tip: "Opens Unbind this home on Owner & PIN, which asks you to type a word before it runs."},
			{Title: "Factory reset", Icon: "refresh", Undoable: true,
				What:   "Puts authama back to how it was before it was set up, by moving its folders aside.",
				Resets: "The config and state homes: manifest, settings, token records, history and owner.", Keeps: "The moved folders, renamed aside; ~/.ssh, its keys and config.", Undo: "Rename the folders back.",
				Command: "authama reset", Href: "/repair#factory-reset", Button: "Review…",
				Tip: "Opens Factory reset on Repair, which lists what moves aside and asks you to type reset."},
		}},
	}}
}

func (h ownerPages) handleReset(w http.ResponseWriter, r *http.Request) {
	v := h.s.newView(r, "reset", "Reset", nil)
	v.Data = resetOptions(v.DevRoot != "", repair.HistoryPermanent(nil))
	h.s.render(w, http.StatusOK, "reset", v)
}
