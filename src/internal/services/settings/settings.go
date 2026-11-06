// Package settings reads the user's two settings files - <config home>/.env and
// <config home>/authama.json - into the process environment at start-up.
//
// Every setting authama has is an AUTHAMA_* environment variable (see
// envnames). The files are two more ways to supply them, and the order is:
//
//	command-line flags > the real environment > .env > authama.json > defaults
//
// That order is kept by construction: a file only ever sets a variable the
// environment does not already have (under either its current or legacy
// name), .env is applied before authama.json, and flags are read by each
// command ahead of the variable. The tool's own state - the history database,
// logs, snapshots, bundles, the lock - is not configuration and lives apart,
// in the state home (see paths).
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/simtabi/authama/src/internal/util/envnames"
)

// SchemaURL is where the published JSON Schema for authama.json lives.
const SchemaURL = "https://raw.githubusercontent.com/simtabi/authama/main/docs/schema/authama.schema.json"

// Version is the authama.json format version this build reads.
const Version = 1

// Kind is a setting's value type.
type Kind string

const (
	KindString Kind = "string"
	KindPath   Kind = "path"
	KindInt    Kind = "integer"
	KindBool   Kind = "boolean"
	KindEnum   Kind = "enum"
)

// Spec is one user setting: its place in authama.json and the variable it
// supplies.
type Spec struct {
	Section, Name string
	Env           string
	Kind          Kind
	Enum          []string
	Min           int
	Default       string
	Doc           string
}

// Key is the dotted authama.json path, e.g. "backup.age_recipient".
func (s Spec) Key() string { return s.Section + "." + s.Name }

// Specs is every setting authama.json can hold, in documentation order. The
// published schema is generated from it (TestSchemaIsCurrent).
var Specs = []Spec{
	{Section: "paths", Name: "state_dir", Env: envnames.StateDir, Kind: KindPath,
		Default: "$XDG_STATE_HOME/authama or ~/.local/state/authama",
		Doc:     "Where authama keeps its own state: the history database and audit log, snapshots, bundles, the lock and caches."},
	{Section: "paths", Name: "age_identity_file", Env: envnames.AgeIdentityFile, Kind: KindPath,
		Doc: "The age identity (private key) `ssh restore` decrypts bundles with."},
	{Section: "paths", Name: "tokens_config", Env: envnames.TokensConfig, Kind: KindPath,
		Default: "./authama-tokens.json, else <config home>/tokens.json",
		Doc:     "The token config: which environment variables build which registry credential."},
	{Section: "paths", Name: "tokens_catalog", Env: envnames.TokensCatalog, Kind: KindPath,
		Default: "the built-in catalog",
		Doc:     "A catalog.json to use instead of the built-in token services and providers."},
	{Section: "backup", Name: "age_recipient", Env: envnames.AgeRecipient, Kind: KindString,
		Doc: "The age recipient (public key) bundles are encrypted to. Required for `ssh bundle` and the backups destructive commands take first."},
	{Section: "backup", Name: "snapshot_retain", Env: envnames.SnapshotRetain, Kind: KindInt, Min: 1, Default: "10",
		Doc: "How many ~/.ssh snapshots to keep."},
	{Section: "backup", Name: "old_key_max_age_days", Env: envnames.OldKeyMaxAgeDays, Kind: KindInt, Min: 0, Default: "90",
		Doc: "Days an archived predecessor key may stay before doctor flags it; 0 turns the check off."},
	{Section: "ssh", Name: "auto_pin", Env: envnames.AutoPin, Kind: KindBool, Default: "true",
		Doc: "Pin a new host's key on first use (trust on first use) when a profile is minted."},
	{Section: "git", Name: "mode", Env: envnames.GitMode, Kind: KindEnum, Enum: []string{"profile", "ssh", "https"},
		Default: "the manifest's defaults.git.mode, else profile",
		Doc:     "How `ssh git` rewrites remotes."},
	{Section: "tokens", Name: "docker_creds_store", Env: envnames.DockerCredsStore, Kind: KindEnum, Enum: []string{"login", "refuse"},
		Default: "ask at a terminal, else refuse",
		Doc:     "What to do with registries Docker keeps in a credential store: store them with `docker login` or refuse and warn."},
	{Section: "history", Name: "enabled", Env: envnames.History, Kind: KindBool, Default: "true",
		Doc: "Record every change in the history database (`authama history`)."},
	{Section: "notifications", Name: "desktop", Env: envnames.Notifications, Kind: KindBool, Default: "true",
		Doc: "Show desktop notifications for keys due for rotation and dangling keys."},
	{Section: "vm", Name: "hypervisors", Env: envnames.VMHypervisors, Kind: KindString,
		Default: "every built-in driver and every custom one in the hypervisors section",
		Doc:     "The hypervisors you have and use, comma-separated (e.g. \"parallels,vmpal\"). `ssh vm list`, `ssh vm sync`, doctor and the web UI query only these; a VM host on another hypervisor is reported, never changed."},
	{Section: "vm", Name: "prune_gone", Env: envnames.VMPruneGone, Kind: KindBool, Default: "true",
		Doc: "Let `ssh vm sync` remove the host entry (never its key files) and Docker context of a VM its hypervisor no longer has. Off: sync only reports it."},
	{Section: "vm", Name: "repin_changed", Env: envnames.VMRepin, Kind: KindBool, Default: "false",
		Doc: "Let `ssh vm sync` replace a VM host's known_hosts pin when the guest's host key changed (a rebuilt VM), as `--repin` does. Off: sync reports it."},
	{Section: "web", Name: "addr", Env: envnames.WebAddr, Kind: KindString, Default: "127.0.0.1",
		Doc: "The address `serve` listens on. A non-loopback address still needs --allow-remote."},
	{Section: "web", Name: "port", Env: envnames.WebPort, Kind: KindInt, Min: 1, Default: "7822",
		Doc: "The port `serve` listens on first. When it is taken the next free one is used, as with the default; only --port makes a taken port an error."},
	{Section: "web", Name: "base_path", Env: envnames.WebBasePath, Kind: KindString, Default: "/",
		Doc: "A path the web UI is served under, such as /authama, so it can sit behind a reverse proxy beside other sites. Every link, form and redirect carries it."},
	{Section: "web", Name: "auto_restart", Env: envnames.WebAutoRestart, Kind: KindBool, Default: "true",
		Doc: "Restart `serve` by itself when its binary is rebuilt or reinstalled (make install), on the same address with the same sign-in, and reload every open page into the new build; a page with unsaved input offers Reload instead. Off: a page says it is from an older build and offers Restart now."},
}

