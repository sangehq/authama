package tokensvc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/simtabi/ssh-manager/src/v3/internal/util/clipboard"
)

// CopyToClipboard copies a secret through the platform's clipboard tool, so
// it can be pasted without ever being printed.
var CopyToClipboard = func(secret string) error {
	name, args, err := clipboard.Command()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(secret)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
