package dataset

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// home is a home with two profiles, one with a code and named servers, a
// private key on disk and an npm token, every secret generated for the run.
func home(t *testing.T) (paths.Paths, Options, []string) {
	t.Helper()
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	userHome := filepath.Join(base, "home")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	random := func() string {
		b := make([]byte, 18)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(b)
	}
	tokenValue, keyBody := random(), random()
	write(p.Manifest(), `{"version":1,"defaults":{"key_type":"ed25519","naming":{"envs":["prd","stg"]}},"profiles":{
	  "simtabi":{"key_scope":"per_service","code":"simbi","category":"orgs","hosts":[
	    {"alias":"simbi-prd-web-00","hostname":"203.0.113.10","user":"ploi","tags":["web","prd"],
	     "raw_options":{"ServerAliveInterval":"60"}},
	    {"alias":"oribi-web","hostname":"203.0.113.11","user":"ploi"}]},
	  "work":{"key_scope":"per_service","hosts":[{"alias":"gh","hostname":"github.com","user":"git"}]}}}`)
	write(p.Inventory(), `{"version":1,"keys":{}}`)
	write(filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519"), "-----BEGIN OPENSSH PRIVATE KEY-----\n"+keyBody+"\n")
	write(filepath.Join(userHome, ".npmrc"), "//registry.npmjs.org/:_authToken="+tokenValue+"\n")
	env := map[string]string{"HOME": userHome, "USERPROFILE": userHome}
	o := Options{Getenv: func(k string) string { return env[k] }, Cwd: base}
	return p, o, []string{tokenValue, keyBody}
}

func TestTheDatasetHoldsEveryRecordAndLinksThem(t *testing.T) {
	p, o, _ := home(t)
	ds, err := Build(p, o)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Version != Version || len(ds.Profiles) != 2 || len(ds.Hosts) != 3 {
		t.Fatalf("version %d, %d profiles, %d hosts", ds.Version, len(ds.Profiles), len(ds.Hosts))
	}
	keys := map[string]Key{}
	for _, k := range ds.Keys {
		keys[k.Ref] = k
	}
	for _, h := range ds.Hosts {
		k, ok := keys[h.KeyRef]
		if !ok {
			t.Errorf("host %s names key %q, which is not in the dataset", h.Alias, h.KeyRef)
			continue
		}
		found := false
		for _, a := range k.Hosts {
			found = found || a == h.Alias
		}
		if !found || k.Profile != h.Profile {
			t.Errorf("key %s does not link back to host %s", k.Ref, h.Alias)
		}
	}
	var simtabi Profile
	for _, pr := range ds.Profiles {
		if pr.Name == "simtabi" {
			simtabi = pr
		}
	}
	if simtabi.Code != "simbi" || !simtabi.OwnCode || simtabi.Hosts != 2 || simtabi.Keys != 2 || len(simtabi.Naming.Envs) != 2 {
		t.Errorf("simtabi = %+v", simtabi)
	}
	if len(ds.Tokens) != 1 || ds.Tokens[0].Target != "npm" || !ds.Tokens[0].SecretPresent {
		t.Errorf("tokens = %+v", ds.Tokens)
	}
}

// A server named by the rules carries its parts; one named before them
// carries none, and says so.
func TestAHostCarriesTheNamingPartsOfItsAlias(t *testing.T) {
	p, o, _ := home(t)
	ds, err := Build(p, o)
	if err != nil {
		t.Fatal(err)
	}
	hosts := map[string]Host{}
	for _, h := range ds.Hosts {
		hosts[h.Alias] = h
	}
	named := hosts["simbi-prd-web-00"]
	if !named.Named || named.Env != "prd" || named.Role != "web" || named.Code != "simbi" || named.Number != "00" {
		t.Errorf("named host = %+v", named)
	}
	if len(named.Options) != 1 || named.Options[0] != (Option{Name: "ServerAliveInterval", Value: "60"}) {
		t.Errorf("options = %+v", named.Options)
	}
	if older := hosts["oribi-web"]; older.Named || older.Env != "" {
		t.Errorf("older host = %+v", older)
	}
}

// No secret reaches the dataset: no type has a field for one, and the JSON
// of a home holding a token and a private key holds neither.
func TestTheDatasetNeverHoldsASecret(t *testing.T) {
	allowed := map[string]bool{"secret_present": true, "token_env": true}
	checked := 0
	var walk func(reflect.Type)
	seen := map[reflect.Type]bool{}
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Slice || rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := range rt.NumField() {
			f := rt.Field(i)
			name := strings.ToLower(f.Name + " " + f.Tag.Get("json"))
			checked++
			// A bool or a number cannot carry a secret's value (holds_private_keys
			// says whether a snapshot has keys; it holds none), so only fields
			// that can hold text are judged by their name.
			if k := f.Type.Kind(); k == reflect.Bool || (k >= reflect.Int && k <= reflect.Float64) {
				walk(f.Type)
				continue
			}
			for _, bad := range []string{"secret", "password", "private", "passphrase", "value_of", "token_value"} {
				if strings.Contains(name, bad) && !allowed[strings.Split(f.Tag.Get("json"), ",")[0]] {
					t.Errorf("%s.%s may hold a secret (%q)", rt.Name(), f.Name, bad)
				}
			}
			walk(f.Type)
		}
	}
	walk(reflect.TypeOf(Dataset{}))
	if checked < 50 {
		t.Fatalf("checked %d fields; the walk stopped early", checked)
	}

	p, o, secrets := home(t)
	ds, err := Build(p, o)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range secrets {
		if strings.Contains(string(b), s) {
			t.Fatal("a secret from the home reached the dataset's JSON")
		}
	}
	if strings.Contains(string(b), "PRIVATE KEY") {
		t.Fatal("private key material reached the dataset's JSON")
	}
}

func TestAHomeWithNoManifestSaysSo(t *testing.T) {
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	if _, err := Build(p, Options{Getenv: func(string) string { return "" }}); err == nil || !strings.Contains(err.Error(), "authama init") {
		t.Fatalf("err = %v, want one saying to run authama init", err)
	}
}
