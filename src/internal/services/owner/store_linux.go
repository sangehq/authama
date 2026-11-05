package owner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/simtabi/authama/src/internal/util/paths"
)

// secretService keeps the key in the desktop keyring (GNOME Keyring, KWallet)
// through secret-tool, which reads the secret on its standard input.
type secretService struct{ acct string }

func osStore(p paths.Paths) Store {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return nil
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil
	}
	return secretService{acct: account(p)}
}

func (secretService) Name() string { return StoreSecret }

func (s secretService) attrs() []string { return []string{"service", service, "account", s.acct} }

func (s secretService) Load() ([]byte, error) {
	out, err := exec.Command("secret-tool", append([]string{"lookup"}, s.attrs()...)...).Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		// secret-tool exits 1 for "no such item" and for a locked or absent
		// keyring alike; either way there is no key to use.
		return nil, ErrNoKey
	}
	return keyFromHex(strings.TrimSpace(string(out)))
}

func (s secretService) Save(key []byte) error {
	cmd := exec.Command("secret-tool", append([]string{"store", "--label=authama owner key"}, s.attrs()...)...)
	cmd.Stdin = strings.NewReader(keyHex(key))
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	return nil
}

func (s secretService) Remove() error {
	return exec.Command("secret-tool", append([]string{"clear"}, s.attrs()...)...).Run()
}
