package change

import (
	"fmt"
	"io"
	"strings"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/naming"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// NamingFields are the naming rules a NamingSet change sets. A nil field is
// left as it is, as an unset flag of `ssh naming set` is; an empty one goes
// back to inheriting.
type NamingFields struct {
	Alias, Enforce, CodeFallback *string
	Envs, Roles                  *[]string
	Start                        *int
}

// Empty is true when the change would set nothing.
func (f NamingFields) Empty() bool {
	return f.Alias == nil && f.Enforce == nil && f.CodeFallback == nil && f.Envs == nil && f.Roles == nil && f.Start == nil
}

// args is the flags `ssh naming set` takes for f, in the order its help lists them.
func (f NamingFields) args() []string {
	var out []string
	if f.Alias != nil {
		out = append(out, "--alias="+*f.Alias)
	}
	if f.Envs != nil {
		out = append(out, "--envs="+strings.Join(*f.Envs, ","))
	}
	if f.Roles != nil {
		out = append(out, "--roles="+strings.Join(*f.Roles, ","))
	}
	if f.Enforce != nil {
		out = append(out, "--enforce="+*f.Enforce)
	}
	if f.Start != nil {
		out = append(out, fmt.Sprintf("--start=%d", *f.Start))
	}
	if f.CodeFallback != nil {
		out = append(out, "--code-fallback="+*f.CodeFallback)
	}
	return out
}

func (f NamingFields) apply(n *manifest.Naming) {
	if f.Alias != nil {
		n.Alias = *f.Alias
	}
	if f.Envs != nil {
		n.Envs = nilIfNone(*f.Envs)
	}
	if f.Roles != nil {
		n.Roles = nilIfNone(*f.Roles)
	}
	if f.Enforce != nil {
		n.Enforce = *f.Enforce
	}
	if f.Start != nil {
		n.Start = *f.Start
	}
	if f.CodeFallback != nil {
		n.CodeFallback = *f.CodeFallback
	}
}

func nilIfNone(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return list
}

// namingSubjects names the profile a naming change is for, or none for the
// manifest's own rules.
func namingSubjects(profile string) []history.Subject {
	if profile == "" {
		return nil
	}
	return []history.Subject{{Kind: history.KindProfile, Name: profile}}
}

// writeKept reports, as the CLI does, the hosts whose key name a change kept.
func writeKept(out io.Writer, kept []string) {
	for _, h := range kept {
		_, _ = fmt.Fprintf(out, "  kept its key name: %s\n", h)
	}
}

// NamingSet is `authama ssh naming set [--profile <p>] <flags>`: the
// manifest's naming rules, or one profile's own. Every existing host keeps
// its key name.
func NamingSet(profile string, f NamingFields) Change {
	args := f.args()
	if profile != "" {
		args = append([]string{"--profile", profile}, args...)
	}
	return Change{
		Command: "naming set", Args: args, Subjects: namingSubjects(profile),
		Run: func(p paths.Paths, out io.Writer) error {
			kept, err := editor.SetNaming(editor.New(p), profile, f.apply)
			if err != nil {
				return err
			}
			where := "every profile"
			if profile != "" {
				where = profile
			}
			_, _ = fmt.Fprintf(out, "naming rules saved for %s\n", where)
			writeKept(out, kept)
			return nil
		},
	}
}

// NamingKeyTemplate is `authama ssh naming key-template <tpl> [--profile <p>]
// --yes`: how new keys are named. Every existing host keeps its key name.
func NamingKeyTemplate(profile, tpl string) Change {
	args := []string{tpl, "--yes"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return Change{
		Command: "naming key-template", Args: args, Subjects: namingSubjects(profile),
		Run: func(p paths.Paths, out io.Writer) error {
			if err := naming.CheckKeyName(tpl); err != nil {
				return err
			}
			kept, err := editor.SetKeyTemplate(editor.New(p), profile, tpl)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "new keys are named %s\n", tpl)
			writeKept(out, kept)
			return nil
		},
	}
}

// ProfileCode is `authama ssh profile edit <name> --code <code>`: the code a
// profile's servers are named by ("" clears it). Every existing host keeps its
// key name; the config is rendered again, as the command does.
func ProfileCode(name, code string) Change {
	return Change{
		Command: "profile edit", Args: []string{name, "--code=" + code},
		Subjects: []history.Subject{{Kind: history.KindProfile, Name: name}},
		Run: func(p paths.Paths, out io.Writer) error {
			kept, err := editor.SetProfileCode(editor.New(p), name, code)
			if err != nil {
				return err
			}
			writeKept(out, kept)
			_, _ = fmt.Fprintf(out, "edited profile %s (run `authama ssh reconcile` to mint any key "+
				"this now points at)\n", name)
			return ApplyManifestEdit(out, p)
		},
	}
}
