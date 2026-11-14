package keyaudit

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// The states a reserved key can be in. Only ReservedKept is the expected one;
// the others are warnings, because the note no longer describes the file.
const (
	ReservedKept    = "kept"
	ReservedMissing = "missing"
	ReservedChanged = "changed"
	ReservedAdopted = "adopted"
)

// Reserved is one key kept on purpose: reserved in the manifest
// (`authama ssh key reserve`) or named as the key of a devbox in authama.json
// that has no VM yet. It is never a loose-key finding.
type Reserved struct {
	Name        string `json:"name"`
	For         string `json:"for,omitempty"`
	Note        string `json:"note,omitempty"`
	Since       string `json:"since,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	// Source is "manifest" or "devbox <alias>" (authama.json).
	Source string `json:"source"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

// Problem reports whether the reservation needs attention.
func (r Reserved) Problem() bool { return r.State != ReservedKept }

// Line renders a reservation for doctor and `key list`.
func (r Reserved) Line() string {
	line := r.Name + " - reserved"
	if r.For != "" {
		line += " for " + r.For
	}
	if r.Note != "" {
		line += ": " + r.Note
	}
	if r.Source != "" && r.Source != "manifest" {
		line += "  (authama.json)"
	}
	if r.Problem() {
		line += "  [" + r.State + ": " + r.Detail + "]"
	}
	return line
}

// WithPlanned adds reservations that come from outside the manifest: the keys
// of devboxes declared in authama.json without a VM yet. The map's For is
// expected to name the devbox ("devbox docker").
func (s *Service) WithPlanned(planned map[string]manifest.ReservedKey) *Service {
	s.planned = planned
	return s
}

// Reservations merges the manifest's reserved keys with planned ones; the
// manifest's entry wins, borrowing For from the plan when it has none. The
// second map gives each name's source.
func Reservations(m *manifest.Manifest, planned map[string]manifest.ReservedKey) (map[string]manifest.ReservedKey, map[string]string) {
	out := map[string]manifest.ReservedKey{}
	src := map[string]string{}
	for name, r := range planned {
		out[name] = r
		src[name] = strings.TrimSpace("devbox " + strings.TrimPrefix(r.For, "devbox "))
	}
	if m != nil {
		for name, r := range m.ReservedKeys {
			if r.For == "" {
				r.For = planned[name].For
			}
			out[name] = r
			src[name] = "manifest"
		}
	}
	return out, src
}

// reserved checks every reservation against ~/.ssh.
func (s *Service) reserved() []Reserved {
	res, src := Reservations(s.m, s.planned)
	if len(res) == 0 {
		return nil
	}
	loose := map[string]LooseKey{}
	for _, k := range LooseKeysIn(s.sshDir, manifest.KeyRoot(s.m)) {
		loose[k.Name] = k
	}
	out := make([]Reserved, 0, len(res))
	for name, r := range res {
		e := Reserved{Name: name, For: r.For, Note: r.Note, Since: r.Since, Fingerprint: r.Fingerprint,
			Source: src[name], State: ReservedKept}
		unreserve := "authama ssh key unreserve " + name
		if src[name] != "manifest" {
			unreserve = "remove the devbox's key from authama.json, or give the devbox its vm and run `authama ssh vm sync`"
		}
		_, privErr := os.Lstat(filepath.Join(s.sshDir, name))
		fp, fpErr := PublicFingerprint(s.sshDir, name)
		switch {
		case privErr != nil && fpErr != nil:
			e.State, e.Detail, e.Fix = ReservedMissing, "~/.ssh/"+name+" is gone", unreserve
		case loose[name].CopyOf != "":
			e.State = ReservedAdopted
			e.Detail = "authama now manages it as ~/.ssh/" + loose[name].CopyOf
			e.Fix = unreserve + "   (the reservation has done its job)"
		case r.Fingerprint != "" && fpErr == nil && fp != r.Fingerprint:
			e.State, e.Detail = ReservedChanged, "~/.ssh/"+name+".pub is not the key that was reserved ("+fp+")"
			e.Fix = "authama ssh key reserve " + name + "   (to keep the new key), or " + unreserve
		case privErr != nil:
			e.State, e.Detail = ReservedMissing, "the private key ~/.ssh/"+name+" is gone"
			e.Fix = unreserve
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// MarkReserved labels the loose keys that are reserved, for pickers and pages.
func MarkReserved(keys []LooseKey, res map[string]manifest.ReservedKey) []LooseKey {
	for i := range keys {
		if r, ok := res[keys[i].Name]; ok {
			keys[i].Reserved = true
			keys[i].ReservedFor = r.For
			keys[i].ReservedNote = r.Note
		}
	}
	return keys
}

// PublicFingerprint is the OpenSSH SHA256 fingerprint of ~/.ssh/<name>.pub,
// computed from the public half only (what `ssh-keygen -lf` prints).
func PublicFingerprint(sshDir, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(sshDir, name+".pub"))
	if err != nil {
		return "", err
	}
	return FingerprintOf(string(b))
}

// FingerprintOf fingerprints one authorized_keys-style public key line.
func FingerprintOf(line string) (string, error) {
	f := strings.Fields(strings.TrimSpace(line))
	if len(f) < 2 {
		return "", errors.New("not a public key line")
	}
	blob, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil {
		return "", errors.New("not a public key line")
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

// ReservedLines renders the reservations as doctor's info block.
func (r Report) ReservedLines() []string {
	if len(r.Reserved) == 0 {
		return nil
	}
	lines := []string{"reserved keys (kept on purpose, not dangling):"}
	for _, e := range r.Reserved {
		lines = append(lines, "  "+e.Line())
		if e.Problem() && e.Fix != "" {
			lines = append(lines, "    -> "+e.Fix)
		}
	}
	return lines
}

// ReservedProblems are the reservations that need attention.
func (r Report) ReservedProblems() []Reserved {
	var out []Reserved
	for _, e := range r.Reserved {
		if e.Problem() {
			out = append(out, e)
		}
	}
	return out
}
