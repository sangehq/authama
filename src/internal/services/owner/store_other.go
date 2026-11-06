//go:build !linux && !darwin && !windows

package owner

import "github.com/simtabi/authama/src/internal/util/paths"

// osStore: no OS key store authama drives on this platform; the file is used.
func osStore(paths.Paths) Store { return nil }
