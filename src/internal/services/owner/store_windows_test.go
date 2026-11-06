package owner

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

// The DPAPI store round-trips a key for this account, and what it writes is
// not the key: DPAPI encrypted it.
func TestDPAPIKeepsTheKeyEncrypted(t *testing.T) {
	root := t.TempDir()
	d := dpapi{paths.Paths{ConfigDir: filepath.Join(root, "cfg")}}
	if _, err := d.Load(); !errors.Is(err, ErrNoKey) {
		t.Fatalf("Load before Save: %v", err)
	}
	key := []byte(testcred.Secret("dpapi"))[:32]
	if err := d.Save(key); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(d.path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), keyHex(key)) {
		t.Fatal("the file holds the key in clear")
	}
	got, err := d.Load()
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("Load = %x, %v", got, err)
	}
	if err := d.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Load(); !errors.Is(err, ErrNoKey) {
		t.Fatalf("Load after Remove: %v", err)
	}
}
