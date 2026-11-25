package setupxfer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/bundler"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/secrets"
)

// FullResult is a full export written.
type FullResult struct {
	Path     string
	SHA256   string
	Contents []string // archive member names; never a value
	// Missing names a variable the setup reads a token from that had no value
	// here, so the export carries none for it.
	Missing []string
}

// WriteFull writes p's full setup, encrypted with passphrase, to path, which
// must not exist. lookup reads a token's value (os.LookupEnv, after .env was
// loaded). The token values go into the archive from memory: no plaintext
// copy of them, or of any key, is written anywhere.
func WriteFull(p paths.Paths, path string, passphrase []byte, version string, now time.Time,
	lookup func(string) (string, bool)) (FullResult, error) {
	d, err := Build(p, version, now)
	if err != nil {
		return FullResult{}, err
	}
	envText, missing := EnvText(d.EnvNames, lookup)
	extra := map[string][]byte{bundler.SecretsMember: []byte(envText)}
	if len(d.Tokens) > 0 {
		extra["config/tokens.json"] = d.Tokens
	}
	if len(d.Settings) > 0 {
		extra["config/authama.json"] = d.Settings
	}
	b, err := bundler.New(p.SSHDir, p.ConfigDir, bundler.PassphraseCipher{Passphrase: passphrase}).
		WithKeyRoot(manifest.KeyRootAt(p.Manifest())).WithSetupMembers(extra)
	if err != nil {
		return FullResult{}, err
	}
	res, err := b.WriteFile(path)
	if err != nil {
		return FullResult{}, err
	}
	return FullResult{Path: res.AgePath, SHA256: res.SHA256, Contents: res.Contents, Missing: missing}, nil
}

// Full is a full export decrypted and read, nothing written yet.
type Full struct {
	Doc       Doc    // its config, as a setup document (checked by Parse)
	Inventory []byte // its inventory.json, or nil
	Env       string // its token values, as .env lines: only the names its setup reads tokens from
	// EnvSkipped names the export's .env lines that are not imported: a name
	// its setup reads no token from, or one that would change how authama or
	// the programs it runs behave (GIT_*, SSH_*, LD_*, PATH, ...).
	EnvSkipped []string
	b          *bundler.Bundler
	opened     *bundler.Opened // the key files only
}

// OpenFull decrypts the full export at path with passphrase and checks it:
// the config as Parse checks a document, every key file against the
// backup's allow-list. Replaced key files will be kept in replacedDir.
func OpenFull(p paths.Paths, path string, passphrase []byte, replacedDir string, allowRisky bool) (*Full, error) {
	b := bundler.New(p.SSHDir, p.ConfigDir, bundler.PassphraseCipher{Passphrase: passphrase}).
		WithKeyRoot(manifest.KeyRootAt(p.Manifest())).WithReplacedDir(replacedDir)
	if allowRisky {
		b = b.AllowRiskyOptions()
	}
	o, err := b.Open(path, "")
	if err != nil {
		return nil, err
	}
	f := &Full{b: b, opened: o, Doc: Doc{Format: Format, Version: Version}}
	take := func(name string) []byte { data, _ := o.Take(name); return data }
	f.Doc.Manifest = take("config/manifest.json")
	f.Doc.Providers = take("config/providers.json")
	f.Doc.Tokens = take("config/tokens.json")
	f.Doc.Settings = take("config/authama.json")
	f.Inventory = take("config/inventory.json")
	f.Env = string(take(bundler.SecretsMember))
	body, err := json.Marshal(f.Doc)
	if err != nil {
		return nil, err
	}
	if f.Doc, err = Parse(body); err != nil {
		return nil, err
	}
	if f.Env, f.EnvSkipped, err = onlyTokenNames(f.Env, f.Doc); err != nil {
		return nil, err
	}
	if len(f.Inventory) > 0 {
		var inv inventory.Inventory
		if err := json.Unmarshal(f.Inventory, &inv); err != nil {
			return nil, fmt.Errorf("the export's inventory does not parse: %w", err)
		}
	}
	return f, nil
}

// Risky lists what in the export could redirect or weaken connections, or run
// a program, once imported: the SSH options in its manifest (as the key backup
// checks them), its providers that send a token elsewhere, and its custom
// hypervisor commands.
func (f *Full) Risky() []manifest.RiskyOption {
	return append(append(f.opened.Risky(), f.Doc.riskyContent()...), riskyEnv(f.Env)...)
}

// riskyEnv names the token values that run a command when used (cmd:, op://):
// the export's maker chose that command, and authama runs it on the first
// provider call that reads the token. A literal value is only ever sent.
func riskyEnv(env string) []manifest.RiskyOption {
	pairs, _ := settings.ParseDotenv(strings.NewReader(env))
	var r []manifest.RiskyOption
	for _, p := range pairs {
		if secrets.Runs(p.Value) {
			r = append(r, manifest.RiskyOption{Where: ".env", Key: p.Key, Value: p.Value,
				Why: "authama runs this command to read the token"})
		}
	}
	return r
}

