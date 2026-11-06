// Package repair finds what is wrong with an authama home, ~/.ssh and the
// token files, and fixes what can be fixed without guessing.
//
// It composes the services that already own each fix - doctor's permission
// rules, pubkeys' .pub regeneration, configsvc's render, the history store's
// integrity check - rather than doing any of them a second way. A finding it
// cannot fix safely (an unreadable manifest, a seal that no longer matches)
// is reported with the command that deals with it, never patched over.
//
// Nothing is deleted. A corrupt history database is renamed aside, and a new
// one is created and filled from audit.log the next time authama records.
package repair

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/services/configsvc"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/owner"
	"github.com/simtabi/authama/src/internal/services/pubkeys"
	"github.com/simtabi/authama/src/internal/services/snapshots"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/util/homeperms"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/util/perms"
)

// Area is what a finding concerns.
type Area string

const (
	Tool   Area = "tool"   // the config and state homes, the history
	SSH    Area = "ssh"    // ~/.ssh: modes, public keys, the rendered config
	Tokens Area = "tokens" // the tools' credential files
)

// Areas lists every area, in the order they are checked.
var Areas = []Area{Tool, SSH, Tokens}

// Finding is one thing wrong, and how it is put right.
type Finding struct {
	Area    Area
	Problem string
	// Fix says what Fix will do, when Fixable; otherwise what the person
	// should do, usually a command.
	Fix     string
	Fixable bool
	apply   func() error
}

// Home is what a repair works on.
type Home struct {
	P paths.Paths
	// Tokens resolves the tools' credential files; nil skips the token area.
	Tokens *tokensvc.Env
	// EmitUseKeychain is the rendered config's macOS UseKeychain setting, as
	// the command that renders it decides.
	EmitUseKeychain bool
	Now             func() time.Time
}

// Check finds what is wrong in the areas asked for (all when none are).
// It changes nothing.
func (h Home) Check(areas ...Area) []Finding {
	want := map[Area]bool{}
	for _, a := range areas {
		want[a] = true
	}
	all := len(want) == 0
	var out []Finding
	if all || want[Tool] {
		out = append(out, h.tool()...)
	}
	if all || want[SSH] {
		out = append(out, h.ssh()...)
	}
	if (all || want[Tokens]) && h.Tokens != nil {
		out = append(out, h.tokens()...)
	}
	return out
}

// Fix applies every fixable finding, in order, and reports what it did. It
// goes on past a fix that fails, so one bad file does not stop the rest, and
// returns the failures joined.
func Fix(findings []Finding, out io.Writer) error {
	var failed []error
	for _, f := range findings {
		if !f.Fixable {
			continue
		}
		if err := f.apply(); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", f.Problem, err))
			_, _ = fmt.Fprintf(out, "NOT fixed  [%s] %s: %v\n", f.Area, f.Problem, err)
			continue
		}
		_, _ = fmt.Fprintf(out, "fixed      [%s] %s: %s\n", f.Area, f.Problem, f.Fix)
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d fix(es) failed", len(failed))
	}
	return nil
}

func (h Home) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h Home) tool() []Finding {
	var out []Finding
	p := h.P
	for _, dir := range []string{p.ConfigDir, p.State(), p.StateDir(), p.LogDir()} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			d := dir
			out = append(out, Finding{Area: Tool, Problem: d + " is missing", Fix: "create it, owner-only", Fixable: true,
				apply: func() error { return os.MkdirAll(d, 0o700) }})
		}
	}
	if db := p.HistoryDB(); fileExists(db) {
		if err := history.QuickCheck(db); err != nil {
			stamp := h.now().UTC().Format("20060102-150405")
			aside := db + ".corrupt-" + stamp
			out = append(out, Finding{Area: Tool, Problem: "the history database fails its integrity check (" + err.Error() + ")",
				Fix: "rename it to " + filepath.Base(aside) + " (kept); a new one is filled from audit.log", Fixable: true,
				apply: func() error { return moveAside(db, aside) }})
		}
	}
	if fileExists(p.Manifest()) {
		if _, err := manifest.Load(p.Manifest()); err != nil {
			out = append(out, Finding{Area: Tool, Problem: "the manifest does not load: " + err.Error(),
				Fix: "fix it by hand, or put back an earlier one: `authama history` lists the changes that wrote it"})
		}
	}
	if fileExists(p.Inventory()) {
		if _, err := inventory.Load(p.Inventory()); err != nil {
			out = append(out, Finding{Area: Tool, Problem: "the inventory does not load: " + err.Error(),
				Fix: "fix it by hand; `authama ssh reconcile` records the keys on disk again"})
		}
	}
	if st := owner.Verify(p); !st.Clean() {
		out = append(out, Finding{Area: Tool, Problem: "the owner seal: " + st.Describe(),
			Fix: "see what changed with `authama owner verify`, then `authama owner reseal` if it was you"})
	}
	return out
}

