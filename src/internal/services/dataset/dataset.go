// Package dataset builds the export dataset: every profile, key, host and
// token of a home as typed, related records, with no secret on any of them.
//
// It is the first stage of the export pipeline (dataset, then a selection,
// then a format). It reads through the same services the pages and the CLI
// use, so a fact has one source: a host's status is query's, a key's state
// is keysvc's, a token's presence is tokensvc's. Records link by name (a
// host's Key is a key's Ref), never by copying, so a selection can follow a
// relation in either direction.
package dataset

import (
	"fmt"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/query"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Version is the dataset's shape. A field is only ever added; a rename or a
// removal bumps it, so a saved selection can tell what it was written for.
const Version = 1

// Dataset is a home's records, in manifest order within each list.
type Dataset struct {
	Version  int       `json:"version"`
	Profiles []Profile `json:"profiles"`
	Keys     []Key     `json:"keys"`
	Hosts    []Host    `json:"hosts"`
	Tokens   []Token   `json:"tokens"`
}

// Profile is one profile and its rules.
type Profile struct {
	Name       string `json:"name"`
	Code       string `json:"code"`        // its own code, or the fallback in effect; "" when none
	OwnCode    bool   `json:"own_code"`    // Code is the profile's own, not the fallback
	Category   string `json:"category"`    // orgs, clients, personal, ...
	KeyScope   string `json:"key_scope"`   // per_service, per_host, ...
	KeyStore   string `json:"key_store"`   // disk or 1password
	KeyComment string `json:"key_comment"` // its own key_comment template; "" inherits
	Naming     Naming `json:"naming"`      // the naming rules in effect for it
	Hosts      int    `json:"hosts"`
	Keys       int    `json:"keys"`
}

// Naming is the naming rules in effect for a profile.
type Naming struct {
	Alias   string   `json:"alias"`
	KeyName string   `json:"key_name"`
	Envs    []string `json:"envs"`
	Roles   []string `json:"roles"`
	Enforce string   `json:"enforce"`
}

// Key is one key: where it is, what it is, and which hosts use it.
type Key struct {
	Ref             string   `json:"ref"` // profile/name, the selector the CLI takes
	Profile         string   `json:"profile"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Fingerprint     string   `json:"fingerprint"`
	Created         string   `json:"created"`
	ExpiresOn       string   `json:"expires_on"`
	RotateAfterDays int      `json:"rotate_after_days"`
	Store           string   `json:"store"` // disk or 1password
	OnDisk          bool     `json:"on_disk"`
	Recorded        bool     `json:"recorded"` // the inventory records it
	State           string   `json:"state"`    // no-key, needs-redeploy, deployed
	Hosts           []string `json:"hosts"`    // aliases using it
	Deployments     int      `json:"deployments"`
}

// Host is one host with every SSH setting it has.
type Host struct {
	Alias       string   `json:"alias"`
	Profile     string   `json:"profile"`
	Hostname    string   `json:"hostname"`
	User        string   `json:"user"`
	Port        int      `json:"port"`
	Provider    string   `json:"provider"` // e.g. github/vcs or server
	Key         string   `json:"key"`      // the key's name; "" when it has none
	KeyRef      string   `json:"key_ref"`  // profile/name, to join with Key.Ref
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	Env         string   `json:"env"`    // parsed from the alias by the naming rules; "" for an older name
	Role        string   `json:"role"`   // likewise
	Code        string   `json:"code"`   // likewise
	Number      string   `json:"number"` // likewise, as written (00)
	Named       bool     `json:"named"`  // the alias follows the naming rules
	RequiresVPN bool     `json:"requires_vpn"`
	VPNName     string   `json:"vpn_name"`
	VM          bool     `json:"vm"`
	Proxy       bool     `json:"proxy"`
	TokenEnv    string   `json:"token_env"` // the variable's name, never its value
	Options     []Option `json:"options"`   // raw ssh_config options, in order
}

// Option is one raw ssh_config option of a host.
type Option struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Token is one installed credential, described: never the secret.
type Token struct {
	Target        string `json:"target"` // the tool: npm, composer, git, ...
	Host          string `json:"host"`
	Kind          string `json:"kind"`
	Username      string `json:"username"`
	SecretPresent bool   `json:"secret_present"`
	Description   string `json:"description"`
	File          string `json:"file"`
}

// Options are what Build reads from the process, for a test to replace.
type Options struct {
	// Getenv finds the token files (HOME and the tools' own variables);
	// nil is os.Getenv, as the tokens page does.
	Getenv func(string) string
	// Cwd is the directory project-level token files are looked for in; ""
	// is the working directory.
	Cwd string
}

// Build reads the home's records.
func Build(p paths.Paths, o Options) (*Dataset, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, err // it names the file, and says to run authama init
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		inv = &inventory.Inventory{Keys: map[string]inventory.KeyRecord{}}
	}
	ds := &Dataset{Version: Version, Profiles: []Profile{}, Keys: []Key{}, Hosts: []Host{}, Tokens: []Token{}}

	rows, err := keysvc.New(m, inv, p.SSHDir).Rows("")
	if err != nil {
		return nil, fmt.Errorf("keys: %w", err)
	}
	keysPer := map[string]int{}
	for _, r := range rows {
		keysPer[r.Ref.Profile]++
		ds.Keys = append(ds.Keys, keyOf(r))
	}

	for _, name := range m.ProfileNames() {
		ds.Profiles = append(ds.Profiles, profileOf(m, name, keysPer[name]))
	}

	groups, err := query.New(m, inv, p.Providers()).Groups("", "", "", "")
	if err != nil {
		return nil, fmt.Errorf("hosts: %w", err)
	}
	for _, g := range groups {
		byAlias := map[string]manifest.Host{}
		for _, h := range m.Profiles[g.Name].Hosts {
			byAlias[h.Alias] = h
		}
		for _, r := range g.Rows {
			ds.Hosts = append(ds.Hosts, hostOf(m, g.Name, r, byAlias[r.Alias]))
		}
	}

	for _, t := range tokensvc.NewEnv(p, o.Getenv, nil, o.Cwd).Inventory("", false) {
		// Inventory(..., false) never reads a secret; Token has no field for one.
		ds.Tokens = append(ds.Tokens, Token{Target: t.Target, Host: t.Host, Kind: t.Kind, Username: t.Username,
			SecretPresent: t.SecretPresent, Description: t.Description, File: t.File})
	}
	return ds, nil
}

func keyOf(r keysvc.Row) Key {
	store := "disk"
	if r.InOnePassword() {
		store = "1password"
	}
	hosts := r.Hosts
	if hosts == nil {
		hosts = []string{}
	}
	return Key{Ref: r.Ref.String(), Profile: r.Ref.Profile, Name: r.Ref.KeyName, Type: r.Type,
		Fingerprint: r.Fingerprint, Created: r.Created, ExpiresOn: r.ExpiresOn, RotateAfterDays: r.RotateAfterDays,
		Store: store, OnDisk: r.PrivateOnDisk, Recorded: r.Recorded, State: r.Status, Hosts: hosts,
		Deployments: len(r.Deployments)}
}

func profileOf(m *manifest.Manifest, name string, keys int) Profile {
	p := m.Profiles[name]
	store := p.KeyStore
	if store == "" {
		store = "disk"
	}
	r := manifest.ProfileNamingRules(m, name)
	nonNil := func(s []string) []string {
		if s == nil {
			return []string{}
		}
		return s
	}
	return Profile{Name: name, Code: manifest.ProfileCode(m, name), OwnCode: p.Code != "", Category: p.Category,
		KeyScope: p.KeyScope, KeyStore: store, KeyComment: p.KeyComment,
		Naming: Naming{Alias: r.Alias, KeyName: r.KeyName, Envs: nonNil(r.Envs), Roles: nonNil(r.Roles), Enforce: r.Enforce},
		Hosts:  len(p.Hosts), Keys: keys}
}

func hostOf(m *manifest.Manifest, profile string, r query.HostRow, h manifest.Host) Host {
	out := Host{Alias: r.Alias, Profile: profile, Hostname: r.Hostname, User: h.User, Port: h.Port,
		Provider: r.ProviderLabel, Key: r.KeyName, Status: r.Status, Tags: r.Tags,
		RequiresVPN: h.RequiresVPN, VM: h.VM != nil, Proxy: h.Proxy != nil, Options: []Option{}}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if r.KeyName != "" {
		out.KeyRef = manifest.KeyRef{Profile: profile, KeyName: r.KeyName}.String()
	}
	if h.VPNName != nil {
		out.VPNName = *h.VPNName
	}
	if h.TokenEnv != nil {
		out.TokenEnv = *h.TokenEnv
	}
	for _, k := range h.RawOptions.Keys() {
		out.Options = append(out.Options, Option{Name: k, Value: h.RawOptions.Get(k)})
	}
	if parts, ok := manifest.ProfileNamingRules(m, profile).Parse(r.Alias); ok {
		out.Named = true
		out.Env, out.Role, out.Code = parts["env"], parts["role"], parts["code"]
		for k, v := range parts {
			if k != "env" && k != "role" && k != "code" {
				out.Number = v // the number part, whatever its width ({nn}, {nnn})
			}
		}
	}
	return out
}
