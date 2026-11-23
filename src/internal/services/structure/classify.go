package structure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/keyaudit"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Purpose kind and leftover-copy removal (an approved, snapshotted delete).
const (
	KindPurpose    = "purpose"     // move a managed key into tools/<tool>/
	KindCopyRemove = "copy-remove" // delete a leftover copy of a managed key
)

// Classify adds purpose and copy-remove suggestions to the layout list.
// It never moves files.
func Classify(p paths.Paths, layout []Suggestion) ([]Suggestion, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		if os.IsNotExist(err) {
			return layout, nil
		}
		return nil, err
	}
	out := append([]Suggestion{}, layout...)
	out = upgradeCopySuggestions(out, p)
	purpose, err := purposeSuggestions(p, m)
	if err != nil {
		return nil, err
	}
	out = append(out, purpose...)
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = ID(out[i].Kind, out[i].Path)
		}
	}
	return out, nil
}

func upgradeCopySuggestions(ss []Suggestion, p paths.Paths) []Suggestion {
	refs := ConfigRefs(p)
	for i, s := range ss {
		if s.Kind != KindCopy {
			continue
		}
		// A leftover with no remaining IdentityFile (or other) references
		// becomes an approvable copy-remove. If something still points at it,
		// it stays an info-only KindCopy.
		abs := filepath.Join(p.SSHDir, strings.TrimPrefix(s.Path, "~/.ssh/"))
		if len(refs[abs]) > 0 {
			continue
		}
		ss[i].Kind = KindCopyRemove
		ss[i].Info = false
		ss[i].Plan = "copy-remove " + s.Path
		ss[i].Detail = s.Name + " is a leftover copy of a managed key with no remaining references; approving deletes it after a snapshot (the managed copy is kept)"
		if s.Detail != "" && strings.Contains(s.Detail, "copy of") {
			ss[i].Evidence = []string{s.Detail}
		}
	}
	return ss
}

func purposeSuggestions(p paths.Paths, m *manifest.Manifest) ([]Suggestion, error) {
	var out []Suggestion
	refs := ConfigRefs(p)
	gitKeys := signingKeyPaths(p)
	dockerHosts := dockerSSHHosts(p)
	for _, pname := range m.ProfileNames() {
		prof := m.Profiles[pname]
		seen := map[string]bool{}
		// declared keys and host key_names
		var names []string
		for _, k := range prof.Keys {
			names = append(names, k.Name)
		}
		for _, h := range prof.Hosts {
			if h.KeyName != nil && *h.KeyName != "" {
				names = append(names, *h.KeyName)
			}
		}
		for _, name := range names {
			if seen[name] {
				continue
			}
			seen[name] = true
			ref := manifest.KeyRef{Profile: pname, KeyName: name}
			spec, _ := m.KeySpecFor(ref)
			if spec.EffectiveRole() == manifest.RoleTool && spec.Tool != "" {
				continue // already classified
			}
			abs := m.KeyPath(p.SSHDir, ref)
			if _, err := os.Lstat(abs); err != nil {
				continue
			}
			comment := pubComment(abs + ".pub")
			c := classifyKey(p, m, pname, name, abs, comment, refs[abs], gitKeys, dockerHosts, prof)
			if c.tool == "" || c.confidence < 0.50 {
				continue
			}
			to := "~/.ssh/" + m.ProfileRel(pname) + "/tools/" + c.tool + "/" + name
			s := Suggestion{
				Kind:       KindPurpose,
				Path:       "~/.ssh/" + m.KeyRel(ref),
				Profile:    pname,
				Name:       name,
				To:         to,
				Tool:       c.tool,
				ToolKind:   c.kind,
				Confidence: c.confidence,
				Evidence:   c.evidence,
				Detail:     "move into tools/" + c.tool + "/ (" + strings.Join(c.evidence, "; ") + ")",
				Plan:       PurposeLine(pname, name, c.tool, c.kind),
			}
			out = append(out, s)
		}
	}
	return out, nil
}

type classified struct {
	tool, kind string
	confidence float64
	evidence   []string
}

func classifyKey(p paths.Paths, m *manifest.Manifest, profile, name, abs, comment string, inUse, gitKeys, dockerHosts []string, prof manifest.Profile) classified {
	var ev []string
	tool, kind := "", ""
	conf := 0.0
	bump := func(t, k string, c float64, why string) {
		if t == "" {
			return
		}
		ev = append(ev, why)
		if c >= conf {
			tool, kind, conf = t, k, c
		}
	}
	lowerName := strings.ToLower(name + " " + comment)
	if strings.Contains(lowerName, "git-signing") || strings.Contains(lowerName, "git signing") ||
		(strings.Contains(lowerName, "signing") && !strings.Contains(lowerName, "docker")) {
		bump(manifest.ToolGit, manifest.ToolKindGitSigning, 0.80, "name or comment looks like git-signing")
	}
	if strings.Contains(lowerName, "docker-tunnel") || strings.Contains(lowerName, "docker") || strings.Contains(lowerName, "mac-to-devbox") {
		bump(manifest.ToolDocker, manifest.ToolKindDocker, 0.80, "name or comment looks like a docker tunnel")
	}
	for _, h := range prof.Hosts {
		key := ""
		if h.KeyName != nil {
			key = *h.KeyName
		}
		if key != name {
			continue
		}
		if h.Provider != nil && (*h.Provider == "github" || *h.Provider == "gitlab") {
			// git-ssh authentication keys stay at the profile root
			if tool == manifest.ToolGit && kind == manifest.ToolKindGitSigning {
				// keep signing if other signals say signing
			} else {
				bump("", "", conf, "used as a git host key; left at the profile root")
				return classified{evidence: ev} // do not move auth keys
			}
		}
		if h.VM != nil {
			bump(manifest.ToolDocker, manifest.ToolKindDocker, 0.75, "host "+h.Alias+" is a VM")
		}
		for _, alias := range append([]string{h.Alias}, h.Aliases...) {
			for _, dh := range dockerHosts {
				if strings.EqualFold(dh, alias) {
					bump(manifest.ToolDocker, manifest.ToolKindDocker, 0.95, "docker context uses ssh://"+alias)
				}
			}
		}
	}
	for _, g := range gitKeys {
		if sameFile(g, abs) || sameFile(g, abs+".pub") {
			bump(manifest.ToolGit, manifest.ToolKindGitSigning, 1.0, "user.signingkey points here")
		}
	}
	_ = m
	_ = p
	_ = inUse
	if tool == "" {
		return classified{evidence: ev}
	}
	return classified{tool: tool, kind: kind, confidence: conf, evidence: ev}
}

