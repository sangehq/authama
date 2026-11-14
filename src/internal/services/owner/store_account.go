//go:build darwin || linux

package owner

import "github.com/sangehq/authama/src/internal/util/paths"

// account is the home a stored key belongs to, so two homes (a pinned
// AUTHAMA_CONFIG_DIR beside the default) keep separate keys.
func account(p paths.Paths) string { return p.ConfigDir }