func (h Home) ssh() []Finding {
	var out []Finding
	p := h.P
	for _, t := range snapshots.FindTempArtifactsIn(p.SSHDir, manifest.KeyRootAt(p.Manifest())) {
		tmp := t
		out = append(out, Finding{Area: SSH, Problem: "a crashed command left " + tmp, Fix: "remove it", Fixable: true,
			apply: func() error { return os.RemoveAll(tmp) }})
	}
	for _, mp := range append(perms.IterManagedPathsIn(p.SSHDir, manifest.KeyRootAt(p.Manifest())), homeperms.SecretPerms(p)...) {
		if fileExists(mp.Path) && !perms.PermsOK(mp.Path, mp.Mode) {
			m := mp
			out = append(out, Finding{Area: SSH, Problem: m.Path + " has a mode ssh or authama does not accept",
				Fix: fmt.Sprintf("set it to %o", uint32(m.Mode.Perm())), Fixable: true,
				apply: func() error { return perms.SetPerms(m.Path, m.Mode) }})
		}
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return out // reported under the tool area
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		return out
	}
	if rows, err := keysvc.New(m, inv, p.SSHDir).Select(keysvc.Selection{All: true}); err == nil {
		var missing []keysvc.Row
		for _, r := range rows {
			if r.PrivateOnDisk && !r.PublicOnDisk {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			rows := missing
			out = append(out, Finding{Area: SSH, Problem: fmt.Sprintf("%d key(s) have a private half and no .pub", len(rows)),
				Fix: "write each .pub from its private key", Fixable: true,
				apply: func() error {
					for _, r := range pubkeys.New(m, inv).Regenerate(rows, false) {
						if r.Action == pubkeys.Failed {
							return fmt.Errorf("%s: %s", r.Ref, r.Detail)
						}
					}
					return inv.Save(p.Inventory())
				}})
		}
	}
	svc := configsvc.New(p.SSHDir, m, h.EmitUseKeychain)
	if chk, err := svc.Check(false); err == nil && !chk.BlockInSync() {
		out = append(out, Finding{Area: SSH, Problem: "~/.ssh/config does not match the manifest",
			Fix: "render it again (what is outside authama's block is kept)", Fixable: true,
			apply: func() error { _, err := svc.Write(false); return err }})
	}
	return out
}

func (h Home) tokens() []Finding {
	var out []Finding
	// Windows keeps these files private with ACLs, not mode bits: a mode read
	// there says nothing about who can read the file.
	for _, t := range tokenTargetsWithModes() {
		path := h.Tokens.TargetPath(t, "")
		fi, err := os.Stat(path)
		if err != nil || fi.IsDir() {
			continue
		}
		if fi.Mode().Perm()&0o077 != 0 {
			f := path
			out = append(out, Finding{Area: Tokens, Problem: fmt.Sprintf("%s holds credentials and is readable by others (%o)", f, uint32(fi.Mode().Perm())),
				Fix: "set it to 600", Fixable: true, apply: func() error { return os.Chmod(f, 0o600) }})
		}
	}
	if cfg := h.P.Tokens(); fileExists(cfg) {
		if b, err := os.ReadFile(cfg); err == nil {
			if _, err := tokens.ParseConfig(string(b)); err != nil {
				out = append(out, Finding{Area: Tokens, Problem: "tokens.json does not parse: " + err.Error(),
					Fix: "fix it in the web UI's editor (it shows where), or by hand"})
			}
		}
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// moveAside renames a file and the SQLite sidecars beside it.
func moveAside(path, to string) error {
	if err := os.Rename(path, to); err != nil {
		return err
	}
	for _, side := range []string{"-wal", "-shm"} {
		if fileExists(path + side) {
			_ = os.Rename(path+side, to+side)
		}
	}
	return nil
}

// tokenTargetsWithModes is every token target whose file mode means
// something on this platform: all of them, except on Windows.
func tokenTargetsWithModes() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return tokens.Targets()
}
