package web

import (
	"fmt"
	"net/url"

	"github.com/simtabi/authama/src/internal/util/envnames"
)

// preflight is one check a job's review makes before it may run. A failed
// blocking check keeps the Start button off and says what to do.
type preflight struct {
	Label, Detail string
	OK, Blocking  bool
}

// describeJob gives a job its name for people and its guardrails: what it is
// called on the review, its button and its confirm dialog, whether it needs
// an encrypted backup first, and whether it cannot be undone.
func describeJob(f url.Values, spec *jobSpec) {
	key := f.Get("key")
	n := len(spec.items)
	keys := func(verb string) string {
		if n == 1 {
			return verb + " 1 key"
		}
		return fmt.Sprintf("%s %d keys", verb, n)
	}
	switch f.Get("action") {
	case "deploy":
		spec.title = "Deploy " + key
	case "rotate":
		spec.title, spec.backup = "Rotate "+key, true
	case "rotate-keys":
		spec.title, spec.backup = keys("Rotate"), true
	case "rollback":
		spec.title = "Roll " + key + " back"
	case "revoke":
		spec.title, spec.destroy = "Revoke "+key, true
	case "revoke-keys":
		spec.title, spec.destroy = keys("Revoke"), true
	case "delete-keys":
		spec.title, spec.destroy, spec.backup = keys("Delete"), true, checked(f, "purge")
	case "bundle-restore":
		spec.title = "Restore " + f.Get("bundle")
	case "knownhosts-init":
		spec.title = "Pin host keys"
	case "knownhosts-pin":
		spec.title = "Pin " + f.Get("host") + "'s host keys"
	}
	if spec.title == "" {
		spec.title = "Run the job"
	}
}

// preflights are the checks a job's review shows before it may start.
func preflights(f url.Values, spec jobSpec, busy bool) []preflight {
	checks := []preflight{{Label: "No other job is running", OK: !busy, Blocking: true,
		Detail: "One change runs at a time. Watch the one running, and start this when it ends."}}
	if spec.backup {
		checks = append(checks, preflight{Label: "A backup recipient is set", Blocking: true,
			OK:     envnames.Get(nil, envnames.AgeRecipient) != "",
			Detail: "An encrypted backup of ~/.ssh is written first, to the age recipient in Settings (backup.age_recipient, or AUTHAMA_AGE_RECIPIENT). Set one, then review again."})
	}
	if f.Get("action") == "bundle-restore" {
		checks = append(checks, preflight{Label: "An age identity is set to decrypt with", Blocking: true,
			OK:     envnames.Get(nil, envnames.AgeIdentityFile) != "",
			Detail: "Set paths.age_identity_file in Settings (AUTHAMA_AGE_IDENTITY_FILE); it cannot have a passphrase here."})
	}
	if reachesServers(f) {
		checks = append(checks, preflight{Label: "It reaches real servers", OK: true,
			Detail: "Hosts are logged in to with ssh, with no password prompt: a host that needs a password fails, and the rest go on."})
	}
	return checks
}

// blocked reports whether a blocking check failed.
func blocked(checks []preflight) bool {
	for _, c := range checks {
		if c.Blocking && !c.OK {
			return true
		}
	}
	return false
}

// reachesServers reports whether a job logs in to other machines: all but a
// bundle restore, pinning host keys (which only asks for them), and deleting
// keys without revoking them.
func reachesServers(f url.Values) bool {
	switch f.Get("action") {
	case "bundle-restore", "knownhosts-init", "knownhosts-pin":
		return false
	case "delete-keys":
		return checked(f, "revoke")
	}
	return true
}
