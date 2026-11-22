package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// structurePage is /structure: the SSH folder structure as a tree -
// categories, profiles, their keys and hosts - beside the keys the structure
// watcher found outside it. Every move is chosen with a select (the keyboard
// and no-script way) or by dragging an item onto a category or profile,
// which sets the same select; the moves then go through the change preview
// (action structure-plan) like every other change, so the files that move
// and the manifest and config diffs are shown before anything is written.
type structurePage struct{ s *Server }

// StructureView is the /structure page.
type StructureView struct {
	Root        string // the key root, as ~/.ssh/<root>
	Groups      []StructureGroup
	Suggestions []StructureSuggestion
	Dismissed   []StructureSuggestion
	ShowAll     bool
	Checked     time.Time
	Categories  []string
	ListCmd     string
	InstallCmd  string
}

// StructureGroup is one category (Name "" is the profiles in none).
type StructureGroup struct {
	Name     string
	Dir      string
	Profiles []StructureProfile
}

// StructureProfile is one profile in the tree, with the moves its select offers.
type StructureProfile struct {
	Name, Category, Dir string
	OnePassword         bool
	Keys                []StructureKey
	Hosts               []StructureHost
	Moves               []StructureOption // to another category
}

// StructureKey is one key a profile declares.
type StructureKey struct {
	Name, Path string
	Present    bool
	Adopted    bool // kept where it is (`key adopt`): never moved
}

// StructureHost is one host and the key it uses.
type StructureHost struct{ Alias, Hostname, Key string }

// StructureOption is one choice in a move select: its plan line and the
// category or profile it moves to (what a drop target names).
type StructureOption struct {
	Line, Target, Label string
}

// StructureSuggestion is one key outside the layout, with its move choices.
type StructureSuggestion struct {
	structure.Suggestion
	Moves []StructureOption
}

func (sp structurePage) handleStructure(w http.ResponseWriter, r *http.Request) {
	v, err := sp.view(r.URL.Query().Get("dismissed") == "1")
	if err != nil {
		sp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	sp.s.render(w, http.StatusOK, "structure", sp.s.newView(r, "structure", "Structure", v))
}

func (sp structurePage) view(showAll bool) (StructureView, error) {
	p := sp.s.opts.Engine.Home()
	v := StructureView{ShowAll: showAll,
		ListCmd:    brand.Command + " ssh structure list",
		InstallCmd: brand.Command + " ssh structure install"}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		if os.IsNotExist(err) {
			return v, nil
		}
		return v, err
	}
	v.Root = "~/.ssh/" + manifest.KeyRoot(m)
	v.Categories = m.Categories()
	v.Groups = structureGroups(p, m)

	all, checked, err := structure.Current(p)
	if err != nil {
		return v, err
	}
	v.Checked = checked
	for _, s := range all {
		row := StructureSuggestion{Suggestion: s, Moves: suggestionMoves(m, s)}
		if s.Dismissed {
			v.Dismissed = append(v.Dismissed, row)
		} else {
			v.Suggestions = append(v.Suggestions, row)
		}
	}
	return v, nil
}

