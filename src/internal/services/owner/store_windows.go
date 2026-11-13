package owner

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// dpapi keeps the key in a file in the state home, encrypted with DPAPI for
// the current user: only this Windows account, on this machine, can decrypt
// it, which is the binding Windows itself enforces.
type dpapi struct{ p paths.Paths }

func osStore(p paths.Paths) Store { return dpapi{p} }

func (dpapi) Name() string       { return StoreDPAPI }
func (d dpapi) path() string     { return filepath.Join(d.p.StateDir(), "owner.key.dpapi") }
func entropy() *windows.DataBlob { return blob([]byte(service)) }

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func (b2 *dpapiOut) bytes() []byte {
	return append([]byte(nil), unsafe.Slice(b2.Data, b2.Size)...)
}

type dpapiOut struct{ windows.DataBlob }

func (d dpapi) Load() ([]byte, error) {
	enc, err := os.ReadFile(d.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	var out dpapiOut
	if err := windows.CryptUnprotectData(blob(enc), nil, entropy(), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out.DataBlob); err != nil {
		return nil, err
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	return keyFromHex(string(out.bytes()))
}

func (d dpapi) Save(key []byte) error {
	var out dpapiOut
	if err := windows.CryptProtectData(blob([]byte(keyHex(key))), nil, entropy(), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out.DataBlob); err != nil {
		return err
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) }()
	if err := fs.EnsureDir(d.p.StateDir(), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(d.path(), string(out.bytes()), 0o600)
}

func (d dpapi) Remove() error {
	err := os.Remove(d.path())
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoKey
	}
	return err
}
