package owner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Store keeps the sealing key for the user.
type Store interface {
	Name() string
	Load() ([]byte, error)
	Save(key []byte) error
	Remove() error
}

// ErrNoKey is a store that holds no key for this home.
var ErrNoKey = errors.New("no sealing key is stored")

// StoreEnv chooses the store: "file" forces the owner-only file, for a
// machine with no keyring session (a server over ssh, a container).
const StoreEnv = "AUTHAMA_OWNER_STORE"

// Store names.
const (
	StoreFile     = "file"
	StoreKeychain = "keychain"
	StoreSecret   = "secret-service"
	StoreDPAPI    = "dpapi"
)

// StoreFor picks the store for a new key: the OS's own where it has one this
// process can reach, else the file. A --dev-root sandbox always uses the
// file inside the sandbox, so a test never writes to the real keychain.
func StoreFor(p paths.Paths) Store {
	if p.IsDev() || strings.EqualFold(os.Getenv(StoreEnv), StoreFile) {
		return fileStore{p}
	}
	if s := osStore(p); s != nil {
		return s
	}
	return fileStore{p}
}

// storeNamed is the store a key was saved in, by the name owner.json records.
func storeNamed(p paths.Paths, name string) Store {
	if name != StoreFile && !p.IsDev() {
		if s := osStore(p); s != nil && s.Name() == name {
			return s
		}
	}
	return fileStore{p}
}

// fileStore is an owner-only file in the state home: the fallback where the
// OS offers no store, and the one Windows wraps in DPAPI.
type fileStore struct{ p paths.Paths }

func (f fileStore) path() string { return filepath.Join(f.p.StateDir(), "owner.key") }
func (fileStore) Name() string   { return StoreFile }

func (f fileStore) Load() ([]byte, error) {
	b, err := os.ReadFile(f.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	return keyFromHex(strings.TrimSpace(string(b)))
}

func (f fileStore) Save(key []byte) error {
	if err := fs.EnsureDir(f.p.StateDir(), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(f.path(), keyHex(key)+"\n", 0o600)
}

func (f fileStore) Remove() error {
	err := os.Remove(f.path())
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoKey
	}
	return err
}
