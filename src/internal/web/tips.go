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
	"profile-add":        "Adds a profile: a group of hosts that share a key policy and a folder under ~/.ssh/profiles/.",
	"profile-scope":      "Per service mints a key for each host; shared gives every host in the profile one key.",
	"profile-comment":    "The comment ssh-keygen writes into keys this profile mints from now on. Existing keys keep theirs.",
	"profile-category":   "Files the profile's key folder under ~/.ssh/profiles/<category>/, and moves its keys there.",
	"profile-rotate":     "Replaces every key the profile's hosts use, one at a time, deploying each before retiring the old one.",
	"host-add":           "Adds a host to a profile's ssh config. Its key is minted on the next reconcile.",
	"host-vm":            "Adds a host whose address the hypervisor gives each time ssh connects.",
	"host-bulk":          "Adds or updates many hosts at once, one per line.",
	"knownhosts-init":    "Scans hosts and pins the keys of each that answers into known_hosts (trust on first use).",
	"key-regen-pub":      "Rebuilds this key's .pub from its private key or 1Password item. Previewed first; the private key is never changed.",
	"key-regen-pub-many": "Rebuilds the .pub of one profile's keys, or every key. Previewed first; no private key changes.",
	"naming-set":         "Changes how new servers are named: the alias template, the envs and roles it allows, and how strictly. Existing hosts keep their names and key names.",
	"naming-key":         "Changes how new keys are named. Every existing host keeps the key name it has now.",
	"profile-code":       "The code a profile's servers are named by, the {code} in the alias. Existing hosts keep their key names.",
	"net":                "Probes every host's SSH port now and says which answer and which need a VPN. Reads only; the same as ssh net.",
	"net-host":           "Probes this host's SSH port now and says whether it answers or needs a VPN. Reads only; the same as ssh net.",
	"net-again":          "Probes the same hosts again. Reads only.",
	"validate-again":     "Runs every check again. Reads only.",
	"validate-open":      "Opens the page that shows what this check reads.",
	"host-pin":           "Asks this host for its keys and shows their fingerprints. Nothing is trusted until you confirm.",
	"host-edit":          "Changes this host's hostname, user, port or tags, previewed first.",
	"key-add":            "Declares a key in a profile and mints it.",
	"key-mint":           "Mints every key the manifest names that is not on disk yet. Existing keys are left alone.",
	"key-lifetime":       "How long the key lives before rotation is due. The expiry date is counted again.",
	"key-revoke":         "Removes the key from every server and account it is deployed to, and never deploys it again.",
	"vm-adopt":           "Copies a loose keypair into a profile and adds the VM host that uses it. The loose files stay.",

	// Config, backups, exports.
	"ssh-config":     "The config files the manifest renders into ~/.ssh, and whether the ones on disk match.",
	"bundle":         "An encrypted backup of everything in ~/.ssh, private keys included, to your age recipient.",
	"prune":          "Deletes snapshots older than the newest ones you keep. You see the list before anything is removed.",
	"csv":            "Choose the profiles, keys, columns and rows, see the first rows, then download.",
	"download":       "Downloads a file. Nothing in this home changes.",
	"export-command": "Copies the authama export command that writes this file from a terminal: the same list, filters, columns, sort and format.",
	"reconcile":      "Mints the keys the manifest names and renders ~/.ssh/config, previewed first.",
	"render":         "Writes ~/.ssh/config again from the manifest, keeping everything outside authama's block.",
	"token-add":      "Adds or replaces a token in a tool's credential file. The secret never comes back to this page.",
	"token-apply":    "Writes every token tokens.json names, from the environment this server was started in.",
	"token-config":   "Edits tokens.json, which names environment variables, never secrets. Checked as you type.",
	"history":        "Every change made to this home, by whom and from where, with what it wrote.",
	"copy-ssh":       "Copies the ssh command for this host to the clipboard.",
	"pin-set":        "A PIN this page asks for at sign-in and before changes. Set only if a forgotten one could be reset.",
	"pin-remove":     "Turns the PIN off: sign-in and changes stop asking for one.",
	"owner-unbind":   "Removes the binding, the seal and the PIN. Your keys, tokens and history stay.",
	"factory-reset":  "Moves the authama home aside (kept) and starts again. ~/.ssh is not touched.",
	"repair":         "Fixes what can be fixed safely, previewed first. ~/.ssh is snapshotted before.",
	"delete":         "Previews the delete first; nothing is removed until you confirm.",
	"restore":        "Lays a backup back over ~/.ssh, after snapshotting it. You review the job first.",
	"more":           "More actions for this page.",

	// Added for every remaining control, 2026-11-06.
	"apply":                "Makes exactly the change previewed above and records it in History. Nothing else is written.",
	"job-run":              "Starts the job reviewed above. It runs step by step, this page follows it, and you can cancel between steps.",
	"job-yes":              "Answers yes: the job carries on with its next step.",
	"job-no":               "Answers no: the job stops here. Steps already done stay done, and History records where it stopped.",
	"job-cancel":           "Stops the job once its current step finishes. Steps already done stay done.",
	"open-row":             "Opens its page beside the list, with everything about it. Nothing changes.",
	"copy-value":           "Copies this value to the clipboard. Nothing changes.",
	"row-history":          "Shows every change made to this, newest first.",
	"profile-hosts":        "Lists the hosts in this profile.",
	"key-open-rotate":      "Opens the key's page, where you can rotate it or roll it back.",
	"key-deploy":           "Installs this key's public half on every host that uses it, then checks that ssh connects with it.",
	"key-deploy-revoked":   "This key is revoked, so it is never deployed again. Mint or declare a new key instead.",
	"key-rollback":         "Puts the previous key back on every host and retires this one; use it when a rotation went wrong.",
	"key-unreserve":        "Previews removing the reservation, so doctor reports this loose key again. The key files stay.",
	"docker-context":       "Previews creating a Docker context that reaches this host's Docker over ssh. Nothing is written until you confirm.",
	"doctor-fix":           "Previews what doctor --fix repairs (permissions, missing files and folders). Nothing is written until you confirm.",
	"providers-export":     "Previews writing providers.json: the built-in providers as a file you can edit. Nothing is written until you confirm.",
	"clean":                "Asks first, then previews removing stale known_hosts pins, stale records and leftover files. Nothing is removed until you confirm.",
	"settings-init":        "Previews writing a starter authama.json with every setting at its default. Nothing is written until you confirm.",
	"theme-light":          "Uses the light theme in this browser.",
	"theme-dark":           "Uses the dark theme in this browser.",
	"theme-auto":           "Follows this computer's light or dark setting.",
	"fingerprint":          "Switches Touch ID unlock for sign-in and changes, previewed first. Your PIN keeps working either way.",
	"owner-reseal":         "Previews accepting what changed outside authama as the new sealed state. Nothing is written until you confirm.",
	"unlock":               "Checks the PIN and opens this session. Nothing else changes.",
	"token-init":           "Previews writing a starter tokens.json that names the environment variable each tool reads. Nothing is written until you confirm.",
	"token-import":         "Previews importing the older token file into tokens.json. Nothing is written until you confirm.",
	"vm-sync":              "Previews refreshing the VM hosts from the hypervisor: names and addresses. Nothing is written until you confirm.",
	"vm-schedule-on":       "Previews installing a daily 09:30 sync (launchd, systemd, cron or Task Scheduler). Nothing is written until you confirm.",
	"vm-schedule-off":      "Previews removing the daily sync. The VM hosts stay as they are.",
	"audit-notify":         "Sends a desktop alert now if a key is due for rotation, and none if not. No key changes.",
	"notify-test":          "Sends one test desktop alert, to check alerts reach you. Nothing else happens.",
	"snapshot-compare":     "Shows the lines that differ between this snapshot and the one chosen. Nothing changes.",
	"snapshot-compare-now": "Shows what changed in ~/.ssh since this snapshot. Nothing changes.",
	"filter-clear":         "Removes every filter and shows the whole list again.",
	"export-clear":         "Clears this tab's filters. The preview updates; nothing is downloaded.",
	"export-columns-reset": "Puts back the list's own columns, names and order.",
	"export-add-column":    "Adds the chosen field as a column at the end.",
	"export-add-step":      "Adds the chosen transform as the last step.",
	"export-save":          "Saves this selection under the name given: in your own store, or in the manifest after a preview.",
	"export-copy":          "Copies the file, in the chosen format, to the clipboard. Nothing is saved.",
	"csv-profiles":         "Choose which profiles' rows go in the file.",
	"csv-keys":             "Choose which keys' rows go in the file.",
	"csv-columns":          "Choose, rename and order the file's columns.",
	"csv-reset":            "Puts every profile, key and column back, as the file was first offered.",
	"csv-open-export":      "Opens this list on the Export page, for more filters and formats.",
	"search":               "Searches every page, form and download. Ctrl+K, ⌘K or / opens it from anywhere; ? lists every shortcut.",
	"shortcuts":            "Lists every keyboard shortcut.",
	"menu-toggle":          "Shows or hides the main menu.",
	"events-all":           "Opens Events: every change to ~/.ssh, the config home and the token files, with filters.",
	"tour":                 "Walks through the header and the overview one step at a time. Esc ends it.",
	"restart-now":          "Restarts this server from the build now installed, on the same address and sign-in. The page reloads by itself.",
	"reload":               "Reloads this page into the new build. Unsaved input on this page is lost.",
	"panel-back":           "Goes back to what this panel showed before.",
	"panel-full":           "Opens this as a page of its own.",
	"back-home":            "Goes back to the overview.",
	"docs":                 "Opens the documentation in a new page.",
	"source":               "Opens authama's source code on GitHub.",
	"report-issue":         "Opens GitHub to report a bug or ask for a feature. Nothing is sent from here.",
	"menu-settings":        "Opens Settings: theme, defaults, and where authama keeps things.",
	"menu-events":          "Opens Events: every file change, with filters.",
	"menu-doctor":          "Opens Doctor: checks this home and offers fixes.",
	"menu-about":           "Opens About: this build, whether it is current, and this computer.",
	"cancel-change":        "Leaves without making the change. Nothing is written.",
	"export-grain":         "Switches which list the file is made from.",
	"export-share":         "Copies a link that opens this page with the same choices. Nothing is saved.",
	"filter-apply":         "Shows only what matches these filters.",
	"confirm-destructive":  "Goes on to the preview of this change. Nothing is removed until you confirm that too.",
	"panel-submit":         "Checks the form and shows what it would do. Nothing is written until you confirm.",
	"events-area":          "Shows only the file events in this area. Nothing changes.",
	"bell":                 "Recent changes to ~/.ssh, the config home and the token files. A dot means one in the last day.",
	"account":              "Who this server runs as, and Settings, Events, Doctor, About and the tour.",
	"licences":             "Jumps to the licences of the front-end code bundled in this binary.",
}

