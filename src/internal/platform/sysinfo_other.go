//go:build !darwin && !linux && !windows

package platform

import "runtime"

// fillSystem names the OS only: platforms outside the released set (linux,
// darwin, windows) have no system details wired up.
func fillSystem(si *SystemInfo) { si.OS = runtime.GOOS }
