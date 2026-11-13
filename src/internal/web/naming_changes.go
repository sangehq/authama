package web

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/naming"
	"github.com/sangehq/authama/src/internal/services/change"
)

// profileCodeRe is the manifest's rule for a profile's code.
var profileCodeRe = regexp.MustCompile(`^[a-z0-9]*$`)

// namingChange builds the Naming page's three changes. The rules form posts
// every field with what the scope stores now; only the fields that differ
// become flags, so the change reads as the `naming set` a person would type
// and a field left alone is never written.
func namingChange(action string, f url.Values, h home) (change.Change, error) {
	m, err := manifest.Load(h.p.Manifest())
	if err != nil {
		return change.Change{}, err
	}
	profile, err := optionalName(f, "profile")
	if err != nil {
		return change.Change{}, err
	}
	if _, ok := m.Profiles[profile]; profile != "" && !ok {
		return change.Change{}, badForm("unknown profile %q", profile)
	}
	switch action {
	case "naming-set":
		var cur manifest.Naming
		if n := scopeNaming(m, profile); n != nil {
			cur = *n
		}
		fields, err := namingFields(f, cur)
		if err != nil {
			return change.Change{}, err
		}
		if fields.Empty() {
			return change.Change{}, badForm("nothing to change: every field is as it is now")
		}
		return change.NamingSet(profile, fields), nil
	case "naming-key-template":
		tpl, err := plain(f, "key_name")
		if err != nil {
			return change.Change{}, err
		}
		if err := naming.CheckKeyName(tpl); err != nil {
			return change.Change{}, badForm("%s", err.Error())
		}
		return change.NamingKeyTemplate(profile, tpl), nil
	default: // profile-code
		pname, err := name(f, "name")
		if err != nil {
			return change.Change{}, err
		}
		pr, ok := m.Profiles[pname]
		if !ok {
			return change.Change{}, badForm("unknown profile %q", pname)
		}
		code, err := plain(f, "code")
		if err != nil {
			return change.Change{}, err
		}
		if !profileCodeRe.MatchString(code) {
			return change.Change{}, badForm("a code is lowercase letters and digits")
		}
		if code == pr.Code {
			return change.Change{}, badForm("nothing to change: %s's code is already %q", pname, code)
		}
		return change.ProfileCode(pname, code), nil
	}
}

// scopeNaming is what the manifest stores for a scope: defaults.naming when
// profile is "", else that profile's own naming.
func scopeNaming(m *manifest.Manifest, profile string) *manifest.Naming {
	if profile == "" {
		return m.Defaults.Naming
	}
	return m.Profiles[profile].Naming
}

// namingFields is the form's rules, each one set only where it differs from cur.
func namingFields(f url.Values, cur manifest.Naming) (change.NamingFields, error) {
	var out change.NamingFields
	var errs []error
	text := func(field, now string, allowed ...string) *string {
		v, err := plain(f, field)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if len(allowed) > 0 && v != "" && !slices.Contains(allowed, v) {
			errs = append(errs, badForm("%s must be one of %s", field, strings.Join(allowed, ", ")))
			return nil
		}
		if v == now {
			return nil
		}
		return &v
	}
	list := func(field string, now []string) *[]string {
		v, err := plain(f, field)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		var items []string
		for _, it := range strings.Split(v, ",") {
			if it = strings.TrimSpace(it); it == "" {
				continue
			}
			if !nameRe.MatchString(it) {
				errs = append(errs, badForm("%s: %q is not a name authama accepts", field, it))
				return nil
			}
			items = append(items, it)
		}
		if slices.Equal(items, now) {
			return nil
		}
		if items == nil {
			items = []string{}
		}
		return &items
	}
	out.Alias = text("alias", cur.Alias)
	out.Envs = list("envs", cur.Envs)
	out.Roles = list("roles", cur.Roles)
	out.Enforce = text("enforce", cur.Enforce, naming.EnforceOff, naming.EnforceWarn, naming.EnforceError)
	out.CodeFallback = text("code_fallback", cur.CodeFallback, naming.CodeNone, naming.CodeProfile)
	switch s := strings.TrimSpace(f.Get("start")); s {
	case "", "0", "1":
		if n := map[string]int{"": 0, "0": 0, "1": 1}[s]; n != cur.Start {
			out.Start = &n
		}
	default:
		errs = append(errs, badForm("start must be 0 or 1"))
	}
	return out, errors.Join(errs...)
}
