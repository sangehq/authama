package web

import (
	"fmt"
	"html/template"
)

// tips is every explainer a control on these pages shows, by id: what using
// it does, and in particular that nothing is written before a preview is
// confirmed. One catalogue, so the wording stays consistent and every id a
// template names is checked (TestEveryTipATemplateNamesExists).
var tips = map[string]string{
	// Opening a form never changes anything.
	"preview": "Shows exactly what would change, file by file. Nothing is written until you confirm.",
	"review":  "Shows what the job will do on other machines. It runs only when you start it.",
	"run":     "Confirms this change now, the same as the button at the foot of the review.",

	// Profiles, hosts, keys.
	"profile-add":      "Adds a profile: a group of hosts that share a key policy and a folder under ~/.ssh/profiles/.",
	"profile-scope":    "Per service mints a key for each host; shared gives every host in the profile one key.",
	"profile-comment":  "The comment ssh-keygen writes into keys this profile mints from now on. Existing keys keep theirs.",
	"profile-category": "Files the profile's key folder under ~/.ssh/profiles/<category>/, and moves its keys there.",
	"profile-rotate":   "Replaces every key the profile's hosts use, one at a time, deploying each before retiring the old one.",
	"host-add":         "Adds a host to a profile's ssh config. Its key is minted on the next reconcile.",
	"host-vm":          "Adds a host whose address the hypervisor gives each time ssh connects.",
	"host-bulk":        "Adds or updates many hosts at once, one per line.",
	"host-edit":        "Changes this host's hostname, user, port or tags, previewed first.",
	"key-add":          "Declares a key in a profile and mints it.",
	"key-mint":         "Mints every key the manifest names that is not on disk yet. Existing keys are left alone.",
	"key-lifetime":     "How long the key lives before rotation is due. The expiry date is counted again.",
	"key-revoke":       "Removes the key from every server and account it is deployed to, and never deploys it again.",
	"vm-adopt":         "Copies a loose keypair into a profile and adds the VM host that uses it. The loose files stay.",

	// Config, backups, exports.
	"ssh-config":    "The config files the manifest renders into ~/.ssh, and whether the ones on disk match.",
	"bundle":        "An encrypted backup of everything in ~/.ssh, private keys included, to your age recipient.",
	"prune":         "Deletes snapshots older than the newest ones you keep. You see the list before anything is removed.",
	"csv":           "Choose the columns and rows, see the first rows, then download.",
	"download":      "Downloads a file. Nothing in this home changes.",
	"reconcile":     "Mints the keys the manifest names and renders ~/.ssh/config, previewed first.",
	"render":        "Writes ~/.ssh/config again from the manifest, keeping everything outside authama's block.",
	"token-add":     "Adds or replaces a token in a tool's credential file. The secret never comes back to this page.",
	"token-apply":   "Writes every token tokens.json names, from the environment this server was started in.",
	"token-config":  "Edits tokens.json, which names environment variables, never secrets. Checked as you type.",
	"history":       "Every change made to this home, by whom and from where, with what it wrote.",
	"copy-ssh":      "Copies the ssh command for this host to the clipboard.",
	"pin-set":       "A PIN this page asks for at sign-in and before changes. Set only if a forgotten one could be reset.",
	"pin-remove":    "Turns the PIN off: sign-in and changes stop asking for one.",
	"owner-unbind":  "Removes the binding, the seal and the PIN. Your keys, tokens and history stay.",
	"factory-reset": "Moves the authama home aside (kept) and starts again. ~/.ssh is not touched.",
	"repair":        "Fixes what can be fixed safely, previewed first. ~/.ssh is snapshotted before.",
	"delete":        "Previews the delete first; nothing is removed until you confirm.",
	"restore":       "Lays a backup back over ~/.ssh, after snapshotting it. You review the job first.",
	"more":          "More actions for this page.",
}

// tipText is the explainer for id. An unknown id is an error, so a template
// naming one fails to render rather than showing nothing.
func tipText(id string) (string, error) {
	t, ok := tips[id]
	if !ok {
		return "", fmt.Errorf("no tip named %q in web/tips.go", id)
	}
	return t, nil
}

// tipAttr is ` data-tip="…"` for an element, escaped by html/template's
// attribute context like any other value.
func tipAttr(id string) (template.HTMLAttr, error) {
	t, err := tipText(id)
	if err != nil {
		return "", err
	}
	return template.HTMLAttr(` data-tip="` + template.HTMLEscapeString(t) + `"`), nil //nolint:gosec // the text is escaped above, and comes from the catalogue, not a request
}
