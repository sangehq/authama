package bundler

import (
	"fmt"
	"os"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/util/homeperms"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// FinishRestore re-asserts perms on the restored tree and config-home secrets,
// then re-renders the config. The bundle does not carry the rendered files, so
// without the render ~/.ssh has no category configs; a failure is returned
// rather than dropped.
func FinishRestore(p paths.Paths) error {
	for _, mp := range perms.IterManagedPathsIn(p.SSHDir, manifest.KeyRootAt(p.Manifest())) {
		_ = perms.SetPerms(mp.Path, mp.Mode)
	}
	for _, sp := range homeperms.SecretPerms(p) {
		if _, e := os.Stat(sp.Path); e == nil {
			_ = perms.SetPerms(sp.Path, sp.Mode)
		}
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return fmt.Errorf("config render skipped: %w", err)
	}
	if _, err := configsvc.New(p.SSHDir, m, platform.EmitUseKeychain()).Write(false); err != nil {
		return fmt.Errorf("config render failed: %w", err)
	}
	return nil
}
