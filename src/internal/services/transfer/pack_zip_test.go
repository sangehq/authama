package transfer

import (
	"archive/zip"
	"bytes"
	"testing"
	"time"
)

// Every file in an export zip is dated when the export was made, owner-only.
func TestZipDatesEveryFileWithTheExportTime(t *testing.T) {
	at := time.Date(2026, 11, 22, 20, 10, 5, 0, time.UTC)
	b, err := Zip([]File{{"setup.json", []byte("{}")}, {"README.txt", []byte("hi")}}, at)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if !f.Modified.Equal(at) {
			t.Errorf("%s is dated %s, not %s", f.Name, f.Modified, at)
		}
		if f.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %v", f.Name, f.Mode())
		}
	}
}
