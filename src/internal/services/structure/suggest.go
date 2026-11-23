// Package structure finds keys and key files that sit outside the layout
// authama expects - a pair loose in ~/.ssh, a key in the wrong profile
// directory, a key dropped in the config home - and turns each into a
// suggested move. It never moves anything on its own: a suggestion is a plan
// line the owner approves (`authama ssh structure approve`, or the web
// structure page), and Apply carries out approved plans only, with a backup
// of every file it moves and a history entry for each move.
package structure

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/keyaudit"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Kinds of suggestion.
const (
	KindLoose      = "loose"       // a key pair directly in ~/.ssh
	KindUntracked  = "untracked"   // a key in a declared profile's directory that no profile declares
	KindOrphan     = "orphan"      // a key in a key-root directory no profile owns
	KindMisplaced  = "misplaced"   // a declared key in another directory than the manifest resolves
	KindConfigHome = "config-home" // a key pair in the config home, where keys never belong
	KindCopy       = "copy"        // a leftover copy of a key authama manages: nothing to move
)

// Suggestion is one thing outside the expected structure, and what approving
// it would do. Paths read "~/.ssh/<rel>" or "config/<name>".
type Suggestion struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Profile string `json:"profile,omitempty"` // the profile it would join (a guess for a loose key)
	Name    string `json:"name,omitempty"`    // its key name in that profile
	To      string `json:"to,omitempty"`      // where it would be
	Detail  string `json:"detail"`
	// InUse lists the hand-written ssh config files that name the key as an
	// IdentityFile; approving the move rewrites those lines to the new path.
	InUse []string `json:"in_use,omitempty"`
	// Plan is the line that approves it ("" when a profile must be chosen
	// first, or when there is nothing to move).
	Plan string `json:"plan,omitempty"`
	// NeedsProfile is set when no profile could be guessed: approve it with
	// one named.
	NeedsProfile bool `json:"needs_profile,omitempty"`
	// Info marks a suggestion with nothing to approve: a note.
	Info      bool `json:"info,omitempty"`
	Dismissed bool `json:"dismissed,omitempty"`
	// Purpose-classification fields (KindPurpose / KindCopyRemove).
	Tool       string   `json:"tool,omitempty"`
	ToolKind   string   `json:"tool_kind,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

// ID is a suggestion's stable short id: the same file in the same state keeps
// it across scans, so a dismissal sticks.
func ID(kind, path string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + path))
	return hex.EncodeToString(sum[:])[:8]
}

// Suggest lists what sits outside the expected structure under p, sorted by
// path. A home without a manifest has no structure to compare with and gets
// nothing back.
func Suggest(p paths.Paths) ([]Suggestion, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	refs := ConfigRefs(p)
	var out []Suggestion
	out = append(out, looseIn(p, m, refs)...)
	out = append(out, keyTree(p, m)...)
	out = append(out, configHome(p, m)...)
	for i := range out {
		out[i].ID = ID(out[i].Kind, out[i].Path)
	}
	out, err = Classify(p, out)
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

// looseIn is the key pairs directly in ~/.ssh: neither reserved (kept on
// purpose), nor adopted where they are (tracked), nor a copy of a managed key
// (only the owner deletes those).
func looseIn(p paths.Paths, m *manifest.Manifest, refs map[string][]string) []Suggestion {
	adopted := map[string]bool{}
	for _, pname := range m.ProfileNames() {
		for _, k := range m.Profiles[pname].Keys {
			if k.Path != "" {
				adopted[k.Path] = true
			}
		}
	}
	var out []Suggestion
	for _, k := range keyaudit.LooseKeysIn(p.SSHDir, manifest.KeyRoot(m)) {
		if _, kept := m.ReservedKeys[k.Name]; kept || adopted[k.Name] {
			continue
		}
		s := Suggestion{Kind: KindLoose, Path: "~/.ssh/" + k.Name, Name: k.Name,
			InUse: refs[filepath.Join(p.SSHDir, k.Name)]}
		if k.CopyOf != "" {
			s.Kind, s.Info = KindCopy, true
			s.Detail = "a leftover copy of ~/.ssh/" + k.CopyOf + ", which authama manages; remove it yourself " +
				"once ssh through the managed copy works (authama never deletes a key it did not make)"
			out = append(out, s)
			continue
		}
		s.Detail = "a key loose in ~/.ssh: move it into a profile's directory so authama renders, rotates and audits it"
		if k.Devbox {
			s.Detail += "; its " + k.Hint + " suggests a devbox VM (`authama ssh vm adopt-key` wires it to one)"
		}
		if len(s.InUse) > 0 {
			s.Detail += "; the IdentityFile lines naming it are rewritten to the new path"
		}
		s.Profile = guessProfile(m, k.Name, k.Comment)
		finishMove(&s, p, m, "~/.ssh/"+k.Name)
		out = append(out, s)
	}
	return out
}

// keyTree is what sits in the key root's profile directories but not where
// the manifest resolves it.
func keyTree(p paths.Paths, m *manifest.Manifest) []Suggestion {
	owned := map[manifest.KeyRef]bool{}
	if all, err := m.KeyRefs(); err == nil {
		for _, ref := range all {
			owned[ref] = true
		}
	}
	seen := map[string]bool{}
	var out []Suggestion
	for _, path := range manifest.GlobKeyTree(p.SSHDir, manifest.KeyRoot(m), "*") {
		base := filepath.Base(path)
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() || strings.HasPrefix(base, ".") || base == "config" || base == "known_hosts" {
			continue
		}
		dir := filepath.Dir(path)
		name := strings.TrimSuffix(base, ".pub")
		priv := filepath.Join(dir, name)
		if seen[priv] {
			continue
		}
		seen[priv] = true
		loc, ok := m.LocateKeyPath(p.SSHDir, priv)
		if !ok {
			// A directory no profile owns: keep the old guess (the folder name).
			loc = manifest.LocateKey{Profile: filepath.Base(dir), Name: name}
		}
		if strings.HasSuffix(loc.Name, ".pub") {
			loc.Name = strings.TrimSuffix(loc.Name, ".pub")
		}
		ref := manifest.KeyRef{Profile: loc.Profile, KeyName: loc.Name}
		rel := relSSH(p, priv)
		expected := m.KeyPath(p.SSHDir, ref)
		switch {
		case owned[ref] && filepath.Dir(expected) == dir:
			continue
		case owned[ref]:
			if manifest.AdoptedPath(m, ref) != "" {
				continue // adopted elsewhere: this is a stray file of the same name, left alone
			}
			if _, err := os.Lstat(expected); err == nil {
				continue // the declared key is where it belongs; this is another file
			}
			out = append(out, Suggestion{Kind: KindMisplaced, Path: "~/.ssh/" + rel, Profile: ref.Profile,
				Name: ref.KeyName, To: "~/.ssh/" + relSSH(p, expected),
				Detail: "profile " + ref.Profile + " declares " + ref.KeyName + ", but the manifest places it elsewhere; " +
					"until it moves, authama cannot find it",
				Plan: "misplaced " + ref.Profile + "/" + ref.KeyName})
		default:
			if !pairedOrPub(priv) {
				continue
			}
			if _, ok := m.Profiles[ref.Profile]; ok && filepath.Dir(m.KeyPath(p.SSHDir, ref)) == dir {
				s := Suggestion{Kind: KindUntracked, Path: "~/.ssh/" + rel, Profile: ref.Profile, Name: ref.KeyName,
					Detail: "in profile " + ref.Profile + "'s directory, but no profile declares it: declare it there (nothing moves)"}
				finishMove(&s, p, m, "~/.ssh/"+rel)
				out = append(out, s)
				continue
			}
			s := Suggestion{Kind: KindOrphan, Path: "~/.ssh/" + rel, Name: ref.KeyName,
				Detail: "in " + filepath.ToSlash(filepath.Dir(rel)) + ", which no profile owns: move it into a profile " +
					"(or add a profile named " + ref.Profile + " first)"}
			finishMove(&s, p, m, "~/.ssh/"+rel)
			out = append(out, s)
		}
	}
	return out
}

// configHome is the key pairs dropped in the config home, which holds
// authama's configuration and never keys.
func configHome(p paths.Paths, m *manifest.Manifest) []Suggestion {
	var out []Suggestion
	for _, k := range keyaudit.LooseKeysIn(p.ConfigDir, "") {
		s := Suggestion{Kind: KindConfigHome, Path: "config/" + k.Name, Name: k.Name,
			Detail: "a key in the config home, which holds settings, not keys: move it into a profile's directory under ~/.ssh"}
		s.Profile = guessProfile(m, k.Name, k.Comment)
		finishMove(&s, p, m, "config/"+k.Name)
		out = append(out, s)
	}
	return out
}

// finishMove fills in where the key would go and the plan line that approves
// it, when the profile is known; else marks it as needing one.
func finishMove(s *Suggestion, p paths.Paths, m *manifest.Manifest, src string) {
	if s.Profile == "" {
		s.NeedsProfile = true
		return
	}
	if !planSafe(src) || !planSafe(s.Name) {
		s.NeedsProfile = false
		s.Info = true
		s.Detail += " (its name has spaces or quotes: rename it first)"
		return
	}
	s.To = "~/.ssh/" + relSSH(p, filepath.Join(m.ProfileDir(p.SSHDir, s.Profile), s.Name))
	s.Plan = KeyLine(src, s.Profile, s.Name)
}

// KeyLine is the plan line that moves the key at src into profile as name.
func KeyLine(src, profile, name string) string {
	line := "key " + src + " profile " + profile
	if name != "" && name != filepath.Base(strings.TrimPrefix(src, "config/")) {
		line += " name " + name
	}
	return line
}

// guessProfile picks the one profile whose name appears as a word in the
// key's name or comment ("id_ed25519_work", "me@work"); "" when none or
// several do.
func guessProfile(m *manifest.Manifest, name, comment string) string {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(name+" "+comment), func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '@' || r == ' ' || r == '/'
	}) {
		words[w] = true
	}
	found := ""
	for _, pname := range m.ProfileNames() {
		if words[strings.ToLower(pname)] {
			if found != "" {
				return ""
			}
			found = pname
		}
	}
	return found
}

func pairedOrPub(priv string) bool {
	_, e1 := os.Lstat(priv)
	_, e2 := os.Lstat(priv + ".pub")
	return e1 == nil && e2 == nil
}

func relSSH(p paths.Paths, abs string) string {
	rel, err := filepath.Rel(p.SSHDir, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

func planSafe(s string) bool { return s != "" && !strings.ContainsAny(s, " \t\n\"'\\") }
