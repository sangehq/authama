// Package setupxfer moves a whole authama setup between machines, in two forms.
//
// The setup document (Build, Parse) holds no secret: the manifest, the
// providers, the settings without their machine-specific paths, the token
// config - names of environment variables, never values - and the names
// themselves. It is JSON, safe to read, diff and keep in a repository.
//
// The full export (WriteFull, OpenFull) adds what the document leaves out:
// every private key under the key root, the inventory, and the token values
// the setup names, read from the environment (and so from .env). It is an
// age file encrypted with a passphrase (util/agepass), so `age -d` opens it
// too, and the archive inside is the backup bundle's own (services/bundler)
// with the settings, the token config and secrets/tokens.env beside it.
//
// Import merges either into a home (Merge, MergeEnv): what is new is added,
// what is the same is left alone, and what differs is a conflict the Policy
// settles - keep what is here, replace it, or fail before writing anything.
package setupxfer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/hypervisor"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/providers"
	"github.com/sangehq/authama/src/internal/core/tokens"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Format names a setup document; Version is the one this build writes and reads.
const (
	Format  = "authama-setup"
	Version = 1
)

// MaxDocBytes bounds a setup document Parse accepts.
const MaxDocBytes = 4 << 20

// machineSections are the settings sections that name places on this machine;
// a setup document leaves them out and an import never writes them.
var machineSections = map[string]bool{"paths": true}

// Doc is a setup document. Secrets is always false: the field is there so a
// reader can tell this file from the encrypted full export at a glance.
type Doc struct {
	Format     string          `json:"format"`
	Version    int             `json:"version"`
	ExportedAt string          `json:"exported_at"`
	Authama    string          `json:"authama,omitempty"`
	Secrets    bool            `json:"secrets"`
	Manifest   json.RawMessage `json:"manifest,omitempty"`
	Providers  json.RawMessage `json:"providers,omitempty"`
	Tokens     json.RawMessage `json:"tokens,omitempty"`
	Settings   json.RawMessage `json:"settings,omitempty"`
	// EnvNames are the environment variables the setup reads tokens from.
	EnvNames []string `json:"env_names,omitempty"`
	// Profiles and Keys list what the manifest declares, for the reader:
	// every key a profile names or one of its hosts uses, as profile/name.
	// The document carries none of the keys.
	Profiles []string `json:"profiles,omitempty"`
	Keys     []string `json:"keys,omitempty"`
}

// Build reads p's setup into a document. A file p lacks is left out.
func Build(p paths.Paths, version string, now time.Time) (Doc, error) {
	d := Doc{Format: Format, Version: Version, ExportedAt: now.UTC().Format(time.RFC3339), Authama: version}
	read := func(path string) (json.RawMessage, error) {
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("%s is not valid JSON", path)
		}
		var out bytes.Buffer
		if err := json.Compact(&out, b); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	var err error
	if d.Manifest, err = read(p.Manifest()); err != nil {
		return d, err
	}
	if d.Providers, err = read(p.Providers()); err != nil {
		return d, err
	}
	if d.Tokens, err = read(p.Tokens()); err != nil {
		return d, err
	}
	if len(d.Tokens) > 0 {
		// The token config holds names; one holding a value is not exported.
		if err := tokens.CheckNamesOnly(string(d.Tokens)); err != nil {
			return d, fmt.Errorf("%s holds something other than variable names, so it is not exported: %w", p.Tokens(), err)
		}
	}
	settings, err := read(p.SettingsFile())
	if err != nil {
		return d, err
	}
	if d.Settings, err = withoutMachineSections(settings); err != nil {
		return d, fmt.Errorf("%s: %w", p.SettingsFile(), err)
	}
	d.EnvNames = EnvNames(p)
	if m, err := manifest.Load(p.Manifest()); err == nil {
		for _, name := range m.ProfileNames() {
			d.Profiles = append(d.Profiles, name)
			d.Keys = append(d.Keys, profileKeys(m, name)...)
		}
	}
	return d, nil
}

func withoutMachineSections(settings json.RawMessage) (json.RawMessage, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(settings, &top); err != nil {
		return nil, err
	}
	for s := range machineSections {
		delete(top, s)
	}
	return json.Marshal(top)
}

