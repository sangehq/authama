package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/change"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/snapshots"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

// Generated per run (internal/util/testcred), never written down.
var (
	credSecretA      = testcred.Secret("secreta")
	credSecretB      = testcred.Secret("secretb")
	credSecretParity = testcred.Secret("secretparity")
)

// Every change the web offers must leave the home exactly as the CLI command
// it stands for does. Each case runs the CLI in one home and the change, through
// Preview and Apply, in an identical second home, then compares every file
// either could have written.

func ptr[T any](v T) *T { return &v }

// twinHomes builds a home by running setup through the CLI, then copies it.
func twinHomes(t *testing.T, setup [][]string) (cli, web paths.Paths) {
	t.Helper()
	cli = editHome(t)
	for _, args := range setup {
		run(t, args...)
	}
	mutation.Release()
	base := t.TempDir()
	web = paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	copyTree(t, cli.SSHDir, web.SSHDir)
	copyTree(t, cli.ConfigDir, web.ConfigDir)
	return cli, web
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if fi.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, fi.Mode().Perm()); err != nil {
			return err
		}
		// Snapshots are ordered by modification time, so the copy keeps it.
		return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
	})
	if err != nil {
		t.Fatal(err)
	}
}

// stateOf reads the files a change may write, with the home's own paths
// replaced so two homes compare equal when they hold the same thing. Two mints
// of the same key differ in their bytes, so key blobs, fingerprints and private
// key contents compare by shape: the same keys, in the same places, recorded
// the same way.
func stateOf(t *testing.T, p paths.Paths) map[string]string {
	t.Helper()
	out := map[string]string{}
	if b, err := os.ReadFile(p.Manifest()); err == nil {
		out["manifest.json"] = string(b)
	}
	if b, err := os.ReadFile(p.Providers()); err == nil {
		out["providers.json"] = string(b)
	}
	out["inventory.json"] = inventoryShape(t, p)
	// Snapshot counts are compared for prune only (see the case): the web takes
	// a snapshot before every change it makes, where the CLI's config render,
	// providers --export and doctor --fix take none.
	out["snapshots"] = fmt.Sprint(len(snapshots.List(p.SnapshotsDir())))
	bundles, _ := filepath.Glob(filepath.Join(p.DistDir(), "authama-*.age"))
	out["bundles"] = fmt.Sprint(len(bundles))
	for _, f := range []string{p.Manifest(), p.Inventory()} {
		if fi, err := os.Stat(f); err == nil {
			out["mode "+filepath.Base(f)] = fi.Mode().Perm().String()
		}
	}
	_ = filepath.Walk(p.SSHDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(p.SSHDir, path)
		key := "ssh/" + filepath.ToSlash(rel)
		if !snapshots.Archivable(fi.Name()) {
			out[key] = "<present " + fi.Mode().Perm().String() + ">"
			return nil
		}
		b, _ := os.ReadFile(path)
		out[key] = volatile(strings.ReplaceAll(string(b), p.SSHDir, "<ssh>"))
		return nil
	})
	return out
}

var (
	keyBlob     = regexp.MustCompile(`(ssh-ed25519|ssh-rsa|ecdsa-sha2-[a-z0-9]+) [A-Za-z0-9+/=]{16,}`)
	fingerprint = regexp.MustCompile(`SHA256:[A-Za-z0-9+/=]{20,}`)
)

func volatile(s string) string {
	return fingerprint.ReplaceAllString(keyBlob.ReplaceAllString(s, "$1 <key>"), "<fingerprint>")
}

// inventoryShape is the inventory with its fingerprint keys dropped and its
// records in path order, so two homes holding different mints of the same
// keys compare equal.
func inventoryShape(t *testing.T, p paths.Paths) string {
	t.Helper()
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		return "unreadable: " + err.Error()
	}
	var recs []string
	for _, r := range inv.Keys {
		b, _ := json.Marshal(r)
		recs = append(recs, volatile(string(b)))
	}
	sort.Strings(recs)
	return strings.Join(recs, "\n")
}