// structureGroups lays the manifest out as the tree: the profiles in no
// category first, then each category in manifest order.
func structureGroups(p paths.Paths, m *manifest.Manifest) []StructureGroup {
	keysOf := map[string][]StructureKey{}
	if refs, err := m.KeyRefs(); err == nil {
		for _, ref := range refs {
			path := m.KeyPath(p.SSHDir, ref)
			rel, _ := filepath.Rel(p.SSHDir, path)
			_, err := os.Lstat(path)
			if m.Profiles[ref.Profile].InOnePassword() {
				_, err = os.Lstat(path + ".pub")
			}
			keysOf[ref.Profile] = append(keysOf[ref.Profile], StructureKey{Name: ref.KeyName,
				Path: "~/.ssh/" + filepath.ToSlash(rel), Present: err == nil,
				Adopted: manifest.AdoptedPath(m, ref) != ""})
		}
	}
	hostsOf := map[string][]StructureHost{}
	if res, err := m.IterResolved(); err == nil {
		for _, rk := range res {
			hostsOf[rk.Profile] = append(hostsOf[rk.Profile], StructureHost{Alias: rk.Host.Alias, Hostname: rk.Host.Hostname, Key: rk.KeyName})
		}
	}
	cats := append([]string{""}, m.Categories()...)
	groups := make([]StructureGroup, 0, len(cats))
	for _, c := range cats {
		g := StructureGroup{Name: c, Dir: "~/.ssh/" + manifest.KeyRoot(m)}
		if c != "" {
			g.Dir += "/" + c
		}
		for _, name := range m.ProfileNames() {
			prof := m.Profiles[name]
			if prof.Category != c {
				continue
			}
			sp := StructureProfile{Name: name, Category: c, Dir: "~/.ssh/" + m.ProfileRel(name),
				OnePassword: prof.InOnePassword(), Keys: keysOf[name], Hosts: hostsOf[name]}
			for _, to := range cats {
				if to == c {
					continue
				}
				line := structure.Move{Kind: structure.MoveCategory, Profile: name, Category: to}.Line()
				label := "into " + to
				if to == "" {
					label = "out of its category"
				}
				sp.Moves = append(sp.Moves, StructureOption{Line: line, Target: "category:" + to, Label: label})
			}
			g.Profiles = append(g.Profiles, sp)
		}
		if c == "" && len(g.Profiles) == 0 && len(cats) > 1 {
			continue // no uncategorised profiles: no empty group, unless it is the only one
		}
		groups = append(groups, g)
	}
	return groups
}

// suggestionMoves is what a suggestion's select offers: its own plan when
// it has one, else a move into each profile that keeps keys on disk.
func suggestionMoves(m *manifest.Manifest, s structure.Suggestion) []StructureOption {
	switch {
	case s.Info:
		return nil
	case s.Kind == structure.KindMisplaced:
		return []StructureOption{{Line: s.Plan, Target: "profile:" + s.Profile, Label: "to " + s.To}}
	}
	var out []StructureOption
	for _, name := range m.ProfileNames() {
		if m.Profiles[name].InOnePassword() {
			continue
		}
		label := "into " + name
		if name == s.Profile {
			label += " (suggested)"
		}
		out = append(out, StructureOption{Line: structure.KeyLine(s.Path, name, s.Name),
			Target: "profile:" + name, Label: label})
	}
	return out
}

// structurePlanChange is the structure-plan action: the move selects (each
// a plan line, or empty for none) and the free-form plan lines, as one plan.
func structurePlanChange(f url.Values, errs []error) (change.Change, []error) {
	var lines []string
	for _, v := range f["move"] {
		if v = strings.TrimSpace(v); v != "" {
			lines = append(lines, v)
		}
	}
	if extra := strings.TrimSpace(f.Get("moves")); extra != "" {
		lines = append(lines, extra)
	}
	if len(lines) == 0 {
		return change.Change{}, append(errs, badForm("choose at least one move: a select, a drag, or a plan line"))
	}
	moves, err := structure.ParsePlan(strings.Join(lines, "\n"))
	if err != nil {
		return change.Change{}, append(errs, badForm("%s", err.Error()))
	}
	return change.StructurePlan(moves), errs
}

// structureDismissChange is the structure-dismiss action: hides a suggestion
// (or, with undo, shows it again). Nothing on disk moves.
func structureDismissChange(f url.Values, errs []error) (change.Change, []error) {
	id := strings.TrimSpace(f.Get("id"))
	if len(id) != 8 || strings.Trim(id, "0123456789abcdef") != "" {
		return change.Change{}, append(errs, badForm("not a suggestion id: %q", id))
	}
	return change.StructureDismiss(id, checked(f, "undo")), errs
}
