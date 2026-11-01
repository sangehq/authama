package tokens

import (
	"path/filepath"
	"strings"
)

// Host describes the machine the target files live on. The services layer
// fills it from the real OS, or from a sandbox in dev mode; this package never
// reads the environment or the filesystem itself.
type Host struct {
	Home    string
	Getenv  func(string) string
	Windows bool
	MacOS   bool
	Exists  func(string) bool // any file or directory
	IsDir   func(string) bool
}

func (h Host) env(k string) string {
	if h.Getenv == nil {
		return ""
	}
	return h.Getenv(k)
}

// expandUser is Path(p).expanduser(): a leading "~" becomes the home, and the
// result is cleaned the way pathlib normalises it, so on Windows an override
// written with forward slashes (NPM_CONFIG_USERCONFIG=C:/x/.npmrc) uses the
// native separator like every other path this package returns.
func (h Host) expandUser(p string) string {
	if p == "~" {
		return h.Home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(h.Home, p[2:])
	}
	return filepath.Clean(p)
}

// PathOverrideVars are the variables the native tools read to relocate their
// files. In dev mode the services layer hides them, so a sandboxed run can
// never be redirected onto a real file.
var PathOverrideVars = []string{"COMPOSER_HOME", "NPM_CONFIG_USERCONFIG", "DOCKER_CONFIG", "NETRC", "XDG_CONFIG_HOME", "APPDATA"}

// TargetPath is where a target's file lives, found the way the native tool
// finds it, honouring its own env overrides. project scopes composer and npm
// to a project directory; docker, git and netrc are always per-user.
func (h Host) TargetPath(target, project string) string {
	switch target {
	case Composer:
		base := project
		if base == "" {
			base = h.ComposerHome()
		}
		return filepath.Join(base, "auth.json")
	case NPM:
		if project != "" {
			return filepath.Join(project, ".npmrc")
		}
		if uc := h.env("NPM_CONFIG_USERCONFIG"); uc != "" {
			return h.expandUser(uc)
		}
		return filepath.Join(h.Home, ".npmrc")
	case Docker:
		dir := filepath.Join(h.Home, ".docker")
		if c := h.env("DOCKER_CONFIG"); c != "" {
			dir = h.expandUser(c)
		}
		return filepath.Join(dir, "config.json")
	case Git:
		primary := filepath.Join(h.Home, ".git-credentials")
		base := filepath.Join(h.Home, ".config")
		if x := h.env("XDG_CONFIG_HOME"); x != "" {
			base = h.expandUser(x)
		}
		xdg := filepath.Join(base, "git", "credentials")
		if h.Exists != nil && !h.Exists(primary) && h.Exists(xdg) {
			return xdg
		}
		return primary
	case Netrc:
		if v := h.env("NETRC"); v != "" {
			return h.expandUser(v)
		}
		if h.Windows {
			return filepath.Join(h.Home, "_netrc")
		}
		return filepath.Join(h.Home, ".netrc")
	}
	return ""
}

// ComposerHome resolves Composer's home the way Composer does.
func (h Host) ComposerHome() string {
	if v := h.env("COMPOSER_HOME"); v != "" {
		return h.expandUser(v)
	}
	if h.Windows {
		appdata := h.env("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(h.Home, "AppData", "Roaming")
		}
		return filepath.Join(appdata, "Composer")
	}
	legacy := filepath.Join(h.Home, ".composer")
	if h.IsDir != nil && h.IsDir(legacy) {
		return legacy
	}
	if x := h.env("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "composer")
	}
	if h.MacOS {
		return legacy
	}
	return filepath.Join(h.Home, ".config", "composer")
}