// Find returns the spec for a dotted key.
func Find(key string) (Spec, bool) {
	for _, s := range Specs {
		if s.Key() == key {
			return s, true
		}
	}
	return Spec{}, false
}

// File is a parsed authama.json: the dotted key of each setting it sets,
// with the value as the environment variable will carry it.
type File struct {
	Path   string
	Values map[string]string
	// VM holds the structured "hypervisors" and "devboxes" sections, which
	// have no environment variable.
	VM VMConfig
}

// ValidationError lists every problem found in an authama.json.
type ValidationError struct {
	Path     string
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s is not valid:\n  %s\n(run `authama settings validate` after fixing it; "+
		"the schema is at %s)", e.Path, strings.Join(e.Problems, "\n  "), SchemaURL)
}

// LoadFile reads and validates authama.json. A missing file is (nil, nil).
func LoadFile(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(path, b)
}

// Parse validates authama.json content.
func Parse(path string, b []byte) (*File, error) {
	bad := &ValidationError{Path: path}
	var top map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&top); err != nil {
		bad.Problems = append(bad.Problems, "not a JSON object: "+err.Error())
		return nil, bad
	}
	if dec.More() {
		bad.Problems = append(bad.Problems, "extra content after the JSON object")
	}
	f := &File{Path: path, Values: map[string]string{}}
	sections := map[string]bool{}
	for _, s := range Specs {
		sections[s.Section] = true
	}
	if raw, ok := top["version"]; ok {
		var v json.Number
		if err := json.Unmarshal(raw, &v); err != nil || v.String() != strconv.Itoa(Version) {
			bad.Problems = append(bad.Problems, fmt.Sprintf("version: this build reads version %d, got %s", Version, string(raw)))
		}
	}
	for _, name := range sortedKeys(top) {
		if name == "$schema" || name == "version" {
			continue
		}
		if structured[name] {
			bad.Problems = append(bad.Problems, parseStructured(f, name, top[name])...)
			continue
		}
		if !sections[name] {
			bad.Problems = append(bad.Problems, fmt.Sprintf("%s: unknown section (known: %s)", name, strings.Join(sectionNames(), ", ")))
			continue
		}
		var sec map[string]json.RawMessage
		d := json.NewDecoder(bytes.NewReader(top[name]))
		d.UseNumber()
		if err := d.Decode(&sec); err != nil || sec == nil {
			bad.Problems = append(bad.Problems, name+": must be an object")
			continue
		}
		for _, key := range sortedKeys(sec) {
			spec, ok := Find(name + "." + key)
			if !ok {
				bad.Problems = append(bad.Problems, fmt.Sprintf("%s.%s: unknown setting (known in %s: %s)", name, key, name, strings.Join(namesIn(name), ", ")))
				continue
			}
			v, err := spec.convert(sec[key])
			if err != nil {
				bad.Problems = append(bad.Problems, spec.Key()+": "+err.Error())
				continue
			}
			f.Values[spec.Key()] = v
		}
	}
	bad.Problems = append(bad.Problems, f.VM.crossCheck()...)
	if len(bad.Problems) > 0 {
		return nil, bad
	}
	return f, nil
}

