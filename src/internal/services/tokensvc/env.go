// Package tokensvc does the I/O for token management: it finds the token
// config, reads and writes the five target files (Composer's auth.json,
// .npmrc, Docker's config.json, the git credential store and .netrc), and
// keeps every write inside the sandbox in dev mode. The formats themselves
// live in internal/core/tokens.
//
// What it records is metadata only: the audit log and history get the target,
// the host and the file, never a secret.
package tokensvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/simtabi/authama/src/v3/internal/core/tokens"
	"github.com/simtabi/authama/src/v3/internal/platform"
	"github.com/simtabi/authama/src/v3/internal/util/paths"
)

// Env is where one invocation's token work happens.
type Env struct {
	Paths paths.Paths
	// Home is the directory the targets resolve under: the real home, or the
	// sandbox's home in dev mode.
	Home string
	// Cwd anchors ./authama-tokens.json and the .gitignore check.
	Cwd     string
	Windows bool
	MacOS   bool
	getenv  func(string) string
	lookup  tokens.Lookup
	// Catalog says where the token catalog came from when a variable
	// replaced the embedded one; see CatalogEnv.
	Catalog CatalogSource
}

// NewEnv builds the token environment for a resolved layout. getenv and
// lookup read the process environment when nil.
//
// In dev mode the variables that relocate a target file (COMPOSER_HOME,
// DOCKER_CONFIG, NETRC, ...) are hidden, so a sandboxed run resolves every
// target under the sandbox no matter what the shell exports. Secret
// variables are still read: a sandbox needs real values to render.
func NewEnv(p paths.Paths, getenv func(string) string, lookup tokens.Lookup, cwd string) Env {
	if getenv == nil {
		getenv = os.Getenv
	}
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	e := Env{
		Paths: p, Home: p.TokenHome(paths.Getenv(getenv)), Cwd: cwd,
		Windows: platform.IsWindows(), MacOS: platform.IsMacOS(),
		getenv: getenv, lookup: lookup,
	}
	if p.IsDev() {
		hidden := map[string]bool{}
		for _, v := range tokens.PathOverrideVars {
			hidden[v] = true
		}
		e.getenv = func(k string) string {
			if hidden[k] {
				return ""
			}
			return getenv(k)
		}
	}
	e.Catalog = useCatalog(e)
	return e
}

// Getenv reads a variable as the targets see it (hidden ones are "").
func (e Env) Getenv(k string) string { return e.getenv(k) }

// Lookup reads a secret variable.
func (e Env) Lookup(k string) (string, bool) { return e.lookup(k) }

// Host is the core view of where the targets live.
func (e Env) Host() tokens.Host {
	return tokens.Host{
		Home: e.Home, Getenv: e.getenv, Windows: e.Windows, MacOS: e.MacOS,
		Exists: exists,
		IsDir: func(p string) bool {
			fi, err := os.Stat(p)
			return err == nil && fi.IsDir()
		},
	}
}

// TargetPath is the file a target reads, optionally scoped to a project.
func (e Env) TargetPath(target, project string) string {
	return e.Host().TargetPath(target, e.abs(project))
}

func (e Env) abs(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if p == "~" || len(p) > 1 && p[0] == '~' && (p[1] == '/' || p[1] == '\\') {
		return filepath.Join(e.Home, p[1:])
	}
	return filepath.Join(e.Cwd, p)
}

// ErrOutsideSandbox is returned for a dev-mode write that would land outside
// the sandbox root.
var ErrOutsideSandbox = errors.New("is outside the dev-mode sandbox")

// Guard refuses, in dev mode, a write to a path outside the sandbox or to a
// symlink (which could point anywhere).
func (e Env) Guard(path string) error {
	if !e.Paths.IsDev() {
		return nil
	}
	if !e.Paths.Contains(path) {
		return fmt.Errorf("%s %w %s; nothing was written", path, ErrOutsideSandbox, e.Paths.DevRoot)
	}
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink; dev mode does not write through links", path)
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
