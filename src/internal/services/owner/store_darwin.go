package owner

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/simtabi/authama/src/internal/util/paths"
)

const securityTool = "/usr/bin/security"

// keychain keeps the key as a generic password in the login keychain. The
// key is written through `security -i`'s standard input, never its arguments,
// so it does not show in the process list.
type keychain struct{ acct string }

func osStore(p paths.Paths) Store {
	if _, err := exec.LookPath(securityTool); err != nil {
		return nil
	}
	return keychain{acct: account(p)}
}

func (keychain) Name() string { return StoreKeychain }

func (k keychain) Load() ([]byte, error) {
	out, err := exec.Command(securityTool, "find-generic-password", "-a", k.acct, "-s", service, "-w").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 { // errSecItemNotFound
			return nil, ErrNoKey
		}
		return nil, err
	}
	return keyFromHex(strings.TrimSpace(string(out)))
}

func (k keychain) Save(key []byte) error {
	if strings.ContainsAny(k.acct, "\"\n") {
		return fmt.Errorf("the config home's path cannot name a keychain item: %q", k.acct)
	}
	cmd := exec.Command(securityTool, "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a \"%s\" -s %s -l \"authama owner key\" -w %s\n",
		k.acct, service, keyHex(key)))
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(errb.String()))
	}
	// security -i reports a failed command on stderr and still exits 0.
	if s := strings.TrimSpace(errb.String()); s != "" {
		return errors.New(s)
	}
	return nil
}

func (k keychain) Remove() error {
	err := exec.Command(securityTool, "delete-generic-password", "-a", k.acct, "-s", service).Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 44 {
		return ErrNoKey
	}
	return err
}
