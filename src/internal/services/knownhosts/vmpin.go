package knownhosts

import (
	"os"
	"strings"
)

// HostToken is the name ssh looks a host up by in known_hosts: hostname, or
// [hostname]:port off port 22.
func HostToken(hostname string, port int) string { return hostToken(hostname, port) }

// Pin states for a host checked against freshly scanned keys.
const (
	PinNone    = "unpinned"
	PinMatch   = "pinned"
	PinChanged = "changed"
)

// PinState compares the trust store's entries for token with scanned keys:
// unpinned (no entry names token), pinned (a scanned key is already trusted)
// or changed (entries name token, but none holds a key the host offers now).
func (s *Service) PinState(token string, scanned []ScannedKey) string {
	b, err := os.ReadFile(s.Path())
	if err != nil {
		return PinNone
	}
	lines := splitNonEmptyTrailing(string(b))
	var named []khLine
	for _, raw := range lines {
		p, ok := parseKHLine(strings.TrimSpace(raw))
		if !ok || (p.marker != "" && p.marker != "@cert-authority") {
			continue
		}
		if hostFieldMatches(p.field, token) {
			named = append(named, p)
		}
	}
	if len(named) == 0 {
		return PinNone
	}
	for _, sk := range scanned {
		f := strings.Fields(sk.Line)
		if len(f) >= 3 && isPinned(named, token, f[1], f[2]) {
			return PinMatch
		}
	}
	return PinChanged
}

// PinAs pins scanned keys under token rather than the address they were
// scanned at - a VM host is reached through a ProxyCommand, so ssh checks the
// key under the alias, while the scan had to go to the hypervisor's address.
func (s *Service) PinAs(token string, scanned []ScannedKey) (int, error) {
	var lines []string
	for _, sk := range scanned {
		f := strings.Fields(sk.Line)
		if len(f) < 3 {
			continue
		}
		lines = append(lines, token+" "+f[1]+" "+f[2])
	}
	if len(lines) == 0 {
		return 0, nil
	}
	return s.Add(lines)
}

// UnpinTagged removes the authama-tagged entries naming token and reports how
// many untagged ones (the user's own) also name it; those are never touched.
func (s *Service) UnpinTagged(token string) (removed, untagged int, err error) {
	b, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	lines := splitNonEmptyTrailing(string(b))
	kept := make([]string, 0, len(lines))
	for _, raw := range lines {
		p, ok := parseKHLine(strings.TrimSpace(raw))
		if ok && hostFieldMatches(p.field, token) {
			if p.tagged() {
				removed++
				continue
			}
			untagged++
		}
		kept = append(kept, raw)
	}
	if removed == 0 {
		return 0, untagged, nil
	}
	return removed, untagged, s.rewrite(s.Path(), kept)
}
