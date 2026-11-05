// Package seal detects authama's state being moved or edited outside authama.
//
// A seal is an HMAC-SHA256, keyed by a secret the OS keeps for the user (see
// services/owner), over each state file's name and content and over the
// binding: who the user is and which machine this is. authama reseals after
// each operation it performs, so when a file no longer matches its seal,
// something other than authama changed it; when the binding no longer
// matches, the home was copied to another account or machine.
//
// This is detection, not prevention. Anyone who can run as the user can read
// and write the user's files, and can run authama to reseal them. What a seal
// adds is that a change made behind authama's back - by another program, a
// sync tool, a restored backup, or a person editing the file - is noticed and
// named the next time authama runs, instead of being acted on silently.
//
// The package is pure: the caller reads the files and supplies the key.
package seal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

// Version is the seal record format.
const Version = 1

// Binding identifies the account and machine a home belongs to.
type Binding struct {
	User    string // the OS user name
	UID     string // the numeric id, or the Windows SID
	Machine string // the OS's machine id
}

// ID is the binding as one opaque value: a hash, so the record does not carry
// the machine id in clear.
func (b Binding) ID() string {
	h := sha256.Sum256([]byte("authama-binding-v1\x00" + b.User + "\x00" + b.UID + "\x00" + b.Machine))
	return hex.EncodeToString(h[:])
}

// Record is what is stored: one MAC per sealed file, and one over the record.
type Record struct {
	Version int               `json:"version"`
	Binding string            `json:"binding"`
	Sealed  time.Time         `json:"sealed"`
	Files   map[string]string `json:"files"` // name -> MAC; "" means the file was absent
	MAC     string            `json:"mac"`
}

// Status is one file's state against its seal.
type Status string

const (
	OK       Status = "ok"
	Changed  Status = "changed"  // present before and now, different content
	Missing  Status = "missing"  // present when sealed, gone now
	Appeared Status = "appeared" // absent when sealed, present now
	Unsealed Status = "unsealed" // not in the record at all
)

// Finding is one file whose state is not OK.
type Finding struct {
	Name   string
	Status Status
}

// Report is the outcome of a verification.
type Report struct {
	// Authentic is false when the record itself fails its MAC: the seal file
	// was edited, or the key is not the one that made it. Nothing else in the
	// report can be trusted then, and Findings is empty.
	Authentic bool
	// Bound is false when the record was made for another account or machine.
	Bound    bool
	Findings []Finding
}

// Clean reports a record that is authentic, bound here, and matches every file.
func (r Report) Clean() bool { return r.Authentic && r.Bound && len(r.Findings) == 0 }

// ErrNoKey is a seal or verify asked for without a key.
var ErrNoKey = errors.New("no sealing key")

// fileMAC binds a file's name, presence and content to the key and binding.
func fileMAC(key []byte, binding, name string, content []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("authama-seal-file-v1\x00" + binding + "\x00" + name + "\x00"))
	sum := sha256.Sum256(content)
	m.Write(sum[:])
	return hex.EncodeToString(m.Sum(nil))
}

// recordMAC covers everything else in the record, so an entry cannot be
// dropped, added, or copied from another record.
func recordMAC(key []byte, r Record) string {
	names := make([]string, 0, len(r.Files))
	for n := range r.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("authama-seal-record-v1\x00")
	b.WriteString(r.Binding)
	b.WriteString("\x00")
	b.WriteString(r.Sealed.UTC().Format(time.RFC3339Nano))
	for _, n := range names {
		b.WriteString("\x00" + n + "=" + r.Files[n])
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(b.String()))
	return hex.EncodeToString(m.Sum(nil))
}

// Make seals files: name -> content, with a nil content for a file that does
// not exist (so its appearing later is noticed too).
func Make(key []byte, b Binding, files map[string][]byte, now time.Time) (Record, error) {
	if len(key) == 0 {
		return Record{}, ErrNoKey
	}
	id := b.ID()
	r := Record{Version: Version, Binding: id, Sealed: now.UTC(), Files: map[string]string{}}
	for name, content := range files {
		if content == nil {
			r.Files[name] = ""
			continue
		}
		r.Files[name] = fileMAC(key, id, name, content)
	}
	r.MAC = recordMAC(key, r)
	return r, nil
}

// Verify checks files against r. A name in files but not in r is Unsealed; a
// name in r but not in files is not checked (the caller decides the set).
func Verify(key []byte, b Binding, r Record, files map[string][]byte) Report {
	if len(key) == 0 || r.Version != Version || !hmac.Equal([]byte(recordMAC(key, r)), []byte(r.MAC)) {
		return Report{}
	}
	rep := Report{Authentic: true, Bound: r.Binding == b.ID()}
	if !rep.Bound {
		return rep
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		content := files[name]
		want, sealed := r.Files[name]
		var st Status
		switch {
		case !sealed:
			st = Unsealed
		case want == "" && content == nil:
			st = OK
		case want == "":
			st = Appeared
		case content == nil:
			st = Missing
		case hmac.Equal([]byte(fileMAC(key, r.Binding, name, content)), []byte(want)):
			st = OK
		default:
			st = Changed
		}
		if st != OK {
			rep.Findings = append(rep.Findings, Finding{Name: name, Status: st})
		}
	}
	return rep
}