func TestEveryWebChangeMatchesItsCommand(t *testing.T) {
	setup := [][]string{
		{"ssh", "profile", "add", "work"},
		{"ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git"},
		{"ssh", "host", "add", "work", "db", "-H", "db.example", "-u", "ops", "--tag", "prod"},
		{"ssh", "reconcile", "--yes", "--no-pin"},
		{"ssh", "key", "add", "work", "spare"},
		{"ssh", "host", "add", "work", "new01", "-H", "new01.example", "-u", "ops"},
	}
	// Changes that act outside the home are compared by what they did there:
	// each side gets its own stand-in docker and notifier, logging its calls.
	logs := map[string]string{}
	stubs := func(name, script string) func(t *testing.T, side string) {
		return func(t *testing.T, side string) {
			bin := t.TempDir()
			logs[side] = filepath.Join(bin, "calls.log")
			stubBin(t, bin, name, `printf '%s\n' "$*" >> "`+logs[side]+`"`+"\n"+script)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		}
	}
	// The web asks `docker context inspect` once per preview, so reads are
	// left out: what must match is every call that changes something.
	writes := func(side string) string {
		b, _ := os.ReadFile(logs[side])
		var keep []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if !strings.Contains(l, " inspect ") {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n")
	}
	sameCalls := func(t *testing.T) {
		a, b := writes("cli"), writes("web")
		if a == "" || a != b {
			t.Errorf("the two sides made different calls:\n--- cli\n%s--- web\n%s", a, b)
		}
	}
	needsAge := func(t *testing.T, _ string) {
		if !hasAge() {
			t.Skip("age not installed")
		}
		if os.Getenv("AUTHAMA_AGE_RECIPIENT") == "" {
			recipient, _ := ageIdentity(t)
			t.Setenv("AUTHAMA_AGE_RECIPIENT", recipient)
		}
	}
	// The token changes write outside the home. Each side gets its own token
	// files, through the variables npm and Composer themselves read, so the
	// two can be compared and neither reaches a real one.
	tokenFiles := map[string]string{}
	tokenSide := func(seed bool) func(t *testing.T, side string) {
		return func(t *testing.T, side string) {
			dir := t.TempDir()
			t.Setenv("NPM_CONFIG_USERCONFIG", filepath.Join(dir, ".npmrc"))
			t.Setenv("COMPOSER_HOME", filepath.Join(dir, "composer"))
			t.Setenv("GH_TOKEN", credSecretParity)
			tokenFiles[side] = dir
			if seed {
				body := "//registry.npmjs.org/:_authToken=" + credSecretA + "\n//npm.example.com/:_authToken=" + credSecretB + "\n"
				if err := os.WriteFile(filepath.Join(dir, ".npmrc"), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// sameTokenFiles checks the web side's file holds what the change was for
	// and lacks what it removed, then that both sides wrote the same bytes.
	sameTokenFiles := func(want, holds, lacks string) func(t *testing.T) {
		return func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(tokenFiles["web"], want))
			if err != nil || !strings.Contains(string(b), holds) || (lacks != "" && strings.Contains(string(b), lacks)) {
				t.Errorf("the web side's %s should hold %q and not %q (%v):\n%s", want, holds, lacks, err, b)
			}
			for _, rel := range []string{".npmrc", filepath.Join("composer", "auth.json")} {
				a, errA := os.ReadFile(filepath.Join(tokenFiles["cli"], rel))
				w, errW := os.ReadFile(filepath.Join(tokenFiles["web"], rel))
				if (errA == nil) != (errW == nil) || string(a) != string(w) {
					t.Errorf("%s differs:\n--- cli (%v)\n%s--- web (%v)\n%s", rel, errA, a, errW, w)
				}
			}
		}
	}
	notifier := stubs("terminal-notifier", "")
	if runtime.GOOS != "darwin" {
		notifier = stubs("notify-send", "")
	}
	// A VM host whose VM the (stubbed) hypervisor no longer lists.
	goneVM := func(p paths.Paths) {
		m, _ := manifest.Load(p.Manifest())
		prof := m.Profiles["work"]
		prof.Hosts = append(prof.Hosts, manifest.Host{Alias: "devbox", Hostname: "devbox", User: "dev",
			VM: &manifest.VMSpec{Hypervisor: "lima", Name: "gone"}})
		m.SetProfile("work", prof)
		_ = m.Save(p.Manifest())
	}
	limaLists := func(t *testing.T, side string) {
		stubs("limactl", `[ "$1" = list ] && printf '%s\n' '{"name":"default","status":"Stopped"}'`)(t, side)
		t.Setenv("AUTHAMA_VM_HYPERVISORS", "lima")
	}
	// One loose keypair, the same bytes in both homes.
	var looseKey, loosePub []byte
	loose := func(p paths.Paths) {
		if looseKey == nil {
			dir, err := os.MkdirTemp("", "loose")
			if err != nil {
				return
			}
			defer func() { _ = os.RemoveAll(dir) }()
			k := filepath.Join(dir, "k")
			if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "docker devbox", "-f", k).Run(); err != nil {
				return
			}
			looseKey, _ = os.ReadFile(k)
			loosePub, _ = os.ReadFile(k + ".pub")
		}
		_ = os.WriteFile(filepath.Join(p.SSHDir, "loose-vm"), looseKey, 0o600)
		_ = os.WriteFile(filepath.Join(p.SSHDir, "loose-vm.pub"), loosePub, 0o644)
	}
	// The schedule is installed into a throwaway home with a stubbed
	// scheduler, so the test never registers a real job.
	schedHomes := map[string]string{}
	schedSide := func(t *testing.T, side string) {
		dir := t.TempDir()
		schedHomes[side] = dir
		t.Setenv("HOME", dir)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
		for _, n := range []string{"launchctl", "systemctl", "crontab", "schtasks"} {
			stubBin(t, dir, n, "exit 0")
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	sameSchedule := func(t *testing.T) {
		list := func(side string) map[string]string {
			out := map[string]string{}
			_ = filepath.WalkDir(schedHomes[side], func(path string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && !strings.Contains(path, "launchctl") {
					rel, _ := filepath.Rel(schedHomes[side], path)
					b, _ := os.ReadFile(path)
					out[rel] = string(b)
				}
				return nil
			})
			return out
		}
		a, b := list("cli"), list("web")
		if len(a) != len(b) {
			t.Errorf("the two sides installed different files:\n--- cli %v\n--- web %v", a, b)
		}
		found := false
		for k, v := range a {
			if b[k] != v {
				t.Errorf("%s differs", k)
			}
			found = found || (strings.Contains(v, "sync") && strings.Contains(v, "--quiet") && strings.Contains(v, "--yes"))
		}
		if !found && runtime.GOOS != "windows" {
			t.Errorf("no installed file runs the sync: %v", a)
		}
	}
	cases := []struct {
		name  string
		cli   []string
		ch    change.Change
		pre   func(p paths.Paths)             // applied to both homes first
		env   func(t *testing.T, side string) // before each side runs
		check func(t *testing.T)              // after both have
	}{
		{"profile add", []string{"ssh", "profile", "add", "home", "--key-comment", "{profile} laptop"},
			change.ProfileAdd("home", "", "", "{profile} laptop"), nil, nil, nil},
		{"profile edit", []string{"ssh", "profile", "edit", "work", "--key-scope", "shared", "--key-name", "work_all"},
			change.ProfileEdit("work", ptr("shared"), ptr("work_all")), nil, nil, nil},
		{"profile delete", []string{"ssh", "profile", "delete", "work", "--yes"}, change.ProfileDelete("work"), nil, nil, nil},
		{"host add", []string{"ssh", "host", "add", "work", "web", "-H", "web.example", "-u", "deploy", "-p", "2222", "--tag", "a", "--tag", "b"},
			change.HostAdd("work", "web", change.HostInput{Hostname: ptr("web.example"), User: ptr("deploy"), Port: ptr(2222),
				Tags: &[]string{"a", "b"}}), nil, nil, nil},
		{"host edit", []string{"ssh", "host", "edit", "work", "db", "-u", "root", "--tag", ""},
			change.HostEdit("work", "db", change.HostInput{User: ptr("root"), Tags: &[]string{""}}), nil, nil, nil},
		{"host delete", []string{"ssh", "host", "delete", "work", "db", "--yes"}, change.HostDelete("work", "db"), nil, nil, nil},
		{"key delete", []string{"ssh", "key", "delete", "work/spare", "--yes"},
			change.KeyDelete(manifest.KeyRef{Profile: "work", KeyName: "spare"}), nil, nil, nil},
		{"vm add", []string{"ssh", "vm", "add", "work", "devbox", "--hypervisor", "parallels", "--vm", "Ubuntu", "-u", "dev", "--yes"},
			change.VMAdd("work", "devbox", "parallels", "Ubuntu", "dev"), nil, nil, nil},
		{"config render", []string{"ssh", "config", "render", "--yes"}, change.ConfigRender(), nil, nil, nil},
		{"reconcile", []string{"ssh", "reconcile", "--yes", "--no-pin"}, change.Reconcile(), nil, nil, nil},
		{"keygen", []string{"ssh", "keygen", "work", "--no-pin", "--yes"}, change.Keygen("work"), nil, nil, nil},
		{"key add", []string{"ssh", "key", "add", "work", "extra", "--type", "ed25519", "--host", "new01"},
			change.KeyAdd("work", "extra", "ed25519", "new01"), nil, nil, nil},
		// A missing .pub is rebuilt from its private key; the web derives it from
		// the real private key, since a preview's copy holds none.
		{"key regen-pub", []string{"ssh", "key", "regen-pub", "work", "--yes"},
			change.KeyRegenPub(paths.Paths{}, keysvc.Selection{Selectors: []string{"work"}}), func(p paths.Paths) {
				pubs, _ := filepath.Glob(filepath.Join(p.SSHDir, "profiles", "work", "*gh*.pub"))
				if len(pubs) == 0 {
					panic("no .pub for work's gh key to remove")
				}
				for _, f := range pubs {
					if err := os.Remove(f); err != nil {
						panic(err)
					}
				}
			}, nil, nil},
		// The Naming page: the manifest's rules, one profile's, the key
		// template (which keeps every existing key name) and a profile's code.
		{"naming set", []string{"ssh", "naming", "set", "--envs=prd,stg", "--enforce=warn", "--start=1"},
			change.NamingSet("", change.NamingFields{Envs: &[]string{"prd", "stg"}, Enforce: ptr("warn"), Start: ptr(1)}), nil, nil, nil},
		{"naming set --profile", []string{"ssh", "naming", "set", "--profile", "work", "--alias={code}-{role}-{nn}", "--code-fallback=profile"},
			change.NamingSet("work", change.NamingFields{Alias: ptr("{code}-{role}-{nn}"), CodeFallback: ptr("profile")}), nil, nil, nil},
		{"naming key-template", []string{"ssh", "naming", "key-template", "{profile}_{alias}_k-{algo}", "--yes"},
			change.NamingKeyTemplate("", "{profile}_{alias}_k-{algo}"), nil, nil, nil},
		{"profile code", []string{"ssh", "profile", "edit", "work", "--code=wk"}, change.ProfileCode("work", "wk"), nil, nil, nil},
		{"providers export", []string{"ssh", "providers", "--export"}, change.ProvidersExport(), nil, nil, nil},
		{"settings init", []string{"settings", "init"}, change.SettingsInit(false), nil, nil, nil},
		{"token init", []string{"token", "init", "--target", "npm"}, change.TokenInit([]string{"npm"}), nil, nil, nil},
		{"profile add in a category", []string{"ssh", "profile", "add", "lab", "--category", "clients"},
			change.ProfileAddIn("lab", "", "", "", "clients"), nil, nil, nil},
		{"profile key comment", []string{"ssh", "profile", "edit", "work", "--key-comment", "{profile} {date}"},
			change.ProfileKeyComment("work", "{profile} {date}"), nil, nil, nil},
		{"snapshots prune", []string{"ssh", "snapshots", "prune", "--keep", "2"}, change.SnapshotsPrune(2), nil, nil, nil},
		{"doctor fix", []string{"doctor", "--fix"}, change.DoctorFix(), func(p paths.Paths) {
			_ = os.Chmod(p.Manifest(), 0o644)
		}, nil, nil},
		{"host apply", []string{"ssh", "host", "apply", "work", "--host", "db db2.example:2200 root db", "--host", "cache cache.example ops", "--yes"},
			change.HostApply("work", []editor.HostSpec{{Alias: "db", Hostname: "db2.example", Port: 2200, User: "root", Role: "db"},
				{Alias: "cache", Hostname: "cache.example", User: "ops"}}, false, ""), nil, nil, nil},
		{"host apply, new profile", []string{"ssh", "host", "apply", "lab", "box", "box.example", "me", "--category", "clients", "--yes"},
			change.HostApply("lab", []editor.HostSpec{{Alias: "box", Hostname: "box.example", User: "me"}}, false, "clients"), nil, nil, nil},
		{"profile category", []string{"ssh", "profile", "edit", "work", "--category", "clients"},
			change.ProfileCategory("work", "clients"), nil, nil, nil},
		{"profile purge", []string{"ssh", "profile", "delete", "work", "--purge", "--yes"}, change.ProfilePurge("work"), nil, needsAge, nil},
		{"clean", []string{"ssh", "clean", "--yes"}, change.Clean(false), func(p paths.Paths) {
			inv, err := inventory.Load(p.Inventory())
			if err != nil {
				return
			}
			// The same record in both homes: spare's, pointed at a key no
			// manifest entry owns.
			for fp, rec := range inv.Keys {
				if strings.HasSuffix(rec.Path, "/spare") {
					rec.Path = "~/.ssh/profiles/work/ghost-ed25519"
					inv.Keys[fp] = rec
				}
			}
			_ = inv.Save(p.Inventory())
		}, nil, nil},
		{"bundle", []string{"ssh", "bundle", "--keep", "1"}, change.Bundle(1), nil, needsAge, nil},
		{"vm docker-context", []string{"ssh", "vm", "docker-context", "devbox"}, change.VMDockerContext("devbox"), func(p paths.Paths) {
			m, _ := manifest.Load(p.Manifest())
			prof := m.Profiles["work"]
			prof.Hosts = append(prof.Hosts, manifest.Host{Alias: "devbox", Hostname: "devbox", User: "dev",
				VM: &manifest.VMSpec{Hypervisor: "lima", Name: "default"}})
			m.SetProfile("work", prof)
			_ = m.Save(p.Manifest())
		}, stubs("docker", `[ "$2" != inspect ]`), sameCalls},
		{"snapshots restore", []string{"ssh", "snapshots", "restore", "ssh-", "--yes"}, change.SnapshotRestore("", "ssh-", 10), func(p paths.Paths) {
			f, _ := os.OpenFile(filepath.Join(p.SSHDir, "config"), os.O_APPEND|os.O_WRONLY, 0o600)
			_, _ = f.WriteString("# edited after the snapshot\n")
			_ = f.Close()
		}, nil, nil},
		{"notify test", []string{"ssh", "notify", "test"}, change.NotifyTest(), nil, notifier, sameCalls},
		{"audit notify", []string{"ssh", "audit", "--notify"}, change.AuditNotify(), nil, notifier, nil},
		{"token delete", []string{"token", "delete", "npm", "npm.example.com"},
			change.TokenDelete("npm", "npm.example.com"), nil, tokenSide(true), sameTokenFiles(".npmrc", credSecretA, "npm.example.com")},
		{"token apply", []string{"token", "apply", "--no-env-hint"}, change.TokenApply(), nil, tokenSide(false),
			sameTokenFiles(filepath.Join("composer", "auth.json"), credSecretParity, "")},
		{"vm sync", []string{"ssh", "vm", "sync", "--yes"}, change.VMSync(false), goneVM, limaLists, nil},
		{"vm adopt-key", []string{"ssh", "vm", "adopt-key", "loose-vm", "work", "devbox", "--hypervisor", "parallels", "--vm", "Ubuntu", "-u", "dev", "--yes"},
			change.VMAdoptKey("loose-vm", "work", "devbox", "parallels", "Ubuntu", "dev"), loose, nil, nil},
		{"vm schedule", []string{"ssh", "vm", "sync", "--install-schedule", "--yes"}, change.VMSchedule(true), nil, schedSide, sameSchedule},
		{"key reserve", []string{"ssh", "key", "reserve", "loose-vm", "--for", "devbox", "--note", "Docker VM, later", "--yes"},
			change.KeyReserve("loose-vm", "devbox", "Docker VM, later"), loose, nil, nil},
		{"key unreserve", []string{"ssh", "key", "unreserve", "loose-vm", "--yes"}, change.KeyUnreserve("loose-vm"), func(p paths.Paths) {
			loose(p)
			_ = editor.New(p).ReserveKey("loose-vm", manifest.ReservedKey{For: "devbox"})
		}, nil, nil},
	}
	if len(cases) != 41 {
		t.Fatalf("%d cases; every change the web offers needs one", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cliHome, webHome := twinHomes(t, setup)
			ch := tc.ch
			// A restore reads its snapshot from the home it restores, which
			// exists only now.
			if ch.Command == "snapshots restore" {
				ch = change.SnapshotRestore(webHome.SnapshotsDir(), "ssh-", 10)
			}
			// regen-pub derives from the private keys of the home it is built for.
			if ch.Command == "key regen-pub" {
				ch = change.KeyRegenPub(webHome, keysvc.Selection{Selectors: []string{"work"}})
			}
			if tc.pre != nil {
				tc.pre(cliHome)
				tc.pre(webHome)
			}
			mutation.Release()
			if tc.env != nil {
				tc.env(t, "cli")
			}
			if out, err := runIn(t, "", tc.cli...); err != nil && !errors.Is(err, errNotClean) {
				t.Fatalf("%v: %v\n%s", tc.cli, err, out)
			}
			mutation.Release()
			if tc.env != nil {
				tc.env(t, "web")
			}
			plan, err := change.Preview(webHome, ch)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Err != "" {
				t.Fatalf("the preview says it would fail: %s", plan.Err)
			}
			if _, _, err := change.Apply(webHome, ch, plan.Hash, 10, history.SourceWeb); err != nil {
				t.Fatalf("apply: %v", err)
			}
			if tc.check != nil {
				tc.check(t)
			}
			a, b := stateOf(t, cliHome), stateOf(t, webHome)
			var names []string
			for k := range a {
				names = append(names, k)
			}
			for k := range b {
				if _, ok := a[k]; !ok {
					names = append(names, k)
				}
			}
			sort.Strings(names)
			if len(names) < 3 {
				t.Fatalf("compared %d files; the comparison is not reaching the homes", len(names))
			}
			for _, n := range names {
				if n == "snapshots" && tc.name != "snapshots prune" {
					continue
				}
				if a[n] != b[n] {
					t.Errorf("%s differs:\n--- cli\n%s\n--- web\n%s", n, a[n], b[n])
				}
			}
		})
	}
}