// pageTips is the one-line description of each page, keyed as the menu
// keys it (navItem), shown as the menu entry's tooltip.
var pageTips = map[string]string{
	"overview":  "What needs attention, counts of everything, and quick actions.",
	"profiles":  "Groups of hosts that share a key policy, and their keys' folders.",
	"naming":    "How new servers and their keys are named, each profile's code, and which servers break the rules.",
	"hosts":     "Every host by profile, as it is rendered into ~/.ssh/config.",
	"keys":      "Every key with its fingerprint and expiry, and the loose keys kept on purpose.",
	"vms":       "Hosts whose address a hypervisor gives each time, and their daily sync.",
	"sshconfig": "The config files the manifest renders into ~/.ssh, and whether they match.",
	"snapshots": "Copies of ~/.ssh taken before each change, and encrypted bundles.",
	"tokens":    "The tokens npm, Composer, Docker, git, PyPI and other tools read.",
	"audit":     "Key age, rotation due dates and where each key is deployed.",
	"security":  "The PIN, the seal and the binding that tie this home to you.",
	"history":   "Every change made to this home, by whom and from where.",
	"events":    "Every file change in ~/.ssh, the config home and the token files.",
	"export":    "Make a file of any list, filtered, in any of nine formats.",
	"doctor":    "Checks this home and offers fixes.",
	"validate":  "Checks the manifest, the settings files, every keypair and ~/.ssh, and says what is wrong.",
	"repair":    "Fixes a damaged home, or starts it again.",
	"settings":  "Theme, defaults, and where authama keeps things.",
	"about":     "This build, this computer, and the licences.",
}

// groupTips says what each menu heading holds, keyed as navGroup keys it.
var groupTips = map[string]string{
	"ssh":      "Profiles, hosts, keys and VMs.",
	"files":    "The rendered ssh config, and the snapshots and bundles of ~/.ssh.",
	"auth":     "Auth tokens, the key audit, and the PIN and seal.",
	"activity": "What changed: History of authama's changes, and Events for every file change.",
	"system":   "Doctor, Validate, Repair, Settings and About.",
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
