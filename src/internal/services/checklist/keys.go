package checklist

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/snapshots"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// Groups a key's steps are shown under.
const (
	gWhere    = "Where it is"
	gTargets  = "Servers and accounts"
	gLocal    = "This machine"
	gElse     = "Elsewhere"
	gBackups  = "Backups"
	gSchedule = "Rotation"
)

// keyFacts is what every key's checklist reads once, rather than once per key.
type keyFacts struct {
	m           *manifest.Manifest
	inv         *inventory.Inventory
	revoked     inventory.Revocations
	retired     inventory.Retirements
	agent       map[string]bool
	agentOK     bool
	snapsWithPK []string // snapshots taken before private keys were excluded
	bundles     map[string][]string
	today       string
}

func (s *Service) facts() (*keyFacts, error) {
	m, err := manifest.Load(s.P.Manifest())
	if err != nil {
		return nil, err
	}
	inv, err := inventory.Load(s.P.Inventory())
	if err != nil {
		return nil, err
	}
	f := &keyFacts{m: m, inv: inv, today: s.Now().Format("2006-01-02")}
	// A record that does not parse is an error: read as empty, a revoked key
	// would be listed as in use with nothing left to do. A missing file is
	// no records, as the loaders say.
	if f.revoked, err = inventory.LoadRevocations(inventory.RevokedPath(s.P.Inventory())); err != nil {
		return nil, err
	}
	if f.retired, err = inventory.LoadRetirements(inventory.RetiredPath(s.P.Inventory())); err != nil {
		return nil, err
	}
	if s.AgentFingerprints != nil {
		f.agent, f.agentOK = s.AgentFingerprints()
	}
	for _, snap := range snapshots.List(s.P.SnapshotsDir()) {
		if snapshots.HoldsKeyMaterial(snap) {
			f.snapsWithPK = append(f.snapsWithPK, filepath.Base(snap))
		}
	}
	f.bundles = bundleContents(s.P.DistDir())
	return f, nil
}

// keyChecklists is every key in use, then every retired and revoked key.
func (s *Service) keyChecklists() ([]Checklist, error) {
	f, err := s.facts()
	if err != nil {
		return nil, err
	}
	rows, err := keysvc.New(f.m, f.inv, s.P.SSHDir).Rows("")
	if err != nil {
		return nil, err
	}
	var out []Checklist
	seen := map[string]bool{}
	for _, r := range rows {
		c := s.liveKey(f, r)
		seen[c.ID] = true
		out = append(out, c)
	}
	// Retired keys: kept in old/ after a rotation, and recorded in retired.json.
	for _, fp := range sortedKeys(f.retired) {
		id := "key:" + fp
		if seen[id] {
			continue
		}
		if _, revoked := f.revoked[fp]; revoked {
			continue // a retired key revoked later shows as revoked
		}
		seen[id] = true
		out = append(out, s.retiredKey(f, fp, f.retired[fp]))
	}
	// Revoked keys whose key is gone from the manifest but not from everywhere.
	for _, fp := range sortedKeys(f.revoked) {
		id := "key:" + fp
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, s.revokedGoneKey(f, fp, f.revoked[fp]))
	}
	return out, nil
}

func keyID(fp string, ref manifest.KeyRef) string {
	if fp != "" {
		return "key:" + fp
	}
	return "key:" + ref.String()
}

