package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ReservedKey is a key file directly in ~/.ssh that is kept on purpose although
// no host uses it yet - a standby key for a devbox that does not exist, say.
// doctor reports it as information rather than a loose key, and `vm adopt-key`
// and `vm sync` release the reservation when they adopt the key. authama never
// touches the file: the reservation is the manifest's note about it.
type ReservedKey struct {
	// For says what the key is kept for ("devbox", "docker devbox"); free text.
	For string `json:"for,omitempty"`
	// Note is anything else worth remembering about it.
	Note string `json:"note,omitempty"`
	// Fingerprint is the public key's SHA256 fingerprint when it was reserved,
	// so a key replaced under the same name is noticed.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Since is the day it was reserved (YYYY-MM-DD).
	Since string `json:"since,omitempty"`
}

// ReservedKeyNames returns the reserved key names, sorted.
func ReservedKeyNames(m *Manifest) []string {
	out := make([]string, 0, len(m.ReservedKeys))
	for n := range m.ReservedKeys {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Reserve adds or replaces a reservation.
func Reserve(m *Manifest, name string, r ReservedKey) {
	if m.ReservedKeys == nil {
		m.ReservedKeys = map[string]ReservedKey{}
	}
	m.ReservedKeys[name] = r
}

// Unreserve removes a reservation, reporting whether there was one.
func Unreserve(m *Manifest, name string) bool {
	if _, ok := m.ReservedKeys[name]; !ok {
		return false
	}
	delete(m.ReservedKeys, name)
	if len(m.ReservedKeys) == 0 {
		m.ReservedKeys = nil
	}
	return true
}

// marshalReserved appends ,"reserved_keys":{...} in name order; nothing when
// there are none, so a manifest without reservations is byte-for-byte as before.
func marshalReserved(m *Manifest, b *bytes.Buffer) error {
	if len(m.ReservedKeys) == 0 {
		return nil
	}
	b.WriteString(`,"reserved_keys":{`)
	for i, name := range ReservedKeyNames(m) {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(name)
		b.Write(kb)
		b.WriteByte(':')
		rb, err := json.Marshal(m.ReservedKeys[name])
		if err != nil {
			return err
		}
		b.Write(rb)
	}
	b.WriteByte('}')
	return nil
}

// ValidReservedKeyName reports whether name can be a reserved key: a file name
// directly in ~/.ssh, not a .pub and not one of ssh's own files.
func ValidReservedKeyName(name string) error {
	if err := safeSegment("reserved key name", name); err != nil {
		return err
	}
	switch {
	case strings.HasSuffix(name, ".pub"):
		return fmt.Errorf("reserved key %q: name the private key, without .pub", name)
	case name == "config" || name == "authorized_keys" || name == "environment" || name == "rc" ||
		name == DefaultKeyRoot || strings.HasPrefix(name, "known_hosts"):
		return fmt.Errorf("reserved key %q: that is one of ssh's own files, not a key", name)
	}
	return nil
}

func checkReserved(m *Manifest) error {
	for _, name := range ReservedKeyNames(m) {
		if err := ValidReservedKeyName(name); err != nil {
			return err
		}
		r := m.ReservedKeys[name]
		for field, v := range map[string]string{"for": r.For, "note": r.Note, "fingerprint": r.Fingerprint, "since": r.Since} {
			if err := rejectControl("reserved_keys."+name+"."+field, v); err != nil {
				return err
			}
			if len(v) > 200 {
				return fmt.Errorf("reserved_keys.%s.%s: at most 200 characters", name, field)
			}
		}
	}
	return nil
}
