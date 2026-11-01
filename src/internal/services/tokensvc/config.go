package tokensvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/fs"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/log"
)

// Config file names and variables.
const (
	ConfigEnv        = "SSH_MANAGER_TOKENS_CONFIG"
	LocalConfigName  = "authama-tokens.json"
	LegacyConfigName = "auth-token-manager.json"
	legacyEnv        = "AUTH_TOKEN_MANAGER_CONFIG"
	legacyEnvOld     = "COMPOSER_AUTH_CONFIG"
)

// ConfigSource is where a token config came from.
type ConfigSource struct {
	Path string
	// Legacy is set for an auth-token-manager location; the caller prints a
	// deprecation notice pointing at `token adopt-legacy`.
	Legacy bool
}

// FindConfig discovers the token config: --config, then
// $SSH_MANAGER_TOKENS_CONFIG, ./authama-tokens.json, <config home>/tokens.json,
// and then, deprecated, auth-token-manager's own locations. It returns a
// zero ConfigSource when there is none.
func (e Env) FindConfig(explicit string) (ConfigSource, error) {
	if explicit != "" {
		p := e.abs(explicit)
		if !isFile(p) {
			return ConfigSource{}, fmt.Errorf("--config file not found: %s", p)
		}
		return ConfigSource{Path: p}, nil
	}
	var cands []ConfigSource
	if v := e.getenv(ConfigEnv); v != "" {
		cands = append(cands, ConfigSource{Path: e.abs(v)})
	}
	cands = append(cands,
		ConfigSource{Path: filepath.Join(e.Cwd, LocalConfigName)},
		ConfigSource{Path: e.Paths.Tokens()})
	cands = append(cands, e.legacyCandidates()...)
	for _, c := range cands {
		if isFile(c.Path) {
			return c, nil
		}
	}
	return ConfigSource{}, nil
}

func (e Env) legacyCandidates() []ConfigSource {
	var out []ConfigSource
	for _, v := range []string{legacyEnv, legacyEnvOld} {
		if p := e.getenv(v); p != "" {
			out = append(out, ConfigSource{Path: e.abs(p), Legacy: true})
		}
	}
	return append(out,
		ConfigSource{Path: filepath.Join(e.Cwd, LegacyConfigName), Legacy: true},
		ConfigSource{Path: filepath.Join(e.Home, ".config", LegacyConfigName), Legacy: true})
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// LoadConfig reads a token config.
func LoadConfig(path string) (*tokens.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config %s: %w", path, err)
	}
	c, err := tokens.ParseConfig(string(b))
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

// Builtin is the config used when none is found: GitHub for Composer, from
// $GH_TOKEN.
func Builtin() *tokens.Config {
	cat, _ := tokens.LoadCatalog()
	c, err := tokens.ParseConfig(cat.DefaultConfig)
	if err != nil {
		return tokens.NewConfig()
	}
	return c
}

// ConfigMode is the mode of a saved token config: it names variables, not
// secrets, so it is readable like any other config and safe to commit.
const ConfigMode os.FileMode = 0o644

// SaveConfig writes a token config atomically.
func (e Env) SaveConfig(path string, c *tokens.Config) error {
	if err := e.Guard(path); err != nil {
		return err
	}
	return fs.WriteTextAtomic(path, c.JSON()+"\n", ConfigMode)
}

// Scaffold writes a starter config to dest, never over an existing file:
// the chosen targets' templates, or the all-targets example.
func (e Env) Scaffold(dest string, targets []string) (string, error) {
	if dest == "" {
		dest = e.Paths.Tokens()
	}
	dest = e.abs(dest)
	if exists(dest) {
		return "", fmt.Errorf("%s already exists (delete it or pick another path)", dest)
	}
	if err := e.Guard(dest); err != nil {
		return "", err
	}
	if len(targets) > 0 {
		c, err := tokens.StarterConfig(targets)
		if err != nil {
			return "", err
		}
		if err := e.SaveConfig(dest, c); err != nil {
			return "", err
		}
		return fmt.Sprintf("wrote %s config -> %s", strings.Join(targets, "+"), dest), nil
	}
	if err := fs.WriteTextAtomic(dest, tokens.AllTargetsTemplate(), ConfigMode); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote config from all-targets.example.json -> %s", dest), nil
}

// ErrNoLegacyConfig means there is no auth-token-manager config to adopt.
var ErrNoLegacyConfig = errors.New("no auth-token-manager config found")

// AdoptLegacy copies the first auth-token-manager config it finds to
// <config home>/tokens.json, so the deprecated locations stop being read.
// The old file is left in place; it is the user's to delete.
func (e Env) AdoptLegacy() (from, to string, err error) {
	to = e.Paths.Tokens()
	var src string
	for _, c := range e.legacyCandidates() {
		if isFile(c.Path) {
			src = c.Path
			break
		}
	}
	if src == "" {
		return "", to, ErrNoLegacyConfig
	}
	if exists(to) {
		return src, to, fmt.Errorf("%s already exists; merge %s into it by hand or move it aside", to, src)
	}
	c, err := LoadConfig(src)
	if err != nil {
		return src, to, err
	}
	if err := e.SaveConfig(to, c); err != nil {
		return src, to, err
	}
	e.audit("token.adopt-legacy", log.Field{Key: "from", Value: src}, log.Field{Key: "to", Value: to})
	return src, to, nil
}

// LoadEnvFile reads VAR=value lines into the lookup, without overriding
// variables that are already set, as --source did.
func (e *Env) LoadEnvFile(path string) error {
	b, err := os.ReadFile(e.abs(path))
	if err != nil {
		return fmt.Errorf("--source file not found: %s", e.abs(path))
	}
	extra := map[string]string{}
	for _, kv := range tokens.ParseEnvFile(string(b)) {
		if _, set := extra[kv[0]]; !set {
			extra[kv[0]] = kv[1]
		}
	}
	prev := e.lookup
	e.lookup = func(k string) (string, bool) {
		if v, ok := prev(k); ok {
			return v, true
		}
		v, ok := extra[k]
		return v, ok
	}
	return nil
}