// liveKey is a key the manifest declares or a host uses.
func (s *Service) liveKey(f *keyFacts, r keysvc.Row) Checklist {
	c := Checklist{Kind: KindKey, ID: keyID(r.Fingerprint, r.Ref), Name: r.Ref.String(), Mode: InUse, Fingerprint: r.Fingerprint}
	ref := r.Ref.String()
	if rev, ok := f.revoked[r.Fingerprint]; ok && r.Fingerprint != "" {
		c.Mode, c.Since = Revoked, rev.RevokedOn
		c.Steps = s.revocationSteps(f, revocationInput{
			ref: ref, fp: r.Fingerprint, priv: r.PrivatePath, pubOnDisk: r.PublicOnDisk, privOnDisk: r.PrivateOnDisk,
			hosts: r.Hosts, vault: r.Vault, removed: rev.Removed, notRemoved: rev.NotRemoved, revoked: true,
		})
		return c
	}
	// Where the key is.
	switch {
	case r.Vault != "":
		c.Steps = append(c.Steps, Step{ID: "vault", Group: gWhere, State: Done,
			Title:  "Private key in the 1Password vault " + r.Vault,
			Detail: "Only the public half is on disk: " + r.PublicPath})
	case r.PrivateOnDisk && r.PublicOnDisk:
		st, detail := Done, r.PrivatePath
		// Mode bits on Unix, the file's ACL on Windows, where every file
		// reports -rw-rw-rw-.
		if !perms.PermsOK(r.PrivatePath, perms.PrivateKeyMode) {
			st, detail = Todo, r.PrivatePath+": others can read it"
		}
		c.Steps = append(c.Steps, Step{ID: "files", Group: gWhere, State: st, Title: "Key pair on disk, owner-only",
			Detail: detail, Fix: fixIf(st != Done, "authama doctor --fix")})
	default:
		c.Steps = append(c.Steps, Step{ID: "files", Group: gWhere, State: Todo, Title: "Key pair on disk",
			Detail: "Not minted yet, or a half is missing: " + r.PrivatePath, Fix: "authama ssh reconcile"})
	}
	if len(r.Hosts) == 0 {
		c.Steps = append(c.Steps, Step{ID: "hosts", Group: gWhere, State: Done, Title: "No host uses it",
			Detail: "Declared, or reserved, with no host wired to it."})
	} else {
		c.Steps = append(c.Steps, Step{ID: "hosts", Group: gWhere, State: Done,
			Title:  fmt.Sprintf("Used by %d host(s) in ~/.ssh/config", len(r.Hosts)),
			Detail: strings.Join(r.Hosts, ", ")})
	}
	// Each place it is installed.
	recorded := map[string]bool{}
	for _, d := range r.Deployments {
		recorded[d.Target] = true
		id := "target:" + d.Target
		switch {
		case byHand(d.Method):
			c.Steps = append(c.Steps, Step{ID: id, Group: gTargets, Manual: true,
				Title:  "Installed on " + d.Target + " (pasted by hand)",
				Detail: "authama cannot see this target; tick it once you have checked the key is there."})
		case d.Verified:
			c.Steps = append(c.Steps, Step{ID: id, Group: gTargets, State: Done,
				Title: "Installed and verified on " + d.Target, Detail: "via " + d.Method + dateSuffix(d.Date)})
		default:
			c.Steps = append(c.Steps, Step{ID: id, Group: gTargets, State: Todo,
				Title: "Installed on " + d.Target + ", login not verified", Fix: "authama ssh deploy " + ref + " " + d.Target})
		}
	}
	for _, h := range r.Hosts {
		if !recorded[h] {
			c.Steps = append(c.Steps, Step{ID: "target:" + h, Group: gTargets, State: Todo,
				Title: "Deploy to " + h, Detail: "A host uses this key, but authama has not installed it there.",
				Fix: "authama ssh deploy " + ref + " " + h})
		}
	}
	// When it is due.
	if r.ExpiresOn != "" {
		st, title := Done, "Not due for rotation until "+r.ExpiresOn
		if r.ExpiresOn <= f.today {
			st, title = Todo, "Due for rotation since "+r.ExpiresOn
		}
		c.Steps = append(c.Steps, Step{ID: "expiry", Group: gSchedule, State: st, Title: title,
			Fix: fixIf(st != Done, "authama ssh rotate "+ref)})
	}
	c.Steps = append(c.Steps, Step{ID: "copies", Group: gElse, Manual: true,
		Title:  "Every copy outside authama is known",
		Detail: "Termius, other machines, CI secrets, accounts the key was added to by hand: a rotation or revocation has to reach these too."})
	return c
}

