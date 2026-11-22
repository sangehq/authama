package setupxfer

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

func home(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, "ssh"), ConfigDir: filepath.Join(root, "config"),
		StateRoot: filepath.Join(root, "config"), DevRoot: root}
	for _, d := range []string{p.SSHDir, p.ConfigDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func withProfiles(t *testing.T, p paths.Paths, names ...string) *manifest.Manifest {
	t.Helper()
	m := manifest.Starter(false)
	for _, n := range names {
		m.SetProfile(n, manifest.Profile{KeyScope: "per_service", Keys: []manifest.KeySpec{{Name: "main"}}})
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(p.Manifest()); err != nil {
		t.Fatal(err)
	}
	return m
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func secret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const tokensJSON = `{"composer":{"github.com":{"type":"github-oauth","env":"GH_TOKEN_TEST"}}}`

func TestDocumentRoundTripCarriesNamesNeverValues(t *testing.T) {
	src := home(t)
	withProfiles(t, src, "work", "home")
	write(t, src.Tokens(), tokensJSON)
	write(t, src.SettingsFile(), `{"version":1,"paths":{"state_dir":"/somewhere/only/here"},"backup":{"snapshot_retain":7}}`)
	value := secret(t)
	t.Setenv("GH_TOKEN_TEST", value)

	d, err := Build(src, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), value) {
		t.Fatal("the setup document holds a token value")
	}
	if strings.Contains(string(body), "/somewhere/only/here") {
		t.Fatal("the setup document holds a machine-specific path")
	}
	if strings.Join(d.EnvNames, ",") != "GH_TOKEN_TEST" || len(d.Keys) != 2 {
		t.Fatalf("names %v keys %v", d.EnvNames, d.Keys)
	}
	got, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}

	dst := home(t)
	res, err := Merge(dst, got, Keep)
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Counts(); c[StateAdd] == 0 || c[StateReplace] != 0 {
		t.Fatalf("into an empty home: %v\n%s", c, res.Format())
	}
	m, err := manifest.Load(dst.Manifest())
	if err != nil || len(m.Profiles) != 2 {
		t.Fatalf("manifest: %v %v", err, m)
	}
	for _, f := range []string{dst.Manifest(), dst.Tokens(), dst.SettingsFile()} {
		fi, err := os.Stat(f)
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", f, err, fi)
		}
	}
	if strings.Contains(read(t, dst.SettingsFile()), "state_dir") {
		t.Fatal("an import wrote a machine-specific path")
	}
	// A second import of the same document changes nothing.
	again, err := Merge(dst, got, Fail)
	if err != nil {
		t.Fatalf("the same document again, --on-conflict fail: %v", err)
	}
	if c := again.Counts(); c[StateAdd]+c[StateReplace]+c[StateKept] != 0 {
		t.Fatalf("a second import: %s", again.Format())
	}
}

