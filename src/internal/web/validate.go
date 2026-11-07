package web

import (
	"net/http"
	"strings"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/configsvc"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/services/validate"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// ValidateView is the /validate page: every check the validate commands run,
// on one page, run when it is asked for. It reads only.
type ValidateView struct {
	Sections []ValidateSection
	// Failed counts the sections that found a problem.
	Failed int
}

// ValidateSection is one check: what it covers, the command that runs the same
// check, and what it found.
type ValidateSection struct {
	ID, Title, What, Command string
	// Link is the page that shows more, or "".
	Link, LinkLabel string
	OK              bool
	Problems        []string
	// Detail is the command's own report, shown as it prints it, when it says
	// more than the problem lines (the ssh config check's diffs).
	Detail string
	// Keys is the key check's rows; the other sections have none.
	Keys []ValidateKey
}

// ValidateKey is one keypair's check, worded as `ssh validate` words it.
type ValidateKey struct {
	Profile, Name, Fingerprint string
	OK                         bool
	Issues, Notes              []string
}

// validatePage serves /validate, kept off pages, which the layering test holds
// to a ceiling of methods.
type validatePage struct{ s *Server }

func (v validatePage) handleValidate(w http.ResponseWriter, r *http.Request) {
	v.s.render(w, http.StatusOK, "validate", v.s.newView(r, "validate", "Validate", validateView(v.s.opts.Engine.Home())))
}

// validateView runs the checks for a home. A manifest that does not load fails
// its own section and leaves the two that need it (keys, ssh config) failed with
// the same reason, rather than the page failing as a whole: the settings check
// still has something to say.
func validateView(p paths.Paths) ValidateView {
	var v ValidateView
	m, mErr := manifest.Load(p.Manifest())
	v.add(manifestSection(p, mErr))
	v.add(settingsSection(p))
	v.add(keysSection(p, m, mErr))
	v.add(sshConfigSection(p, m, mErr))
	return v
}

func (v *ValidateView) add(s ValidateSection) {
	if !s.OK {
		v.Failed++
	}
	v.Sections = append(v.Sections, s)
}

func manifestSection(p paths.Paths, err error) ValidateSection {
	s := ValidateSection{ID: "manifest", Title: "Manifest", Command: brand.Command + " doctor",
		What: "The manifest parses and passes its own rules: names, references between profiles, hosts and keys.",
		Link: "/profiles", LinkLabel: "Profiles", OK: err == nil}
	if err != nil {
		s.Problems = []string{p.Manifest() + ": " + err.Error()}
	}
	return s
}

func settingsSection(p paths.Paths) ValidateSection {
	problems := settings.Problems(p.EnvFile(), p.SettingsFile())
	return ValidateSection{ID: "settings", Title: "Settings", Command: brand.Command + " settings validate",
		What: "authama.json against its schema, and .env's syntax and permissions. A file that is absent is not a problem.",
		Link: "/settings", LinkLabel: "Settings", OK: len(problems) == 0, Problems: problems}
}

func keysSection(p paths.Paths, m *manifest.Manifest, mErr error) ValidateSection {
	s := ValidateSection{ID: "keys", Title: "Keys", Command: brand.Command + " ssh validate",
		What: "Each managed keypair parses, its public key matches the private one, and its permissions are safe.",
		Link: "/keys", LinkLabel: "Keys"}
	if mErr != nil {
		s.Problems = []string{"not checked: the manifest did not load"}
		return s
	}
	checks, err := validate.New(m, p.SSHDir).ValidateKeys("")
	if err != nil {
		s.Problems = []string{err.Error()}
		return s
	}
	s.OK = true
	for _, ch := range checks {
		k := ValidateKey{Profile: ch.Profile, Name: ch.KeyName, OK: ch.OK, Issues: ch.Issues, Notes: ch.Notes}
		if ch.Fingerprint != nil {
			k.Fingerprint = *ch.Fingerprint
		}
		if !ch.OK {
			s.OK = false
			s.Problems = append(s.Problems, ch.Profile+"/"+ch.KeyName+": "+strings.Join(ch.Issues, "; "))
		}
		s.Keys = append(s.Keys, k)
	}
	return s
}

func sshConfigSection(p paths.Paths, m *manifest.Manifest, mErr error) ValidateSection {
	s := ValidateSection{ID: "sshconfig", Title: "SSH config", Command: brand.Command + " ssh config check",
		What: "The files in ~/.ssh match what the manifest renders, and every alias resolves.",
		Link: "/ssh-config", LinkLabel: "SSH config"}
	if mErr != nil {
		s.Problems = []string{"not checked: the manifest did not load"}
		return s
	}
	res, err := configsvc.New(p.SSHDir, m, platform.EmitUseKeychain()).Check(true)
	if err != nil {
		s.Problems = []string{err.Error()}
		return s
	}
	s.OK = res.InSync()
	if !s.OK {
		// The report ends with its verdict; the lines above it are the evidence,
		// diffs included, so they are shown as printed rather than as problems.
		report := strings.TrimSpace(res.Format())
		s.Detail = report
		s.Problems = []string{report[strings.LastIndex(report, "\n")+1:]}
	}
	return s
}