// convert checks a JSON value against the spec and renders it as the
// environment variable's value. null is refused: leave the key out instead.
func (s Spec) convert(raw json.RawMessage) (string, error) {
	switch s.Kind {
	case KindString, KindPath, KindEnum:
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", fmt.Errorf("must be a string, got %s", string(raw))
		}
		if strings.TrimSpace(v) == "" {
			return "", errors.New("must not be empty (leave the key out to use the default)")
		}
		if s.Kind == KindEnum && !contains(s.Enum, v) {
			return "", fmt.Errorf("must be one of %s, got %q", strings.Join(s.Enum, ", "), v)
		}
		return v, nil
	case KindInt:
		var n json.Number
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&n); err != nil {
			return "", fmt.Errorf("must be an integer, got %s", string(raw))
		}
		i, err := strconv.Atoi(n.String())
		if err != nil {
			return "", fmt.Errorf("must be an integer, got %s", string(raw))
		}
		if i < s.Min {
			return "", fmt.Errorf("must be >= %d, got %d", s.Min, i)
		}
		return strconv.Itoa(i), nil
	case KindBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", fmt.Errorf("must be true or false, got %s", string(raw))
		}
		if b {
			return "1", nil
		}
		return "0", nil
	}
	return "", fmt.Errorf("unsupported kind %s", s.Kind)
}

// ApplyResult is what applying authama.json did.
type ApplyResult struct {
	Set, Kept []string // variables
}

// Apply puts the file's values into the environment, never over a variable
// already set (by the environment or by .env), under either spelling. Paths
// starting with ~ are expanded.
func (f *File) Apply(env Env) ApplyResult {
	env = env.orOS()
	var res ApplyResult
	if f == nil {
		return res
	}
	for _, s := range Specs {
		v, ok := f.Values[s.Key()]
		if !ok {
			continue
		}
		if envnames.IsSet(env.Lookup, s.Env) {
			res.Kept = append(res.Kept, s.Env)
			continue
		}
		if s.Kind == KindPath {
			v = expandHome(v)
		}
		if env.Setenv(s.Env, v) == nil {
			res.Set = append(res.Set, s.Env)
		}
	}
	return res
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return h
			}
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// Starter is the authama.json `settings init` writes: the schema reference
// and the version, nothing else, so every setting keeps its default until
// the user sets it.
func Starter() []byte {
	return []byte("{\n  \"$schema\": \"" + SchemaURL + "\",\n  \"version\": " + strconv.Itoa(Version) + "\n}\n")
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sectionNames() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range Specs {
		if !seen[s.Section] {
			seen[s.Section] = true
			out = append(out, s.Section)
		}
	}
	return append(out, structuredNames...)
}

func scalarSectionNames() []string {
	var out []string
	for _, s := range sectionNames() {
		if !structured[s] {
			out = append(out, s)
		}
	}
	return out
}

func namesIn(section string) []string {
	var out []string
	for _, s := range Specs {
		if s.Section == section {
			out = append(out, s.Name)
		}
	}
	return out
}
