package selection

import (
	"fmt"
	"sort"

	"github.com/simtabi/authama/src/internal/services/dataset"
)

// Defaults are each grain's columns when a selection names none: exactly
// the columns, headings and order of the list's CSV before the pipeline
// (hosts.csv, keys.csv, profiles.csv, tokens.csv), which the parity test
// holds them to.
var Defaults = map[string][]Column{
	GrainHost: {{Field: "profile"}, {Field: "alias"}, {Field: "hostname"}, {Field: "provider"}, {Field: "key"},
		{Field: "status"}, {Field: "tags"}},
	GrainKey: {{Field: "ref", As: "key"}, {Field: "profile"}, {Field: "type"}, {Field: "fingerprint"},
		{Field: "created"}, {Field: "expires_on", As: "expires"}, {Field: "hosts"}, {Field: "on_disk"}, {Field: "state"}},
	GrainProfile: {{Field: "profile"}, {Field: "category"}, {Field: "key_scope"}, {Field: "key_store"},
		{Field: "hosts"}, {Field: "keys"}, {Field: "key_comment"}, {Field: "key_names"}},
	GrainToken: {{Field: "target", As: "tool"}, {Field: "host"}, {Field: "kind"}, {Field: "description"},
		{Field: "username", As: "user"}, {Field: "secret_present", As: "secret_set"}, {Field: "file"}},
}

// rowsOf is every row of a grain, and every field its rows have, sorted.
func rowsOf(ds *dataset.Dataset, grain string) ([]row, []string, error) {
	profiles := map[string]dataset.Profile{}
	for _, p := range ds.Profiles {
		profiles[p.Name] = p
	}
	keysByRef := map[string]dataset.Key{}
	keysOf := map[string][]dataset.Key{}
	for _, k := range ds.Keys {
		keysByRef[k.Ref] = k
		keysOf[k.Profile] = append(keysOf[k.Profile], k)
	}
	withProfile := func(f map[string]any, name string) {
		p := profiles[name]
		f["profile.code"], f["profile.category"], f["profile.key_store"] = p.Code, p.Category, p.KeyStore
	}

	var rows []row
	switch grain {
	case GrainProfile:
		for _, p := range ds.Profiles {
			names := []string{}
			for _, k := range keysOf[p.Name] {
				names = append(names, k.Name)
			}
			f := map[string]any{
				"profile": p.Name, "name": p.Name, "code": p.Code, "own_code": p.OwnCode, "category": p.Category,
				"key_scope": p.KeyScope, "key_store": p.KeyStore, "key_comment": p.KeyComment,
				"hosts": p.Hosts, "keys": p.Keys, "key_names": names,
				"naming.alias": p.Naming.Alias, "naming.key_name": p.Naming.KeyName,
				"naming.envs": p.Naming.Envs, "naming.roles": p.Naming.Roles, "naming.enforce": p.Naming.Enforce,
			}
			rows = append(rows, row{f: f, keys: keysOf[p.Name]})
		}
	case GrainKey:
		for _, k := range ds.Keys {
			f := map[string]any{
				"ref": k.Ref, "profile": k.Profile, "name": k.Name, "type": k.Type, "fingerprint": k.Fingerprint,
				"created": k.Created, "expires_on": k.ExpiresOn, "rotate_after_days": k.RotateAfterDays,
				"store": k.Store, "on_disk": k.OnDisk, "recorded": k.Recorded, "state": k.State, "status": k.State,
				"hosts": k.Hosts, "host_count": len(k.Hosts), "deployments": k.Deployments,
			}
			withProfile(f, k.Profile)
			rows = append(rows, row{f: f, keys: []dataset.Key{k}})
		}
	case GrainHost:
		for _, h := range ds.Hosts {
			opts := make([]string, 0, len(h.Options))
			for _, o := range h.Options {
				opts = append(opts, o.Name+"="+o.Value)
			}
			f := map[string]any{
				"alias": h.Alias, "profile": h.Profile, "hostname": h.Hostname, "user": h.User, "port": h.Port,
				"provider": h.Provider, "key": h.Key, "key_ref": h.KeyRef, "status": h.Status, "tags": h.Tags,
				"env": h.Env, "role": h.Role, "code": h.Code, "number": h.Number, "named": h.Named,
				"requires_vpn": h.RequiresVPN, "vpn_name": h.VPNName, "vm": h.VM, "proxy": h.Proxy,
				"token_env": h.TokenEnv, "options": opts,
			}
			k, ok := keysByRef[h.KeyRef]
			f["key.type"], f["key.fingerprint"], f["key.expires_on"], f["key.state"] = k.Type, k.Fingerprint, k.ExpiresOn, k.State
			withProfile(f, h.Profile)
			var keys []dataset.Key
			if ok {
				keys = []dataset.Key{k}
			}
			rows = append(rows, row{f: f, keys: keys})
		}
	case GrainToken:
		for _, t := range ds.Tokens {
			rows = append(rows, row{f: map[string]any{
				"target": t.Target, "host": t.Host, "kind": t.Kind, "username": t.Username,
				"secret_present": t.SecretPresent, "description": t.Description, "file": t.File,
			}})
		}
	default:
		return nil, nil, fmt.Errorf("%w %q; one of %v", ErrUnknownGrain, grain, Grains)
	}
	return rows, fieldsOf(grain, rows), nil
}

// fieldsOf is every field the grain's rows have, sorted. A grain with no
// rows still offers its default columns.
func fieldsOf(grain string, rows []row) []string {
	seen := map[string]bool{}
	for _, c := range Defaults[grain] {
		seen[c.Field] = true
	}
	for _, r := range rows {
		for k := range r.f {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