// EnvNames lists, sorted, the environment variables p's setup reads tokens
// from: every *env field of the token config, and each host's token_env.
func EnvNames(p paths.Paths) []string {
	tok, _ := os.ReadFile(p.Tokens())
	var m *manifest.Manifest
	if loaded, err := manifest.Load(p.Manifest()); err == nil {
		m = loaded
	}
	return envNames(tok, m)
}

// envNames is EnvNames over a token config's text and a manifest (either may
// be empty or nil).
func envNames(tok []byte, m *manifest.Manifest) []string {
	seen := map[string]bool{}
	if len(tok) > 0 {
		if c, err := tokens.ParseConfig(string(tok)); err == nil {
			for _, s := range c.Specs() {
				for _, n := range []string{s.Env, s.UserEnv, s.PassEnv, s.KeyEnv, s.SecretEnv} {
					if n != "" {
						seen[n] = true
					}
				}
			}
		}
	}
	if m != nil {
		for _, prof := range m.Profiles {
			for _, h := range prof.Hosts {
				if h.TokenEnv != nil && *h.TokenEnv != "" {
					seen[*h.TokenEnv] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Marshal renders the document as indented JSON with a trailing newline.
func (d Doc) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Parse reads a setup document and checks it before anything uses it: the
// format and version, a manifest that validates, a token config that holds
// names only, settings that are an object. It never trusts Secrets: a
// document is refused if its token config holds a value whatever it says.
func Parse(b []byte) (Doc, error) {
	var d Doc
	if len(b) > MaxDocBytes {
		return d, fmt.Errorf("the setup document is %d bytes, over the %d this reads", len(b), MaxDocBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, fmt.Errorf("not a setup document: %w", err)
	}
	if d.Format != Format {
		return d, fmt.Errorf("not a setup document (format %q, want %q)", d.Format, Format)
	}
	if d.Version != Version {
		return d, fmt.Errorf("this build reads setup documents of version %d, not %d", Version, d.Version)
	}
	if d.Secrets {
		return d, errors.New("the document says it holds secrets; a setup document never does (a full export is an encrypted .age file)")
	}
	if len(d.Manifest) > 0 {
		if _, err := parseManifest(d.Manifest); err != nil {
			return d, err
		}
	}
	if len(d.Tokens) > 0 {
		if err := tokens.CheckNamesOnly(string(d.Tokens)); err != nil {
			return d, fmt.Errorf("the document's token config: %w", err)
		}
	}
	if len(d.Settings) > 0 {
		var top map[string]json.RawMessage
		if err := json.Unmarshal(d.Settings, &top); err != nil {
			return d, fmt.Errorf("the document's settings are not an object: %w", err)
		}
	}
	if len(d.Providers) > 0 && !json.Valid(d.Providers) {
		return d, errors.New("the document's providers are not valid JSON")
	}
	for _, n := range d.EnvNames {
		if strings.ContainsAny(n, "=\n\r\"' ") || n == "" {
			return d, fmt.Errorf("the document names an invalid environment variable %q", n)
		}
	}
	return d, nil
}

func parseManifest(b []byte) (*manifest.Manifest, error) {
	var m manifest.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("the document's manifest does not parse: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("the document's manifest is not valid: %w", err)
	}
	return &m, nil
}

// Risky lists what in the document could redirect or weaken connections, or
// run a program, once imported: a document does not prove who made it. That is
// the SSH options in its manifest, the providers that would send a token to a
// host this build's catalog does not, and the custom hypervisor commands in its
// settings, which authama runs whenever it lists VMs.
func (d Doc) Risky() []manifest.RiskyOption {
	var r []manifest.RiskyOption
	if len(d.Manifest) > 0 {
		r, _ = manifest.RiskyOptionsInJSON(d.Manifest)
	}
	return append(r, d.riskyContent()...)
}

// riskyContent is Risky without the manifest's SSH options.
func (d Doc) riskyContent() []manifest.RiskyOption {
	var r []manifest.RiskyOption
	if len(d.Providers) > 0 {
		redirects, err := providers.RedirectsIn(d.Providers)
		if err != nil {
			r = append(r, manifest.RiskyOption{Where: "providers.json", Key: "providers", Value: "?", Why: "does not parse, so where it sends tokens cannot be shown"})
		}
		for _, rd := range redirects {
			token := rd.TokenEnv
			if token == "" {
				token = "its token"
			}
			r = append(r, manifest.RiskyOption{Where: "providers.json", Key: rd.Name, Value: rd.Dest,
				Why: "sends " + token + " to this host, which this build's catalog does not"})
		}
	}
	if len(d.Settings) > 0 {
		var top map[string]json.RawMessage
		_ = json.Unmarshal(d.Settings, &top)
		var hv map[string]hypervisor.CommandSpec
		if raw, ok := top[settings.SectionHypervisors]; ok && json.Unmarshal(raw, &hv) == nil {
			names := make([]string, 0, len(hv))
			for n := range hv {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				spec := hv[n]
				for _, c := range []struct {
					field string
					argv  []string
				}{{"list", spec.List}, {"address", spec.Address}, {"start", spec.Start}, {"exec", spec.Exec}} {
					if len(c.argv) > 0 {
						r = append(r, manifest.RiskyOption{Where: "settings hypervisors." + n, Key: c.field,
							Value: strings.Join(c.argv, " "), Why: "authama runs this command"})
					}
				}
			}
		}
		r = append(r, riskySettings(top)...)
	}
	return r
}

// riskySetting is a setting whose value, set by someone else, sends backups to
// their key, hides what happened, re-trusts a changed host key, or opens
// authama to other programs. risky reports whether value (as the document
// spells it) is such a value; a value matching the default is never risky.
type riskySetting struct {
	section, name, why string
	risky              func(value string) bool
}

var riskySettingList = []riskySetting{
	{"backup", "age_recipient", "encrypts your key backups to this recipient", func(v string) bool { return v != "" }},
	{"history", "enabled", "turns off the record of what authama changes", func(v string) bool { return v == "false" }},
	{"vm", "repin_changed", "trusts a VM's changed host key without asking", func(v string) bool { return v == "true" }},
	{"api", "enabled", "opens the HTTP API", func(v string) bool { return v == "true" }},
	{"mcp", "enabled", "opens the MCP server", func(v string) bool { return v == "true" }},
	{"webhooks", "enabled", "sends events to webhook endpoints", func(v string) bool { return v == "true" }},
	{"webhooks", "allow_private", "lets webhooks reach private addresses", func(v string) bool { return v == "true" }},
	{"webhooks", "include_details", "puts error text and subjects in webhook events", func(v string) bool { return v == "true" }},
}

func riskySettings(top map[string]json.RawMessage) []manifest.RiskyOption {
	var r []manifest.RiskyOption
	for _, rs := range riskySettingList {
		var section map[string]any
		if json.Unmarshal(top[rs.section], &section) != nil {
			continue
		}
		raw, ok := section[rs.name]
		if !ok || raw == nil {
			continue
		}
		v := strings.TrimSpace(fmt.Sprint(raw))
		if rs.risky(v) {
			r = append(r, manifest.RiskyOption{Where: "settings " + rs.section, Key: rs.name, Value: v, Why: rs.why})
		}
	}
	return r
}

// profileKeys is every key profile name uses, once each as profile/key: the
// keys it declares, then each host's (its own, the profile's shared one, or
// the name derived from its alias).
func profileKeys(m *manifest.Manifest, name string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, name+"/"+k)
		}
	}
	prof := m.Profiles[name]
	for _, k := range prof.Keys {
		add(k.Name)
	}
	for _, h := range prof.Hosts {
		if k, err := m.ResolvedKeyName(name, h); err == nil {
			add(k)
		}
	}
	return out
}

// RiskyError refuses an import over risky, naming each item and how to
// import it anyway at a terminal.
func RiskyError(risky []manifest.RiskyOption, command string) error {
	lines := []string{"refusing the import: it holds settings that could redirect or weaken your connections, send a token elsewhere, or run a program:"}
	for _, r := range risky {
		lines = append(lines, "  "+r.String())
	}
	lines = append(lines,
		"A setup document or export does not prove who made it.",
		"If you made it yourself, import it with: "+command+" setup import FILE --allow-risky-options",
		"Otherwise do not import it, or ask whoever sent it.")
	return errors.New(strings.Join(lines, "\n"))
}
