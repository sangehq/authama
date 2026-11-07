package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/naming"
)

// NamingView is the /naming page: `ssh naming show` and `ssh naming check`,
// with the forms for `naming set`, `naming key-template` and a profile's code.
type NamingView struct {
	// Rules are the manifest's rules in effect; Own is what defaults.naming
	// holds, empty where it inherits the built-in default.
	Rules NamingRules
	Own   NamingRules
	// Profiles are every profile, in manifest order.
	Profiles []NamingProfile
	// Violations are the servers whose alias breaks the rules, as `naming
	// check` lists them.
	Violations []NamingViolation
	// Servers counts the hosts checked (VM hosts are devboxes, not checked).
	Servers int
	// Form is the manifest's rules form; each profile carries its own.
	Form NamingForm
}

// NamingForm is one rules form: what its scope stores now (each field
// prefilled with it) and what applies when a field is left empty (shown as
// the field's placeholder). The template shares one body between the
// manifest's form and every profile's.
type NamingForm struct {
	ID, Title string
	// Profile is "" for the manifest's rules.
	Profile string
	// Inherit names what an empty field falls back to: "the default" for the
	// manifest, "the manifest" for a profile.
	Inherit        string
	Own, Effective NamingRules
	CSRF           string
}

// NamingRules is one set of rules, worded for the page: lists joined with
// ", " and the start as its first number.
type NamingRules struct {
	Alias, KeyName, Envs, Roles, Enforce, CodeFallback string
	// Start is "" when the rules leave it unset, else "0" or "1".
	Start string
}

// NamingProfile is one profile's code and rules.
type NamingProfile struct {
	Name string
	// Code is what {code} is for the profile; CodeOwn is its own code,
	// "" when it has none and Code comes from the fallback (or is empty).
	Code, CodeOwn string
	// HasOwn is true when the profile overrides the manifest's rules; Own is
	// what it sets, Rules what is in effect.
	HasOwn     bool
	Own, Rules NamingRules
	Form       NamingForm
}

// NamingViolation is one server whose alias breaks its profile's rules.
type NamingViolation struct{ Profile, Message string }

func namingRules(r naming.Rules) NamingRules {
	return NamingRules{Alias: r.Alias, KeyName: r.KeyName, Envs: strings.Join(r.Envs, ", "),
		Roles: strings.Join(r.Roles, ", "), Enforce: r.Enforce, CodeFallback: r.CodeFallback, Start: startText(r.Start)}
}

func ownRules(n *manifest.Naming) NamingRules {
	if n == nil {
		return NamingRules{}
	}
	return NamingRules{Alias: n.Alias, KeyName: n.KeyName, Envs: strings.Join(n.Envs, ", "),
		Roles: strings.Join(n.Roles, ", "), Enforce: n.Enforce, CodeFallback: n.CodeFallback, Start: startText(n.Start)}
}

// startText is a stored start, which is 0 both when it is 0 and when unset.
func startText(n int) string {
	if n == 0 {
		return ""
	}
	return "1"
}

// namingView reads the manifest as `naming show` and `naming check` do. csrf
// goes into each form, which the template renders from its own value.
func namingView(m *manifest.Manifest, csrf string) NamingView {
	v := NamingView{Rules: namingRules(manifest.NamingRules(m)), Own: ownRules(m.Defaults.Naming)}
	// An empty manifest field falls back to the built-in default.
	v.Form = NamingForm{ID: "naming-rules", Title: "Change the naming rules", Inherit: "the default",
		Own: v.Own, Effective: namingRules(naming.Rules{}.WithDefaults()), CSRF: csrf}
	for i, name := range m.ProfileNames() {
		pr := m.Profiles[name]
		np := NamingProfile{Name: name, Code: manifest.ProfileCode(m, name), CodeOwn: pr.Code,
			HasOwn: pr.Naming != nil, Own: ownRules(pr.Naming), Rules: namingRules(manifest.ProfileNamingRules(m, name))}
		// Panel ids are by position: a profile name may hold a dot, which a
		// "#id" selector would read as a class.
		np.Form = NamingForm{ID: fmt.Sprintf("naming-p%d", i), Title: "Naming rules for " + name, Profile: name,
			Inherit: "the manifest", Own: np.Own, Effective: v.Rules, CSRF: csrf}
		v.Profiles = append(v.Profiles, np)
		for _, h := range pr.Hosts {
			if h.VM != nil {
				continue
			}
			v.Servers++
			if msg := manifest.NamingViolation(m, name, h.Alias); msg != "" {
				v.Violations = append(v.Violations, NamingViolation{Profile: name, Message: msg})
			}
		}
	}
	return v
}

// namingPage serves /naming, kept off pages, which the layering test holds to
// a ceiling of methods.
type namingPage struct{ s *Server }

func (np namingPage) handleNaming(w http.ResponseWriter, r *http.Request) {
	h := pages(np)
	m, err := manifest.Load(h.s.opts.Engine.Home().Manifest())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v := h.s.newView(r, "naming", "Naming", nil)
	v.Data = namingView(m, v.CSRF)
	h.s.render(w, http.StatusOK, "naming", v)
}
