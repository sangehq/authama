package tokens

import (
	"errors"
	"fmt"
	"strings"
)

// Every target's file is shared with things this tool does not manage: other
// registries, credential helpers, proxies, npm settings. So writing merges:
// the hosts in the set are replaced and everything else is kept.

// ErrUnreadableJSON means an existing auth.json or docker config.json is not
// a JSON object. The Python tool replaced such a file with a warning; this
// port refuses, because the file may be a half-written edit that still holds
// credentials and replacing it would lose them.
var ErrUnreadableJSON = errors.New("is not a JSON object; fix or move it, then retry (refusing to replace it)")

// Render merges the set's entries for target into existing, the current file
// content (absent = render fresh). The result has no trailing newline; the
// writer adds one.
func (s *Set) Render(target string, existing string, present bool) (string, error) {
	for _, e := range s.Entries(target) {
		if err := Check(e); err != nil {
			return "", fmt.Errorf("%s:%s: %w", e.Target, e.Host, err)
		}
	}
	switch target {
	case Composer:
		base, err := baseObject(existing, present, "auth.json")
		if err != nil {
			return "", err
		}
		return encodeIndented(s.mergeComposer(base)), nil
	case Docker:
		base, err := baseObject(existing, present, "docker config.json")
		if err != nil {
			return "", err
		}
		return encodeIndented(s.mergeDocker(base)), nil
	case NPM:
		return s.mergeNPMRC(existing), nil
	case Git:
		return s.mergeGit(existing), nil
	case Netrc:
		return s.mergeNetrc(existing), nil
	}
	if f, ok := formats[target]; ok {
		return f.merge(s.Entries(target), existing), nil
	}
	return "", fmt.Errorf("unknown target: %s", target)
}

// baseObject parses an existing JSON auth file. Empty or whitespace-only is a
// fresh start; anything else must be a JSON object.
func baseObject(text string, present bool, label string) (*object, error) {
	if !present || pyStrip(text) == "" {
		return newObject(), nil
	}
	v, err := decodeOrdered(text)
	if err != nil {
		return nil, fmt.Errorf("existing %s %w", label, ErrUnreadableJSON)
	}
	o, ok := v.(*object)
	if !ok {
		return nil, fmt.Errorf("existing %s %w", label, ErrUnreadableJSON)
	}
	return o, nil
}

func (s *Set) mergeComposer(base *object) *object {
	out := base.clone()
	fresh := s.composerDoc()
	for _, section := range fresh.keys {
		hostsAny, _ := fresh.get(section)
		hosts := hostsAny.(*object)
		var merged *object
		if cur, ok := out.get(section); ok {
			if o, ok := cur.(*object); ok {
				merged = o.clone()
			}
		}
		if merged == nil {
			merged = newObject()
		}
		for _, h := range hosts.keys {
			merged.set(h, hosts.vals[h])
		}
		out.set(section, merged)
	}
	return out
}

func (s *Set) mergeDocker(base *object) *object {
	out := base.clone()
	var auths *object
	if cur, ok := out.get("auths"); ok {
		if o, ok := cur.(*object); ok {
			auths = o.clone()
		}
	}
	if auths == nil {
		auths = newObject()
	}
	fresh := s.dockerAuths()
	for _, h := range fresh.keys {
		auths.set(h, fresh.vals[h])
	}
	out.set("auths", auths)
	return out
}

func (s *Set) mergeNPMRC(existing string) string {
	regs := map[string]bool{}
	scopes := map[string]bool{}
	for _, e := range s.Entries(NPM) {
		regs[npmRegistry(e.Host)] = true
		if e.Scope != "" {
			scopes[e.Scope] = true
		}
	}
	var kept []string
	for _, line := range pySplitLines(existing) {
		st := pyStrip(line)
		if st == "" || strings.HasPrefix(st, "#") || strings.HasPrefix(st, ";") {
			kept = append(kept, line)
			continue
		}
		if npmLineForAny(st, regs) {
			continue
		}
		drop := false
		for sc := range scopes {
			if strings.HasPrefix(st, sc+":registry") {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, line)
		}
	}
	return joinStripNL(append(kept, pySplitLines(s.NPMRC())...))
}

func npmLineForAny(st string, regs map[string]bool) bool {
	for r := range regs {
		if strings.HasPrefix(st, r+"/:") || strings.HasPrefix(st, r+":") {
			return true
		}
	}
	return false
}

func (s *Set) mergeGit(existing string) string {
	managed := map[string]bool{}
	for _, e := range s.Entries(Git) {
		managed[GitKey(e.Host)] = true
	}
	var kept []string
	for _, line := range pySplitLines(existing) {
		st := pyStrip(line)
		if st == "" || strings.HasPrefix(st, "#") {
			kept = append(kept, line)
			continue
		}
		if managed[GitKey(st)] {
			continue
		}
		kept = append(kept, line)
	}
	return joinStripNL(append(kept, pySplitLines(s.GitCredentials())...))
}

func (s *Set) mergeNetrc(existing string) string {
	managed := map[string]bool{}
	for _, e := range s.Entries(Netrc) {
		managed[e.Host] = true
	}
	var kept []string
	for _, line := range pySplitLines(existing) {
		toks := pyFields(line)
		if len(toks) >= 2 && toks[0] == "machine" && managed[toks[1]] {
			continue
		}
		kept = append(kept, line)
	}
	return joinStripNL(append(kept, pySplitLines(s.NetrcText())...))
}
