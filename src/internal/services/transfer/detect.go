package transfer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/util/agepass"
)

// Kind is what an imported file is.
type Kind string

// The kinds Detect tells apart.
const (
	KindFull      Kind = "full"       // an encrypted full export (age, passphrase)
	KindBundle    Kind = "bundle"     // an age file encrypted to a key: a backup bundle
	KindSetup     Kind = "setup"      // a setup document
	KindPack      Kind = "pack"       // an export pack holding setup.json
	KindPackOnly  Kind = "pack-other" // an export pack without setup.json
	KindSSHConfig Kind = "ssh-config" // an ssh config
	KindTermius   Kind = "termius"    // a Termius CSV
	KindUnknown   Kind = "unknown"
)

// MaxImport is the largest file an import reads.
const MaxImport = 16 << 20

// maxMember is the largest setup.json read out of a pack.
const maxMember = 8 << 20

// Detect tells what data is. It reads no secret: an age file is told by its
// header alone.
func Detect(data []byte) Kind {
	switch {
	case agepass.IsPassphrase(data):
		return KindFull
	case agepass.IsAge(data):
		return KindBundle
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		if _, err := SetupFromPack(data); err == nil {
			return KindPack
		}
		return KindPackOnly
	}
	trim := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if bytes.HasPrefix(trim, []byte("{")) {
		var probe struct {
			Format string `json:"format"`
		}
		if json.Unmarshal(trim, &probe) == nil && probe.Format == setupxfer.Format {
			return KindSetup
		}
		return KindUnknown
	}
	first, _, _ := strings.Cut(string(trim), "\n")
	low := strings.ToLower(first)
	if strings.Contains(low, "label") && strings.Contains(low, "hostname") && strings.Contains(first, ",") {
		return KindTermius
	}
	for _, line := range strings.Split(string(trim), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && (strings.EqualFold(f[0], "Host") || strings.EqualFold(f[0], "Match") || strings.EqualFold(f[0], "Include")) {
			return KindSSHConfig
		}
	}
	return KindUnknown
}

// Explain says what to do with a kind an import does not take itself.
func Explain(k Kind, command string) string {
	switch k {
	case KindBundle:
		return "This is a backup bundle, encrypted to a key rather than a passphrase. Restore it with `" + command + " ssh restore`."
	case KindPackOnly:
		return "This export pack has no setup.json, so there is nothing to import from it. Export it again with the setup document ticked."
	case KindSSHConfig:
		return "This is an ssh config. Bring its hosts in from the SSH import page (`" + command + " ssh import`), which reviews each host."
	case KindTermius:
		return "This is a Termius CSV: it is made for Termius and carries no keys, so it is not imported here. Import a setup document, an export pack or a full export."
	}
	return "This file is not a setup document, an export pack or an encrypted full export."
}

// SetupFromPack reads setup.json out of an export pack.
func SetupFromPack(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != "setup.json" {
			continue
		}
		if f.UncompressedSize64 > maxMember {
			return nil, fmt.Errorf("setup.json in the pack is over %d MiB", maxMember>>20)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxMember+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(body) > maxMember {
			return nil, fmt.Errorf("setup.json in the pack is over %d MiB", maxMember>>20)
		}
		return body, nil
	}
	return nil, errors.New("the pack holds no setup.json")
}
