package structure

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

const (
	pubWork  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB/22QXk5vTmOo31cReZdmvraArThvJ6FLu4g9t8PFIF me@work\n"
	pubOther = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOxRs9PMY8J9exKFH+bcmN5kT1DXeC0ANdIPMVVVWJ4b x\n"
	pubMisc  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOhzOpq/ktNRWOq4VdBXXR2QzbrMNGPqtSS7pSgH8tKp misc\n"
)

func home(t *testing.T, manifestJSON string) paths.Paths {
	t.Helper()
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, ".ssh"), ConfigDir: filepath.Join(root, "config")}
	for _, d := range []string{p.SSHDir, p.ConfigDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if manifestJSON == "" {
		manifestJSON = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
		  "work":{"key_scope":"per_service","hosts":[{"alias":"gh","hostname":"github.com","user":"git"}]},
		  "home":{"key_scope":"per_service","hosts":[]}}}`
	}
	write(t, p.Manifest(), manifestJSON, 0o600)
	return p
}

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func pair(t *testing.T, priv, pub string) {
	t.Helper()
	write(t, priv, "placeholder for a private key: never read\n", 0o600)
	write(t, priv+".pub", pub, 0o644)
}

func byPath(ss []Suggestion) map[string]Suggestion {
	out := map[string]Suggestion{}
	for _, s := range ss {
		out[s.Path] = s
	}
	return out
}

func TestSuggestFindsEachKindAndNeverMoves(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_work"), pubWork)
	pair(t, filepath.Join(p.SSHDir, "id_misc"), pubMisc)
	pair(t, filepath.Join(p.SSHDir, "profiles", "work", "extra"), pubOther)
	pair(t, filepath.Join(p.SSHDir, "profiles", "ghost", "k"), pubOther)
	pair(t, filepath.Join(p.ConfigDir, "dropped_work"), pubWork)
	write(t, filepath.Join(p.SSHDir, "config"), "Host x\n  IdentityFile ~/.ssh/id_work\n", 0o600)

	ss, err := Suggest(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(ss)
	if s := got["~/.ssh/id_work"]; s.Kind != KindLoose || s.Profile != "work" ||
		s.Plan != "key ~/.ssh/id_work profile work" || len(s.InUse) != 1 || s.To != "~/.ssh/profiles/work/id_work" {
		t.Errorf("id_work: %+v", s)
	}
	if s := got["~/.ssh/id_misc"]; !s.NeedsProfile || s.Plan != "" {
		t.Errorf("id_misc should need a profile: %+v", s)
	}
	if s := got["~/.ssh/profiles/work/extra"]; s.Kind != KindUntracked || s.Plan != "key ~/.ssh/profiles/work/extra profile work" {
		t.Errorf("extra: %+v", s)
	}
	if s := got["~/.ssh/profiles/ghost/k"]; s.Kind != KindOrphan || !s.NeedsProfile {
		t.Errorf("orphan: %+v", s)
	}
	if s := got["config/dropped_work"]; s.Kind != KindConfigHome || s.Profile != "work" {
		t.Errorf("config home: %+v", s)
	}
	for _, s := range ss {
		if s.ID == "" || s.ID != ID(s.Kind, s.Path) {
			t.Errorf("unstable id: %+v", s)
		}
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "id_work")); err != nil {
		t.Fatal("Suggest moved a key")
	}
}

func TestReservedAndAdoptedKeysAreLeftAlone(t *testing.T) {
	p := home(t, `{"version":1,"profiles":{"work":{"key_scope":"per_service","hosts":[],
	  "keys":[{"name":"gh","path":"id_adopted"}]}},
	  "reserved_keys":{"id_kept":{"for":"later"}}}`)
	pair(t, filepath.Join(p.SSHDir, "id_adopted"), pubWork)
	pair(t, filepath.Join(p.SSHDir, "id_kept"), pubOther)
	ss, err := Suggest(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 0 {
		t.Fatalf("want nothing, got %+v", ss)
	}
}

func TestApplyMovesBacksUpDeclaresAndRepoints(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_work"), pubWork)
	write(t, filepath.Join(p.SSHDir, "config"), "Host x\n  IdentityFile \"~/.ssh/id_work\"\nHost y\n  IdentityFile ~/.ssh/id_work.pub\n", 0o600)
	moves, err := ParsePlan("# approved\nkey ~/.ssh/id_work profile work name laptop\n")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res, err := Apply(p, moves, &out)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(p.SSHDir, "profiles", "work", "laptop")
	for f, mode := range map[string]os.FileMode{dest: 0o600, dest + ".pub": 0o644} {
		fi, err := os.Stat(f)
		if err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: %v %v", f, fi, err)
		}
	}
	if fi, _ := os.Stat(filepath.Dir(dest)); fi.Mode().Perm() != 0o700 {
		t.Errorf("profile dir mode %v", fi.Mode())
	}
	if _, err := os.Lstat(filepath.Join(p.SSHDir, "id_work")); !os.IsNotExist(err) {
		t.Error("source still there")
	}
	b, _ := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if !strings.Contains(string(b), `IdentityFile "~/.ssh/profiles/work/laptop"`) ||
		!strings.Contains(string(b), "IdentityFile ~/.ssh/profiles/work/laptop.pub") {
		t.Errorf("config not repointed:\n%s", b)
	}
	if res.Backup == "" {
		t.Fatal("no backup")
	}
	if fi, err := os.Stat(filepath.Join(res.Backup, "ssh", "id_work")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("backup: %v %v", fi, err)
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.KeySpecFor(manifest.KeyRef{Profile: "work", KeyName: "laptop"}); !ok {
		t.Error("not declared")
	}
	audit, _ := os.ReadFile(p.AuditLog())
	if !strings.Contains(string(audit), "structure.move") {
		t.Errorf("no history entry: %s", audit)
	}
	ss, _ := Suggest(p)
	if len(ss) != 0 {
		t.Errorf("still suggested after the move: %+v", ss)
	}
}

func TestApplyRefusesBeforeTouchingAnything(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_a"), pubWork)
	pair(t, filepath.Join(p.SSHDir, "profiles", "work", "id_a"), pubOther)
	pair(t, filepath.Join(p.SSHDir, "id_b"), pubOther)
	for name, plan := range map[string]string{
		"dest taken":      "key ~/.ssh/id_b profile work\nkey ~/.ssh/id_a profile work",
		"unknown profile": "key ~/.ssh/id_b profile nobody",
		"two to one":      "key ~/.ssh/id_b profile work name z\nkey ~/.ssh/id_a profile home name z\nkey ~/.ssh/id_a profile work name z",
	} {
		moves, err := ParsePlan(plan)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := Apply(p, moves, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: applied", name)
		}
		if _, err := os.Stat(filepath.Join(p.SSHDir, "id_b")); err != nil {
			t.Fatalf("%s: moved id_b before refusing", name)
		}
	}
	if err := os.Symlink(filepath.Join(p.SSHDir, "id_b"), filepath.Join(p.SSHDir, "id_link")); err != nil {
		t.Fatal(err)
	}
	moves, _ := ParsePlan("key ~/.ssh/id_link profile work")
	if _, err := Apply(p, moves, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("symlink moved: %v", err)
	}
}

func TestParsePlanRefusesEscapes(t *testing.T) {
	for _, bad := range []string{
		"key ../x profile work", "key ~/.ssh/../x profile work", "key /etc/passwd profile work",
		"key config/a/b profile work", "key config/.env profile work", "rm -rf ~", "misplaced work",
		"key ~/.ssh/x profile work name", "", "# only a comment",
	} {
		if _, err := ParsePlan(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	mv, err := ParsePlan("profile work category team\nprofile home category -\nmisplaced work/gh")
	if err != nil || len(mv) != 3 || mv[0].Category != "team" || mv[1].Category != "" || mv[1].Line() != "profile home category -" {
		t.Fatalf("%+v %v", mv, err)
	}
}

func TestMisplacedKeyMovesToWhereTheManifestPutsIt(t *testing.T) {
	p := home(t, `{"version":1,"profiles":{"work":{"key_scope":"per_service","category":"team","hosts":[],
	  "keys":[{"name":"gh"}]}}}`)
	pair(t, filepath.Join(p.SSHDir, "profiles", "work", "gh"), pubWork)
	ss, _ := Suggest(p)
	s := byPath(ss)["~/.ssh/profiles/work/gh"]
	if s.Kind != KindMisplaced || s.Plan != "misplaced work/gh" || s.To != "~/.ssh/profiles/team/work/gh" {
		t.Fatalf("%+v", ss)
	}
	moves, _ := ParsePlan(s.Plan)
	if _, err := Apply(p, moves, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "team", "work", "gh.pub")); err != nil {
		t.Fatal(err)
	}
}

func TestConfigHomeKeysMoveOnApplyOnly(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.ConfigDir, "dropped"), pubWork)
	moves, _ := ParsePlan("key config/dropped profile home")
	if fx := ConfigHomeEffects(p, moves); len(fx) != 1 || !strings.Contains(fx[0], "~/.ssh/profiles/home/dropped") {
		t.Fatalf("effects %v", fx)
	}
	if err := MoveConfigHome(p, moves, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(p, moves, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "home", "dropped")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.ConfigDir, "dropped")); !os.IsNotExist(err) {
		t.Fatal("left in the config home")
	}
}

func TestCheckDismissAndNotify(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_work"), pubWork)
	now := time.Now()
	all, fresh, err := Check(p, now)
	if err != nil || len(all) != 1 || len(fresh) != 1 {
		t.Fatalf("%v %v %v", all, fresh, err)
	}
	if err := MarkNotified(p, fresh, now); err != nil {
		t.Fatal(err)
	}
	if _, fresh, _ = Check(p, now); len(fresh) != 0 {
		t.Fatalf("notified twice: %v", fresh)
	}
	if err := Dismiss(p, []string{all[0].ID}, false, now); err != nil {
		t.Fatal(err)
	}
	all, _, _ = Check(p, now)
	if !all[0].Dismissed {
		t.Fatal("not dismissed")
	}
	if got, missing := Find(all, []string{all[0].ID[:5], "zzzz"}); len(got) != 1 || len(missing) != 1 {
		t.Fatalf("find: %v %v", got, missing)
	}
	if fi, err := os.Stat(StateFile(p)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file: %v %v", fi, err)
	}
}

// A sandbox's ~/.ssh is not named .ssh; its config still says ~/.ssh/.
func TestRefsResolveInASandbox(t *testing.T) {
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, "ssh"), ConfigDir: filepath.Join(root, "config")}
	if got := expand(p, "~/.ssh/id_x"); got != filepath.Join(p.SSHDir, "id_x") {
		t.Fatalf("expand = %s", got)
	}
	write(t, filepath.Join(p.SSHDir, "config"), "Host a\n  IdentityFile ~/.ssh/id_x\n", 0o600)
	changed, err := rewriteRefs(p, filepath.Join(p.SSHDir, "id_x"), filepath.Join(p.SSHDir, "profiles", "w", "id_x"))
	if err != nil || len(changed) != 1 {
		t.Fatalf("%v %v", changed, err)
	}
}

// A profile directory that is a symlink is refused: making and chmodding it
// would land outside ~/.ssh (as the importer refuses).
func TestApplyRefusesASymlinkedProfileDir(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_work"), pubWork)
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(p.SSHDir, "profiles"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(p.SSHDir, "profiles", "work")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	moves, err := ParsePlan("key ~/.ssh/id_work profile work name laptop\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(p, moves, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("applied through a symlinked profile dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "id_work")); err != nil {
		t.Error("the key moved although the plan was refused")
	}
	if ents, _ := os.ReadDir(elsewhere); len(ents) != 0 {
		t.Errorf("wrote through the link: %v", ents)
	}
}

// A symlinked ~/.ssh/config (a dotfiles setup) is repointed through the link,
// which stays a link.
func TestApplyKeepsASymlinkedConfigALink(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_work"), pubWork)
	real := filepath.Join(t.TempDir(), "dotfiles-ssh-config")
	write(t, real, "Host x\n  IdentityFile ~/.ssh/id_work\n", 0o600)
	if err := os.Symlink(real, filepath.Join(p.SSHDir, "config")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	moves, err := ParsePlan("key ~/.ssh/id_work profile work name laptop\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(p, moves, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(p.SSHDir, "config")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the config is no longer a link: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), "~/.ssh/profiles/work/laptop") {
		t.Errorf("the linked file was not repointed:\n%s", b)
	}
}
