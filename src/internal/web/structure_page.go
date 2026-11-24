package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/events"
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
	Rules       []structure.Rule
	// WatchMode is "watching" (fsnotify) or "polling" (the 2s fallback).
	WatchMode string
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
	Tools               []StructureToolFolder
	Moves               []StructureOption // to another category
}

// StructureKey is one key a profile declares, with the profiles it can move
// to (none when it is kept where it is, or in 1Password).
type StructureKey struct {
	Name, Path string
	Present    bool
	Adopted    bool // kept where it is (`key adopt`): never moved
	Moves      []StructureOption
}

// StructureHost is one host and the key it uses, with the profiles it can
// move to.
type StructureHost struct {
	Alias, Hostname, Key string
	Moves                []StructureOption
}

// StructureOption is one choice in a move select: its plan line and the
// category or profile it moves to (what a drop target names).
type StructureOption struct {
	Line, Target, Label string
}

// StructureToolFolder is a tool's folder as one profile owns it: the shared
// <key root>/tools/<tool>/ with that profile's keys in it, shown under the
// profile so each tool key stays grouped with its owner; a drop target for
// purpose moves.
type StructureToolFolder struct {
	Tool, Dir string
	Keys      []StructureKey
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
	mode, _ := sp.s.watchMode.Load().(string)
	if mode == "" {
		mode = events.ModePolling
	}
	v := StructureView{ShowAll: showAll, WatchMode: mode,
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
	v.Rules, _ = structure.LoadRules(p)

	all, checked, err := structure.Current(p)
	if err != nil {
		v.Groups = structureGroups(p, m, nil)
		return v, err
	}
	v.Groups = structureGroups(p, m, all)
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
func structureGroups(p paths.Paths, m *manifest.Manifest, suggestions []structure.Suggestion) []StructureGroup {
	keysOf := map[string][]StructureKey{}
	if refs, err := m.KeyRefs(); err == nil {
		for _, ref := range refs {
			path := m.KeyPath(p.SSHDir, ref)
			rel, _ := filepath.Rel(p.SSHDir, path)
			_, err := os.Lstat(path)
			if m.Profiles[ref.Profile].InOnePassword() {
				_, err = os.Lstat(path + ".pub")
			}
			k := StructureKey{Name: ref.KeyName, Path: "~/.ssh/" + filepath.ToSlash(rel), Present: err == nil,
				Adopted: manifest.AdoptedPath(m, ref) != ""}
			if !k.Adopted {
				k.Moves = crossMoves(m, ref.Profile, func(to string) structure.Move {
					return structure.Move{Kind: structure.MoveDeclared, From: ref.Profile, Key: ref.KeyName, Name: ref.KeyName, Profile: to}
				})
			}
			keysOf[ref.Profile] = append(keysOf[ref.Profile], k)
		}
	}
	hostsOf := map[string][]StructureHost{}
	if res, err := m.IterResolved(); err == nil {
		for _, rk := range res {
			alias, from := rk.Host.Alias, rk.Profile
			hostsOf[rk.Profile] = append(hostsOf[rk.Profile], StructureHost{Alias: alias, Hostname: rk.Host.Hostname, Key: rk.KeyName,
				Moves: crossMoves(m, from, func(to string) structure.Move {
					return structure.Move{Kind: structure.MoveHost, From: from, Name: alias, Profile: to}
				})})
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
			hostKeys, toolFolders := splitToolKeys(p, m, name, keysOf[name], purposeTools(suggestions, name))
			sp := StructureProfile{Name: name, Category: c, Dir: "~/.ssh/" + m.ProfileRel(name),
				OnePassword: prof.InOnePassword(), Keys: hostKeys, Hosts: hostsOf[name], Tools: toolFolders}
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

// crossMoves is what a key's or host's select offers: a move into each other
// profile that keeps its keys on disk. A profile in 1Password offers none.
func crossMoves(m *manifest.Manifest, from string, move func(to string) structure.Move) []StructureOption {
	if m.Profiles[from].InOnePassword() {
		return nil
	}
	var out []StructureOption
	for _, to := range m.ProfileNames() {
		if to == from || m.Profiles[to].InOnePassword() {
			continue
		}
		out = append(out, StructureOption{Line: move(to).Line(), Target: "profile:" + to, Label: "into " + to})
	}
	return out
}

// suggestionMoves is what a suggestion's select offers: its own plan when
// it has one, else a move into each profile that keeps keys on disk.
func suggestionMoves(m *manifest.Manifest, s structure.Suggestion) []StructureOption {
	switch {
	case s.Kind == structure.KindCopyRemove && s.Plan != "":
		return []StructureOption{{Line: s.Plan, Target: "copy-remove", Label: "delete leftover copy (keep the managed key)"}}
	case s.Info:
		return nil
	case s.Kind == structure.KindMisplaced:
		return []StructureOption{{Line: s.Plan, Target: "profile:" + s.Profile, Label: "to " + s.To}}
	}
	var out []StructureOption
	if s.Kind == structure.KindPurpose && s.Plan != "" {
		out = append(out, StructureOption{Line: s.Plan, Target: "tool:" + s.Profile + ":" + s.Tool, Label: "into tools/" + s.Tool + "/ (suggested)"})
	}
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
		for _, tool := range manifest.BuiltinToolSlugs() {
			if s.Kind == structure.KindPurpose && name == s.Profile && tool == s.Tool {
				continue
			}
			line := "purpose " + name + "/" + purposeKeyName(s, name) + " tool " + tool
			out = append(out, StructureOption{Line: line, Target: "tool:" + name + ":" + tool, Label: "into tools/" + tool + "/ (owned by " + name + ")"})
		}
	}
	return out
}

func purposeKeyName(s structure.Suggestion, profile string) string {
	if s.Name != "" {
		return s.Name
	}
	return strings.TrimSuffix(filepath.Base(s.Path), "")
}

func purposeTools(suggestions []structure.Suggestion, profile string) []string {
	var out []string
	for _, s := range suggestions {
		if s.Dismissed || s.Kind != structure.KindPurpose || s.Profile != profile || s.Tool == "" {
			continue
		}
		if !slices.Contains(out, s.Tool) {
			out = append(out, s.Tool)
		}
	}
	return out
}

func splitToolKeys(_ paths.Paths, m *manifest.Manifest, profile string, keys []StructureKey, extra []string) ([]StructureKey, []StructureToolFolder) {
	byTool := map[string][]StructureKey{}
	var hostKeys []StructureKey
	for _, k := range keys {
		spec, ok := m.KeySpecFor(manifest.KeyRef{Profile: profile, KeyName: k.Name})
		if ok && (spec.Role == manifest.RoleTool || spec.Tool != "") {
			byTool[spec.Tool] = append(byTool[spec.Tool], k)
			continue
		}
		hostKeys = append(hostKeys, k)
	}
	var folders []StructureToolFolder
	seen := map[string]bool{}
	for _, tool := range manifest.BuiltinToolSlugs() {
		if len(byTool[tool]) == 0 && !slices.Contains(extra, tool) {
			continue
		}
		seen[tool] = true
		folders = append(folders, StructureToolFolder{
			Tool: tool, Dir: "~/.ssh/" + manifest.KeyRoot(m) + "/" + manifest.ToolsDir + "/" + tool, Keys: byTool[tool],
		})
	}
	for tool, ks := range byTool {
		if seen[tool] {
			continue
		}
		folders = append(folders, StructureToolFolder{
			Tool: tool, Dir: "~/.ssh/" + manifest.KeyRoot(m) + "/" + manifest.ToolsDir + "/" + tool, Keys: ks,
		})
	}
	return hostKeys, folders
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

func structureRuleAddChange(f url.Values, errs []error) (change.Change, []error) {
	tool := strings.TrimSpace(f.Get("tool"))
	if tool == "" {
		return change.Change{}, append(errs, badForm("a rule needs --tool (the tools/<tool>/ slug)"))
	}
	r := structure.Rule{
		Profile:     strings.TrimSpace(f.Get("profile")),
		NameGlob:    strings.TrimSpace(f.Get("name")),
		CommentRe:   strings.TrimSpace(f.Get("comment")),
		Fingerprint: strings.TrimSpace(f.Get("fingerprint")),
		Tool:        tool,
		ToolKind:    strings.TrimSpace(f.Get("kind")),
		AutoApply:   checked(f, "auto") || f.Get("auto") == "1",
	}
	if r.NameGlob == "" && r.CommentRe == "" && r.Fingerprint == "" && r.Profile == "" {
		return change.Change{}, append(errs, badForm("a rule needs a name glob, comment, fingerprint or profile"))
	}
	return change.StructureRuleAdd(r), errs
}

func structureRuleAutoChange(f url.Values, errs []error) (change.Change, []error) {
	id := strings.TrimSpace(f.Get("id"))
	if id == "" {
		return change.Change{}, append(errs, badForm("name the rule id"))
	}
	auto := f.Get("auto") != "0" && f.Get("auto") != "false" && f.Get("auto") != ""
	return change.StructureRuleAuto(id, auto), errs
}

func structureRuleRemoveChange(f url.Values, errs []error) (change.Change, []error) {
	id := strings.TrimSpace(f.Get("id"))
	if id == "" {
		return change.Change{}, append(errs, badForm("name the rule id"))
	}
	return change.StructureRuleRemove(id), errs
}