func pubComment(pub string) string {
	b, err := os.ReadFile(pub)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(b))
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[2:], " ")
}

func sameFile(a, b string) bool {
	a, b = expandHomeish(a), expandHomeish(b)
	return a != "" && filepath.Clean(a) == filepath.Clean(b)
}

func expandHomeish(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "~/") {
		// resolved by the caller against the sandbox home when possible
		return v
	}
	return v
}

// signingKeyPaths reads user.signingkey from gitconfigs that belong to p's
// home (the directory that holds .ssh). It never opens a private key.
func signingKeyPaths(p paths.Paths) []string {
	var out []string
	for _, f := range gitconfigFiles(p) {
		out = append(out, gitConfigValues(f, "user.signingkey")...)
	}
	for i, v := range out {
		out[i] = resolveAgainstHome(p, v)
	}
	return out
}

func gitconfigFiles(p paths.Paths) []string {
	var files []string
	if filepath.Base(p.SSHDir) == ".ssh" {
		home := filepath.Dir(p.SSHDir)
		files = append(files, filepath.Join(home, ".gitconfig"))
		files = append(files, filepath.Join(home, ".config", "git", "config"))
	}
	if p.DevRoot != "" {
		files = append(files, filepath.Join(p.DevRoot, ".gitconfig"))
	}
	// sange include files, if present beside the config home
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(p.ConfigDir), "sange", "git", "profile-*.gitconfig")); len(matches) > 0 {
		files = append(files, matches...)
	}
	if matches, _ := filepath.Glob(filepath.Join(p.ConfigDir, "git", "profile-*.gitconfig")); len(matches) > 0 {
		files = append(files, matches...)
	}
	return files
}

func gitConfigValues(path, key string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	want := strings.ToLower(key)
	for _, raw := range strings.Split(string(b), "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, "[") {
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		// git config files use "signingkey = ..." under [user]
		if k == want || strings.HasSuffix(want, "."+k) || k == strings.TrimPrefix(want, "user.") {
			out = append(out, strings.Trim(strings.TrimSpace(v), `"`))
		}
	}
	return out
}

func resolveAgainstHome(p paths.Paths, v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, `"`)
	if strings.HasPrefix(v, "~/") && filepath.Base(p.SSHDir) == ".ssh" {
		return filepath.Join(filepath.Dir(p.SSHDir), v[2:])
	}
	return v
}

func dockerSSHHosts(p paths.Paths) []string {
	if filepath.Base(p.SSHDir) != ".ssh" {
		return nil
	}
	home := filepath.Dir(p.SSHDir)
	matches, _ := filepath.Glob(filepath.Join(home, ".docker", "contexts", "meta", "*", "meta.json"))
	var out []string
	for _, f := range matches {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc struct {
			Endpoints map[string]struct {
				Host string `json:"Host"`
			} `json:"Endpoints"`
		}
		if json.Unmarshal(b, &doc) != nil {
			continue
		}
		if ep, ok := doc.Endpoints["docker"]; ok {
			if host, ok := strings.CutPrefix(ep.Host, "ssh://"); ok {
				host = strings.TrimSuffix(host, "/")
				if h, _, ok := strings.Cut(host, "@"); ok {
					host = h
					// ssh://user@alias — actually user@host; take after @
				}
				if _, after, ok := strings.Cut(ep.Host, "ssh://"); ok {
					host = after
					if _, h, ok := strings.Cut(host, "@"); ok {
						host = h
					}
					if i := strings.IndexAny(host, "/:"); i >= 0 {
						host = host[:i]
					}
					out = append(out, host)
				}
			}
		}
	}
	return out
}

// PurposeLine is the plan line that recategorises profile/key into tools/<tool>/.
func PurposeLine(profile, key, tool, kind string) string {
	line := "purpose " + profile + "/" + key + " tool " + tool
	if kind != "" && kind != tool {
		line += " kind " + kind
	}
	return line
}

// Public comment helper used by tests.
func pubLineComment(line string) string { return strings.TrimSpace(line) }

// FingerprintOfPub is a thin wrapper around keyaudit so tests and apply can
// compare leftover copies without reading a private key.
func fingerprintOfPub(pubPath string) (string, error) {
	b, err := os.ReadFile(pubPath)
	if err != nil {
		return "", err
	}
	return keyaudit.FingerprintOf(string(b))
}