// retiredKey is a key a rotation replaced. Its private key stays in old/ for
// rollback until the next rotation.
func (s *Service) retiredKey(f *keyFacts, fp string, ret inventory.Retirement) Checklist {
	c := Checklist{Kind: KindKey, ID: "key:" + fp, Name: ret.Key, Mode: Retired, Fingerprint: fp, Since: ret.RetiredOn}
	priv := ""
	if ref, ok := parseRef(ret.Key); ok {
		priv = filepath.Join(manifest.ArchiveDir(f.m, s.P.SSHDir, ref), ref.KeyName)
	}
	_, privErr := os.Lstat(priv)
	c.Steps = s.revocationSteps(f, revocationInput{
		ref: ret.Key, fp: fp, priv: priv, privOnDisk: priv != "" && privErr == nil, retired: true,
		removed: ret.Removed, notRemoved: ret.NotRemoved, byHand: ret.ByHand,
	})
	return c
}

// revokedGoneKey is a revoked key no longer in the manifest.
func (s *Service) revokedGoneKey(f *keyFacts, fp string, rev inventory.Revocation) Checklist {
	c := Checklist{Kind: KindKey, ID: "key:" + fp, Name: rev.Key, Mode: Revoked, Fingerprint: fp, Since: rev.RevokedOn}
	priv := ""
	if rec, ok := f.inv.Keys[fp]; ok && rec.Path != "" {
		priv = expandHome(rec.Path)
	}
	_, err := os.Lstat(priv)
	c.Steps = s.revocationSteps(f, revocationInput{
		ref: rev.Key, fp: fp, priv: priv, privOnDisk: priv != "" && err == nil, revoked: true,
		removed: rev.Removed, notRemoved: rev.NotRemoved,
	})
	return c
}

type revocationInput struct {
	ref, fp, priv         string
	privOnDisk, pubOnDisk bool
	hosts                 []string
	vault                 string
	removed, notRemoved   []string
	byHand                map[string]string
	revoked, retired      bool
}