// KeyItems lists the key files the export would lay down, settled by policy.
func (f *Full) KeyItems(policy Policy) (Result, error) {
	states, err := f.b.KeyStates(f.opened)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, s := range states {
		st := s.State
		switch st {
		case "new":
			st = StateAdd
		case "same":
			st = StateSame
		default:
			st = settle(false, policy)
		}
		res.Items = append(res.Items, Item{Area: "key", Name: s.Dest, State: st})
	}
	return res, nil
}

// LayKeys writes the key files: new ones, and on Replace those that differ,
// each kept first in the replaced folder. On Keep a differing file is left as
// it is. Files are 0600 and folders 0700.
func (f *Full) LayKeys(policy Policy, fingerprintOf func(string) (string, error)) (bundler.RestoreResult, error) {
	states, err := f.b.KeyStates(f.opened)
	if err != nil {
		return bundler.RestoreResult{}, err
	}
	drop := map[string]bool{}
	for _, s := range states {
		if s.State == "same" || (s.State == "differs" && policy != Replace) {
			drop[s.Name] = true
		}
	}
	if policy == Fail {
		for _, s := range states {
			if s.State == "differs" {
				return bundler.RestoreResult{}, errors.New("a key file differs and --on-conflict fail writes nothing: " + s.Dest)
			}
		}
	}
	f.opened.Drop(drop)
	return f.b.LayDown(f.opened, fingerprintOf)
}

// MergeInventory merges the export's inventory into p's, by fingerprint.
func MergeInventory(p paths.Paths, raw []byte, policy Policy) (Result, error) {
	var res Result
	if len(raw) == 0 {
		return res, nil
	}
	var in inventory.Inventory
	if err := json.Unmarshal(raw, &in); err != nil {
		return res, err
	}
	local, err := inventory.Load(p.Inventory())
	if err != nil {
		if _, statErr := os.Stat(p.Inventory()); statErr == nil {
			return res, err
		}
		local = inventory.New()
	}
	fps := make([]string, 0, len(in.Keys))
	for fp := range in.Keys {
		fps = append(fps, fp)
	}
	sort.Strings(fps)
	changed := false
	for _, fp := range fps {
		rec := in.Keys[fp]
		st := StateAdd
		if ours, ok := local.Keys[fp]; ok {
			st = settle(sameJSON(ours, rec), policy)
		}
		res.Items = append(res.Items, Item{Area: "record", Name: rec.Profile + " " + fp, State: st})
		if st == StateAdd || st == StateReplace {
			local.Record(fp, rec)
			changed = true
		}
	}
	if policy == Fail {
		if c := res.Conflicts(); len(c) > 0 {
			return res, ConflictError{Items: c}
		}
	}
	if !changed {
		return res, nil
	}
	return res, local.Save(p.Inventory())
}

// onlyTokenNames keeps the lines of env whose name d's setup reads a token
// from (as the export wrote them: EnvText over EnvNames) and is not a name
// that changes how authama or a program it runs behaves. An export does not
// prove who made it, and every name in .env is set in authama's environment.
func onlyTokenNames(env string, d Doc) (kept string, skipped []string, err error) {
	if env == "" {
		return "", nil, nil
	}
	pairs, warns := settings.ParseDotenv(strings.NewReader(env))
	if len(warns) > 0 {
		return "", nil, fmt.Errorf("the export's token values do not parse: %s", strings.Join(warns, "; "))
	}
	var m *manifest.Manifest
	if len(d.Manifest) > 0 {
		if m, err = parseManifest(d.Manifest); err != nil {
			return "", nil, err
		}
	}
	allowed := map[string]bool{}
	for _, n := range envNames(d.Tokens, m) {
		allowed[n] = !envNameReserved(n)
	}
	var b strings.Builder
	for _, p := range pairs {
		if !allowed[p.Key] {
			skipped = append(skipped, p.Key)
			continue
		}
		b.WriteString(p.Key + "=" + Quote(p.Value) + "\n")
	}
	sort.Strings(skipped)
	return b.String(), slices.Compact(skipped), nil
}

// reservedEnvPrefixes and reservedEnvNames change how authama, git, ssh or the
// dynamic loader behave, so no import writes them into .env, whatever the
// setup names.
var (
	reservedEnvPrefixes = []string{"AUTHAMA_", "GIT_", "SSH_", "LD_", "DYLD_", "GNUPG", "GPG_"}
	reservedEnvNames    = map[string]bool{"PATH": true, "HOME": true, "SHELL": true, "TMPDIR": true, "USER": true,
		"XDG_CONFIG_HOME": true, "XDG_STATE_HOME": true, "XDG_DATA_HOME": true, "XDG_RUNTIME_DIR": true,
		"BROWSER": true, "EDITOR": true, "VISUAL": true, "PAGER": true, "IFS": true, "ENV": true, "BASH_ENV": true,
		"PROMPT_COMMAND": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "HTTP_PROXY": true, "HTTPS_PROXY": true,
		"ALL_PROXY": true, "NO_PROXY": true, "http_proxy": true, "https_proxy": true, "all_proxy": true, "no_proxy": true,
		"GOFLAGS": true, "GOPATH": true, "GOROOT": true, "GOPROXY": true, "GOTOOLCHAIN": true}
)

func envNameReserved(n string) bool {
	if reservedEnvNames[n] {
		return true
	}
	for _, p := range reservedEnvPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}
