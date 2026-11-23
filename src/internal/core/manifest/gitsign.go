package manifest

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// ProfileGit is the git identity a profile commits as, and whether its commits
// and tags are signed with one of its SSH keys. `authama ssh git sign apply`
// renders it into a git config fragment for the profile, one allowed_signers
// file shared by every profile, and an includeIf per directory in the global
// git config, so a checkout under one of Dirs commits as this profile.
type ProfileGit struct {
	// Email is user.email, and the principal the allowed_signers line names.
	Email string `json:"email"`
	// Name is user.name; empty leaves whatever the global config says.
	Name string `json:"name,omitempty"`
	// SigningKey names the profile's key that signs; empty uses the key of the
	// profile's github.com host.
	SigningKey string `json:"signing_key,omitempty"`
	// Sign turns commit and tag signing on; false writes the identity alone.
	Sign bool `json:"sign"`
	// Dirs are the directories whose checkouts use this profile, in the form
	// git's gitdir: condition takes (~/code/acme/). Each one is an includeIf.
	Dirs []string `json:"dirs,omitempty"`
}

// GitDir is d as a gitdir: pattern: always ending in a slash, so it matches
// every checkout below the directory and not only one named like it.
func GitDir(d string) string {
	d = strings.ReplaceAll(strings.TrimSpace(d), `\`, "/")
	if !strings.HasSuffix(d, "/") {
		d += "/"
	}
	return d
}

// checkProfileGit validates every profile's git block, and that no directory
// is claimed by two profiles: git would apply both includes, and the later one
// would quietly win.
func checkProfileGit(m *Manifest) error {
	claimed := map[string]string{}
	names := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		g := m.Profiles[name].Git
		if g == nil {
			continue
		}
		field := func(f string) string { return fmt.Sprintf("profile %q git.%s", name, f) }
		if err := checkGitEmail(field("email"), g.Email); err != nil {
			return err
		}
		if err := checkGitText(field("name"), g.Name); err != nil {
			return err
		}
		if g.SigningKey != "" {
			if err := safeSegment(field("signing_key"), g.SigningKey); err != nil {
				return err
			}
		}
		for _, d := range g.Dirs {
			if err := checkGitDir(field("dirs"), d); err != nil {
				return err
			}
			key := GitDir(d)
			if other, ok := claimed[key]; ok {
				return fmt.Errorf("%s: %q is already profile %q's directory", field("dirs"), d, other)
			}
			claimed[key] = name
		}
	}
	return nil
}

// checkGitEmail accepts one address: it is a principal in allowed_signers,
// where whitespace ends it, a comma separates two and a quote opens an option.
func checkGitEmail(field, v string) error {
	if v == "" {
		return fmt.Errorf("%s is empty: git needs the address the commits are made as", field)
	}
	if err := rejectControl(field, v); err != nil {
		return err
	}
	at := strings.Index(v, "@")
	if at <= 0 || at != strings.LastIndex(v, "@") || at == len(v)-1 || strings.HasPrefix(v, "-") ||
		strings.ContainsAny(v, `",<>\;#`) || strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%s=%q is not one email address", field, v)
	}
	return nil
}

// checkGitText refuses what would end a git config value early.
func checkGitText(field, v string) error {
	if err := rejectControl(field, v); err != nil {
		return err
	}
	if strings.ContainsAny(v, `"\`) {
		return fmt.Errorf("%s=%q must not contain a quote or a backslash", field, v)
	}
	return nil
}

// checkGitDir accepts an absolute directory or one under ~/; a relative
// pattern would match a directory of that name anywhere.
func checkGitDir(field, d string) error {
	if strings.TrimSpace(d) == "" {
		return fmt.Errorf("%s has an empty entry", field)
	}
	if err := checkGitText(field, d); err != nil {
		return err
	}
	if strings.Contains(d, "]") {
		return fmt.Errorf("%s=%q must not contain ]", field, d)
	}
	abs := strings.HasPrefix(d, "/") || strings.HasPrefix(d, "~/") ||
		len(d) > 2 && d[1] == ':' && (d[2] == '/' || d[2] == '\\')
	if !abs {
		return fmt.Errorf("%s=%q must be absolute or start with ~/ (a relative pattern matches that name anywhere)", field, d)
	}
	return nil
}