func TestConflictPolicies(t *testing.T) {
	src := home(t)
	withProfiles(t, src, "work", "extra")
	write(t, src.SettingsFile(), `{"backup":{"snapshot_retain":7}}`)
	d, err := Build(src, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	dst := home(t)
	m := manifest.Starter(false)
	m.SetProfile("work", manifest.Profile{KeyScope: "per_service", Keys: []manifest.KeySpec{{Name: "other"}}})
	if err := m.Save(dst.Manifest()); err != nil {
		t.Fatal(err)
	}
	write(t, dst.SettingsFile(), `{"backup":{"snapshot_retain":3},"paths":{"state_dir":"/mine"}}`)
	before := read(t, dst.Manifest())

	if _, err := Merge(dst, d, Fail); !errors.As(err, &ConflictError{}) {
		t.Fatalf("fail: %v", err)
	}
	if read(t, dst.Manifest()) != before {
		t.Fatal("a failed import wrote the manifest")
	}

	res, err := Merge(dst, d, Keep)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := manifest.Load(dst.Manifest())
	if got.Profiles["work"].Keys[0].Name != "other" || got.Profiles["extra"].KeyScope == "" {
		t.Fatalf("keep: work %+v extra %+v\n%s", got.Profiles["work"], got.Profiles["extra"], res.Format())
	}
	if !strings.Contains(strings.ReplaceAll(read(t, dst.SettingsFile()), " ", ""), `"snapshot_retain":3`) {
		t.Fatalf("keep replaced a setting: %s", read(t, dst.SettingsFile()))
	}

	if _, err := Merge(dst, d, Replace); err != nil {
		t.Fatal(err)
	}
	got, _ = manifest.Load(dst.Manifest())
	settingsNow := strings.ReplaceAll(read(t, dst.SettingsFile()), " ", "")
	if got.Profiles["work"].Keys[0].Name != "main" || !strings.Contains(settingsNow, `"snapshot_retain":7`) {
		t.Fatalf("replace: %+v %s", got.Profiles["work"], settingsNow)
	}
	if !strings.Contains(settingsNow, "/mine") {
		t.Fatal("replace dropped this machine's own paths")
	}
}

func TestParseRefusals(t *testing.T) {
	for name, body := range map[string]string{
		"wrong format":   `{"format":"other","version":1}`,
		"newer version":  `{"format":"authama-setup","version":9}`,
		"claims secrets": `{"format":"authama-setup","version":1,"secrets":true}`,
		"unknown field":  `{"format":"authama-setup","version":1,"private_keys":"x"}`,
		"token value":    `{"format":"authama-setup","version":1,"tokens":{"composer":{"github.com":{"type":"github-oauth","env":"ghp_` + strings.Repeat("a", 36) + `"}}}}`,
		"bad manifest":   `{"format":"authama-setup","version":1,"manifest":{"profiles":{"../x":{}}}}`,
	} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestEnvMergeQuotesAndSettles(t *testing.T) {
	p := home(t)
	write(t, p.EnvFile(), "# mine\nKEEP_ME=1\nSHARED=\"old value\"\n")
	tricky := `va"l $HOME \ ` + "`x`"
	text, missing := EnvText([]string{"NEW_ONE", "SHARED", "ABSENT"}, func(n string) (string, bool) {
		switch n {
		case "NEW_ONE":
			return tricky, true
		case "SHARED":
			return "new value", true
		}
		return "", false
	})
	if strings.Join(missing, ",") != "ABSENT" {
		t.Fatalf("missing %v", missing)
	}
	plan, err := PlanEnv(p.EnvFile(), text, Keep)
	if err != nil || len(plan.Items) != 2 {
		t.Fatalf("plan %v %v", plan, err)
	}
	if strings.Contains(plan.Format(), "new value") || strings.Contains(plan.Format(), tricky) {
		t.Fatal("a plan shows a value")
	}
	backup := filepath.Join(p.StateDir(), "replaced", "x")
	if _, err := MergeEnv(p.EnvFile(), text, backup, Replace); err != nil {
		t.Fatal(err)
	}
	got := read(t, p.EnvFile())
	if !strings.HasPrefix(got, "# mine\nKEEP_ME=1\nSHARED=\"new value\"\n") {
		t.Fatalf(".env:\n%s", got)
	}
	if !strings.Contains(read(t, filepath.Join(backup, "config", ".env")), `"old value"`) {
		t.Fatal("the replaced .env was not kept")
	}
	// What was written reads back as the same value.
	text2, _ := EnvText([]string{"NEW_ONE"}, func(string) (string, bool) { return tricky, true })
	again, err := PlanEnv(p.EnvFile(), text2, Fail)
	if err != nil || again.Items[0].State != StateSame {
		t.Fatalf("quoted value did not read back: %v %v\n%s", err, again.Items, got)
	}
	if fi, _ := os.Stat(p.EnvFile()); fi.Mode().Perm() != 0o600 {
		t.Fatalf(".env mode %v", fi.Mode())
	}
}

func TestFullExportRoundTrip(t *testing.T) {
	src := home(t)
	m := withProfiles(t, src, "work")
	write(t, src.Tokens(), tokensJSON)
	key := m.KeyPath(src.SSHDir, manifest.KeyRef{Profile: "work", KeyName: "main"})
	privBody := "PRIVATE-" + secret(t)
	write(t, key, privBody)
	write(t, key+".pub", "ssh-ed25519 AAAA test\n")
	value := secret(t)
	t.Setenv("GH_TOKEN_TEST", value)
	pass := []byte(secret(t))

	out := filepath.Join(t.TempDir(), "setup.age")
	res, err := WriteFull(src, out, pass, "test", time.Now(), os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	raw := read(t, out)
	if strings.Contains(raw, value) || strings.Contains(raw, privBody) {
		t.Fatal("the full export holds plaintext")
	}
	if fi, _ := os.Stat(out); fi.Mode().Perm() != 0o600 {
		t.Fatalf("export mode %v", fi.Mode())
	}
	if _, err := WriteFull(src, out, pass, "test", time.Now(), os.LookupEnv); err == nil {
		t.Fatal("a full export replaced an existing file")
	}
	if len(res.Contents) == 0 {
		t.Fatal("no contents")
	}

	dst := home(t)
	if _, err := OpenFull(dst, out, []byte(secret(t)), "", false); err == nil {
		t.Fatal("opened with the wrong passphrase")
	}
	replaced := filepath.Join(dst.StateDir(), "replaced", "1")
	f, err := OpenFull(dst, out, pass, replaced, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Env, "GH_TOKEN_TEST=") {
		t.Fatal("no token values")
	}
	items, err := f.KeyItems(Keep)
	if err != nil || len(items.Items) != 2 || items.Items[0].State != StateAdd {
		t.Fatalf("keys %v %v", items, err)
	}
	if _, err := Merge(dst, f.Doc, Keep); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeInventory(dst, f.Inventory, Keep); err != nil {
		t.Fatal(err)
	}
	if _, err := f.LayKeys(Keep, func(string) (string, error) { return "", errors.New("n/a") }); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeEnv(dst.EnvFile(), f.Env, replaced, Keep); err != nil {
		t.Fatal(err)
	}
	dm, _ := manifest.Load(dst.Manifest())
	dkey := dm.KeyPath(dst.SSHDir, manifest.KeyRef{Profile: "work", KeyName: "main"})
	if read(t, dkey) != privBody {
		t.Fatal("the key was not laid down")
	}
	if fi, _ := os.Stat(dkey); fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", fi.Mode())
	}
	if fi, _ := os.Stat(filepath.Dir(dkey)); fi.Mode().Perm() != 0o700 {
		t.Fatalf("key dir mode %v", fi.Mode())
	}
	if !strings.Contains(read(t, dst.EnvFile()), value) {
		t.Fatal("the token value was not merged into .env")
	}

	// A second import where the key here differs: keep leaves it, replace
	// keeps a copy before replacing it.
	write(t, dkey, "LOCAL")
	f2, err := OpenFull(dst, out, pass, replaced, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.LayKeys(Fail, func(string) (string, error) { return "", nil }); err == nil {
		t.Fatal("fail laid down a differing key")
	}
	f3, _ := OpenFull(dst, out, pass, replaced, false)
	if _, err := f3.LayKeys(Keep, func(string) (string, error) { return "", nil }); err != nil || read(t, dkey) != "LOCAL" {
		t.Fatalf("keep: %v %q", err, read(t, dkey))
	}
	f4, _ := OpenFull(dst, out, pass, replaced, false)
	if _, err := f4.LayKeys(Replace, func(string) (string, error) { return "", nil }); err != nil || read(t, dkey) != privBody {
		t.Fatalf("replace: %v", err)
	}
	kept, err := filepath.Glob(filepath.Join(replaced, "ssh", "*", "*", "*"))
	if err != nil || len(kept) == 0 {
		var all []string
		_ = filepath.Walk(replaced, func(p string, _ os.FileInfo, _ error) error { all = append(all, p); return nil })
		if !strings.Contains(strings.Join(all, "\n"), "main") {
			t.Fatalf("the replaced key was not kept: %v", all)
		}
	}
}