// revocationSteps is everything that has to go for a key to be fully revoked
// or retired.
func (s *Service) revocationSteps(f *keyFacts, in revocationInput) []Step {
	var steps []Step
	for _, t := range in.removed {
		steps = append(steps, Step{ID: "removed:" + t, Group: gTargets, State: Done, Title: "Removed from " + t})
	}
	for _, t := range in.notRemoved {
		st := Step{ID: "removed:" + t, Group: gTargets, Title: "Remove from " + t,
			Detail: "authama could not remove it there; it can still log in until it is gone."}
		if url := in.byHand[t]; url != "" {
			st.Manual, st.Link = true, url
			st.Detail = "This target takes keys by hand: delete the old key where its keys are managed, then tick this."
		} else if in.revoked {
			st.State, st.Fix = Todo, "authama ssh key revoke "+in.ref
		} else {
			st.Manual = true
			st.Detail = "authama could not remove it after the rotation; remove the old key on " + t + ", then tick this."
		}
		steps = append(steps, st)
	}
	if in.revoked && len(in.hosts) > 0 {
		steps = append(steps, Step{ID: "hosts", Group: gWhere, State: Todo,
			Title:  fmt.Sprintf("%d host(s) still point at it", len(in.hosts)),
			Detail: strings.Join(in.hosts, ", ") + ": give them a new key, so they stop offering a revoked one.",
			Fix:    "authama ssh keygen " + in.hosts[0] + " --force"})
	}
	// The private key on this machine.
	switch {
	case in.vault != "":
		steps = append(steps, Step{ID: "vault", Group: gLocal, Manual: true,
			Title:  "1Password item archived or deleted",
			Detail: "authama never deletes a vault item. The key is the item named after it in the vault " + in.vault + "."})
	case in.privOnDisk:
		st := Step{ID: "files", Group: gLocal, State: Todo, Title: "Private key deleted from this machine", Detail: in.priv}
		if in.retired {
			st.Detail = in.priv + " is kept so `authama ssh rollback` can bring it back; it goes at the next rotation, or delete it once you will not roll back."
		} else {
			st.Fix = "authama ssh key delete " + in.ref + " --purge"
		}
		steps = append(steps, st)
	default:
		steps = append(steps, Step{ID: "files", Group: gLocal, State: Done, Title: "Private key deleted from this machine"})
	}
	switch {
	case !f.agentOK:
		steps = append(steps, Step{ID: "agent", Group: gLocal, State: Unknown, Title: "Not loaded in ssh-agent",
			Detail: "No ssh-agent answered, so authama could not check."})
	case f.agent[in.fp]:
		fix := "ssh-add -d " + in.priv
		if in.priv == "" {
			fix = "ssh-add -D"
		}
		steps = append(steps, Step{ID: "agent", Group: gLocal, State: Todo, Title: "Remove it from ssh-agent",
			Detail: "ssh-agent still holds it, so ssh can still offer it.", Fix: fix})
	default:
		steps = append(steps, Step{ID: "agent", Group: gLocal, State: Done, Title: "Not loaded in ssh-agent"})
	}
	// Backups that still hold it.
	var inBundles []string
	if ref, ok := parseRef(in.ref); ok {
		for _, b := range sortedKeys(f.bundles) {
			for _, entry := range f.bundles[b] {
				if strings.HasSuffix(entry, "/"+ref.KeyName) && strings.Contains(entry, "/"+ref.Profile+"/") {
					inBundles = append(inBundles, b)
					break
				}
			}
		}
	}
	if len(inBundles) > 0 {
		steps = append(steps, Step{ID: "bundles", Group: gBackups, Manual: true,
			Title:  fmt.Sprintf("%d encrypted bundle(s) that hold it dealt with", len(inBundles)),
			Detail: strings.Join(inBundles, ", ") + ": restoring one brings the key back. Delete them, or keep them knowing that."})
	} else {
		steps = append(steps, Step{ID: "bundles", Group: gBackups, State: Done, Title: "No encrypted bundle holds it"})
	}
	if len(f.snapsWithPK) > 0 {
		steps = append(steps, Step{ID: "snapshots", Group: gBackups, State: Todo,
			Title:  fmt.Sprintf("%d old snapshot(s) still hold private keys", len(f.snapsWithPK)),
			Detail: "Taken before snapshots left private keys out: " + strings.Join(f.snapsWithPK, ", "),
			Fix:    "authama ssh snapshots prune"})
	} else {
		steps = append(steps, Step{ID: "snapshots", Group: gBackups, State: Done, Title: "No snapshot holds private keys"})
	}
	steps = append(steps,
		Step{ID: "accounts", Group: gElse, Manual: true, Title: "Removed from accounts it was added to by hand",
			Detail: "GitHub, GitLab or a server panel where you pasted it yourself rather than through authama."},
		Step{ID: "copies", Group: gElse, Manual: true, Title: "Removed from every other copy",
			Detail: "Termius, other machines, CI secrets, password managers."})
	return steps
}

func byHand(method string) bool { return method == "manual" || method == "web-panel" }

func fixIf(cond bool, cmd string) string {
	if cond {
		return cmd
	}
	return ""
}

func dateSuffix(d *string) string {
	if d == nil || *d == "" {
		return ""
	}
	return ", " + *d
}

func parseRef(s string) (manifest.KeyRef, bool) {
	p, k, ok := strings.Cut(s, "/")
	if !ok || p == "" || k == "" {
		return manifest.KeyRef{}, false
	}
	return manifest.KeyRef{Profile: p, KeyName: k}, true
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, rest)
		}
	}
	return p
}

// bundleContents reads each bundle's .contents sidecar: the files it holds.
func bundleContents(dir string) map[string][]string {
	out := map[string][]string{}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.age.contents"))
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var lines []string
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			if l := strings.TrimSpace(sc.Text()); l != "" {
				lines = append(lines, l)
			}
		}
		out[strings.TrimSuffix(filepath.Base(m), ".contents")] = lines
	}
	return out
}

// agentFingerprints asks ssh-agent which keys it holds. ok is false when no
// agent answers (exit 2) or ssh-add is missing; an agent with no keys is ok.
func agentFingerprints() (map[string]bool, bool) {
	if os.Getenv("SSH_AUTH_SOCK") == "" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh-add", "-l", "-E", "sha256").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return map[string]bool{}, true // the agent holds no keys
		}
		return nil, false
	}
	fps := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, "SHA256:") {
				fps[f] = true
			}
		}
	}
	return fps, true
}
