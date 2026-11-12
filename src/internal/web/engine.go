package web

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/expiry"
	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/renderer"
	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/bundler"
	"github.com/simtabi/authama/src/internal/services/change"
	"github.com/simtabi/authama/src/internal/services/checklist"
	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keyaudit"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/notifier"
	"github.com/simtabi/authama/src/internal/services/overview"
	"github.com/simtabi/authama/src/internal/services/query"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/services/snapshots"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/util/highlight"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// ErrNotFound is a name that matches nothing in the manifest, the snapshots
// or the history. Handlers turn it into a 404.
var ErrNotFound = errors.New("not found")

// Engine is everything the pages read. Every argument is a name - a profile,
// a host alias, a key, a snapshot id, an operation id - resolved here against
// what exists, never a filesystem path. AppEngine is the real one; tests pass
// their own.
type Engine interface {
	Home() paths.Paths
	Overview(now time.Time) (Overview, error)
	Profiles() ([]ProfileView, error)
	Hosts() ([]query.ProfileGroup, error)
	Host(alias string) (HostView, error)
	Keys() (KeysView, error)
	Key(ref string) (KeyView, error)
	Snapshots() ([]SnapshotView, error)
	Bundles() ([]BundleView, error)
	SnapshotDiff(id, against string) (SnapshotDiff, error)
	History(f history.Filter) ([]history.Op, error)
	Op(id string) (OpView, error)
	Doctor() (doctor.Report, error)
	VMs(checkDocker bool) (VMsView, error)
	Audit(now time.Time) (AuditView, error)
	Export(name string, includeGit bool) (string, error)
	Tokens() (TokensView, error)

	// Preview runs a change on a private copy of the home; Apply makes it
	// if a fresh preview under the lock still has the confirmed hash.
	Preview(ch change.Change) (change.Plan, error)
	Apply(ch change.Change, hash string) (change.Plan, change.Result, error)
}

// SettingsView is the settings page: where the settings files are, every
// setting's value in effect and where it came from, and what `authama
// settings validate` reports. None of the settings is a secret: tokens live in
// .env and the credential files, and this view never reads their values.
type SettingsView struct {
	ConfigHome, ConfigSource string
	StateHome                string
	Dotenv, DotenvState      string
	Settings, SettingsState  string
	HasSettings              bool
	Effective                []SettingRow
	Problems                 []string
}

// SettingRow is one setting, as `authama settings show --effective` prints it.
type SettingRow struct{ Key, Variable, Value, Source string }

// Overview is the first page: the banner's rows, and the counts behind them.
type Overview struct {
	Rows     []overview.Row
	Problems int
	Profiles int
	Hosts    int
	Keys     int
	Expiry   []expiry.Status
	Recent   []history.Op
	DevRoot  string
}

// ProfileView is one profile in the profiles list.
type ProfileView struct {
	Name, Category, KeyScope, KeyStore string
	Hosts, Keys                        int
	// KeyComment is the profile's own key_comment template; empty inherits
	// defaults.key_comment.
	KeyComment string
}

// HostView is one host: the manifest entry and what it resolves to.
type HostView struct {
	Detail *query.HostDetail
	Block  string
	Pins   []knownhosts.Entry
	Token  string
	VM     bool // a VM host, which can have a Docker context
}

// KeyView is one key: its row, its detail, and its expiry.
type KeyView struct {
	Detail keysvc.Detail
	// PublicKey is the key's public key line, "" when there is none to show.
	PublicKey string
	// Revoked is the date the key was revoked, "" when it was not; DefaultDays
	// the lifetime a key without its own inherits.
	Revoked     string
	DefaultDays int
	Expiry      *expiry.Status
	Ops         []history.Op
	// Checklist is where the key is used and what is left to rotate or
	// revoke it; nil when it could not be worked out.
	Checklist *checklist.Checklist
}

// SnapshotView is one snapshot in the list.
type SnapshotView struct {
	ID         string
	Taken      time.Time
	Size       int64
	KeyBearing bool
}

// SnapshotDiff compares a snapshot with the tree now, or with another snapshot.
type SnapshotDiff struct {
	ID, Against string // Against "" means the current ~/.ssh
	Files       []FileDiff
	Refused     string // why the snapshot was not opened
}

// FileDiff is one file's change.
type FileDiff struct {
	Path  string
	State string // added | removed | changed
	Lines []history.DiffLine
	// Checked is set when the file's new content was validated (JSON, see
	// util/highlight.Check), and Problem holds what failed, if anything.
	Checked bool
	Problem string
}

// OpView is one operation and its manifest diff.
type OpView struct {
	Op       history.Op
	Diff     []history.DiffLine
	DiffNote string
}

// AuditView is the audit report, as data.
type AuditView struct {
	Keys   []AuditKey
	Expiry []expiry.Status
	Recent []history.Op
}

// AuditKey is one inventory record and where it is deployed.
type AuditKey struct {
	Path, Fingerprint, Status string
	Deployments               []inventory.Deployment
}

// AppEngine reads one authama home through the same services the CLI uses.
// It loads the manifest and inventory on every call: the CLI may change them
// at any moment, and a page must show what is there now.
type AppEngine struct {
	P paths.Paths
	// Retain is how many snapshots a change keeps, as the CLI's
	// $AUTHAMA_SNAPSHOT_RETAIN; zero means 10.
	Retain int
}

// Preview shows what ch would do, without doing it.
func (e AppEngine) Preview(ch change.Change) (change.Plan, error) { return change.Preview(e.P, ch) }

// Apply makes ch if it would still do what was confirmed.
func (e AppEngine) Apply(ch change.Change, hash string) (change.Plan, change.Result, error) {
	retain := e.Retain
	if retain < 1 {
		retain = 10
	}
	return change.Apply(e.P, ch, hash, retain, history.SourceWeb)
}

// Home is the paths this engine reads.
func (e AppEngine) Home() paths.Paths { return e.P }

func (e AppEngine) manifest() (*manifest.Manifest, error) {
	m, err := manifest.Load(e.P.Manifest())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no manifest at %s yet; run `authama init` first", e.P.Manifest())
		}
		return nil, err
	}
	return m, nil
}

func (e AppEngine) inventory() *inventory.Inventory {
	inv, err := inventory.Load(e.P.Inventory())
	if err != nil {
		return &inventory.Inventory{Keys: map[string]inventory.KeyRecord{}}
	}
	return inv
}

// Overview reads the state rows through overview.Rows, the one place that
// decides what needs attention and the command that fixes it.
func (e AppEngine) Overview(now time.Time) (Overview, error) {
	m, _ := manifest.Load(e.P.Manifest())
	ov := Overview{Rows: overview.Rows(e.P, m, now), DevRoot: e.P.DevRoot}
	for _, r := range ov.Rows {
		if r.Fix != "" {
			ov.Problems++
		}
	}
	if m != nil {
		ov.Profiles = len(m.ProfileNames())
		for _, n := range m.ProfileNames() {
			ov.Hosts += len(m.Profiles[n].Hosts)
		}
		if refs, err := m.KeyRefs(); err == nil {
			ov.Keys = len(refs)
		}
		if states, err := notifier.New(e.P, m).States(now); err == nil {
			for _, s := range states {
				if s.NeedsAttention() {
					ov.Expiry = append(ov.Expiry, s)
				}
			}
		}
	}
	ov.Recent, _ = e.History(history.Filter{Limit: 5})
	return ov, nil
}

// Profiles lists every profile with its counts.
func (e AppEngine) Profiles() ([]ProfileView, error) {
	m, err := e.manifest()
	if err != nil {
		return nil, err
	}
	refs, _ := m.KeyRefs()
	keys := map[string]int{}
	for _, r := range refs {
		keys[r.Profile]++
	}
	var out []ProfileView
	for _, n := range m.ProfileNames() {
		p := m.Profiles[n]
		store := p.KeyStore
		if store == "" {
			store = "disk"
		}
		out = append(out, ProfileView{Name: n, Category: p.Category, KeyScope: p.KeyScope, KeyStore: store,
			Hosts: len(p.Hosts), Keys: keys[n], KeyComment: p.KeyComment})
	}
	return out, nil
}

// Hosts lists every host, grouped by profile, as `authama ssh list` does.
func (e AppEngine) Hosts() ([]query.ProfileGroup, error) {
	m, err := e.manifest()
	if err != nil {
		return nil, err
	}
	return query.New(m, e.inventory(), e.P.Providers()).Groups("", "", "", "")
}

// Host is one host as `authama ssh show <alias>` reports it.
func (e AppEngine) Host(alias string) (HostView, error) {
	m, err := e.manifest()
	if err != nil {
		return HostView{}, err
	}
	for _, pname := range m.ProfileNames() {
		for _, h := range m.Profiles[pname].Hosts {
			if h.Alias != alias {
				continue
			}
			d, err := query.New(m, e.inventory(), e.P.Providers()).Detail(alias)
			if err != nil {
				return HostView{}, err
			}
			hd, ok := d.(*query.HostDetail)
			if !ok {
				// A profile and a host share the name; Detail prefers the profile.
				hd = &query.HostDetail{Profile: pname, Alias: h.Alias, Hostname: h.Hostname, User: h.User, Port: h.Port}
			}
			v := HostView{Detail: hd, Token: knownHostsToken(h), VM: h.VM != nil}
			if block, err := renderer.RenderHostBlockFor(m, pname, h); err == nil {
				v.Block = block
			}
			v.Pins = knownhosts.New(e.P.SSHDir).EntriesFor(v.Token)
			return v, nil
		}
	}
	return HostView{}, ErrNotFound
}

func knownHostsToken(h manifest.Host) string {
	if h.Port == 0 || h.Port == 22 {
		return h.Hostname
	}
	return fmt.Sprintf("[%s]:%d", h.Hostname, h.Port)
}

// KeysView is the /keys page: every key, as `authama ssh key list` shows
// them, the loose ~/.ssh keys kept on purpose, and the loose keys that could be.
type KeysView struct {
	Rows     []keysvc.Row
	Reserved []keyaudit.Reserved
	Loose    []keyaudit.LooseKey
	// Public is each key's public key line, by its ref, for the copy action:
	// only a line that is a public key, so a mislabelled file never reaches
	// the page.
	Public map[string]string
	// Profiles are the profiles that have a key, once each, in manifest order.
	Profiles []string
	// KeyRoot is the directory under ~/.ssh that holds the profile directories.
	KeyRoot string
	// Picker is the keys by profile, for choosing which to rotate, revoke or
	// delete; Categories are the categories profiles are filed under.
	Picker     []PickerProfile
	Categories []string
}

// pickerData is the key picker for one bulk action.
type pickerData struct {
	Action     string // rotate | revoke | delete
	Profiles   []PickerProfile
	Categories []string
	// All and Chosen are what the form already chose (every key; keys,
	// profiles and "category:<name>" values), so a review draws its picker
	// ticked.
	All    bool
	Chosen map[string]bool
}

// Picked reports whether the form already chose v: a key, a profile, or a
// "category:<name>".
func (d pickerData) Picked(v string) bool { return d.Chosen[v] }

// Why is "" when the action can act on k, and otherwise why it cannot.
func (d pickerData) Why(k PickerKey) string {
	switch {
	case d.Action == "rotate" && !k.Wired:
		return "no host uses it"
	case d.Action == "revoke" && !k.Recorded:
		return "never minted"
	case d.Action == "delete" && k.Wired:
		return "a host uses it"
	}
	return ""
}

// Eligible is how many of p's keys the action can act on.
func (d pickerData) Eligible(p PickerProfile) int {
	n := 0
	for _, k := range p.Keys {
		if d.Why(k) == "" {
			n++
		}
	}
	return n
}

// InCategory is how many keys the action can act on in the profiles of
// category cat.
func (d pickerData) InCategory(cat string) int {
	n := 0
	for _, p := range d.Profiles {
		if p.Category == cat {
			n += d.Eligible(p)
		}
	}
	return n
}

// Total is how many keys the action can act on, in every profile.
func (d pickerData) Total() int {
	n := 0
	for _, p := range d.Profiles {
		n += d.Eligible(p)
	}
	return n
}

// PickerProfile is one profile's keys in the key picker.
type PickerProfile struct {
	Name     string
	Category string
	Keys     []PickerKey
}

// PickerKey is one key in the key picker: what it is, and whether a host uses
// it (a used key cannot be deleted) or it was minted (one never minted cannot
// be revoked).
type PickerKey struct {
	Ref      string
	Name     string
	Wired    bool
	Recorded bool
	Status   string
	// Type, Hosts and ExpiresOn are shown beside the name, so a key can be
	// told apart from its neighbours without leaving the picker.
	Type      string
	Hosts     []string
	ExpiresOn string
}

// Keys lists every key, as `authama ssh key list` does.
func (e AppEngine) Keys() (KeysView, error) {
	m, err := e.manifest()
	if err != nil {
		return KeysView{}, err
	}
	rows, err := keysvc.New(m, e.inventory(), e.P.SSHDir).Rows("")
	if err != nil {
		return KeysView{}, err
	}
	v := KeysView{Rows: rows, Public: map[string]string{}, KeyRoot: manifest.KeyRoot(m)}
	for _, r := range rows {
		if !slices.Contains(v.Profiles, r.Ref.Profile) {
			v.Profiles = append(v.Profiles, r.Ref.Profile)
		}
		if pub := publicKeyLine(r.PublicPath); pub != "" {
			v.Public[r.Ref.String()] = pub
		}
	}
	v.Reserved, v.Loose = e.reservedAndLoose(m)
	at := map[string]int{}
	for _, r := range rows {
		i, ok := at[r.Ref.Profile]
		if !ok {
			i = len(v.Picker)
			at[r.Ref.Profile] = i
			cat := m.Profiles[r.Ref.Profile].Category
			v.Picker = append(v.Picker, PickerProfile{Name: r.Ref.Profile, Category: cat})
			if cat != "" && !slices.Contains(v.Categories, cat) {
				v.Categories = append(v.Categories, cat)
			}
		}
		v.Picker[i].Keys = append(v.Picker[i].Keys, PickerKey{Ref: r.Ref.String(), Name: r.Ref.KeyName,
			Wired: r.Wired(), Recorded: r.Recorded && r.Fingerprint != "", Status: r.Status,
			Type: r.Type, Hosts: r.Hosts, ExpiresOn: r.ExpiresOn})
	}
	sort.Strings(v.Categories)
	return v, nil
}

// reservedAndLoose are the reserved keys (manifest and planned devboxes) and
// the loose keys that are neither reserved nor a leftover copy.
func (e AppEngine) reservedAndLoose(m *manifest.Manifest) ([]keyaudit.Reserved, []keyaudit.LooseKey) {
	cfg, _ := settings.LoadVMConfig(e.P.SettingsFile())
	rep, err := keyaudit.New(m, e.inventory(), e.P.SSHDir).WithPlanned(cfg.PlannedKeys()).Audit(false)
	if err != nil {
		return nil, nil
	}
	res, _ := keyaudit.Reservations(m, cfg.PlannedKeys())
	var loose []keyaudit.LooseKey
	for _, k := range keyaudit.MarkReserved(keyaudit.LooseKeysIn(e.P.SSHDir, manifest.KeyRootAt(e.P.Manifest())), res) {
		if k.CopyOf == "" && !k.Reserved {
			loose = append(loose, k)
		}
	}
	return rep.Reserved, loose
}

// Key is one key by its profile/key reference.
func (e AppEngine) Key(ref string) (KeyView, error) {
	m, err := e.manifest()
	if err != nil {
		return KeyView{}, err
	}
	refs, err := m.KeyRefs()
	if err != nil {
		return KeyView{}, err
	}
	for _, r := range refs {
		if r.String() != ref {
			continue
		}
		d, err := keysvc.New(m, e.inventory(), e.P.SSHDir).Detail(r)
		if err != nil {
			return KeyView{}, err
		}
		v := KeyView{Detail: d, PublicKey: publicKeyLine(d.PublicPath), DefaultDays: m.Defaults.RotateAfterDays}
		if revs, err := inventory.LoadRevocations(inventory.RevokedPath(e.P.Inventory())); err == nil && d.Fingerprint != "" {
			v.Revoked = revs[d.Fingerprint].RevokedOn
		}
		if states, err := notifier.New(e.P, m).States(time.Now()); err == nil {
			for i := range states {
				if states[i].Profile == r.Profile && states[i].KeyName == r.KeyName {
					v.Expiry = &states[i]
				}
			}
		}
		v.Ops, _ = e.History(history.Filter{Key: r.KeyName, Profile: r.Profile, Limit: 20})
		if c, err := checklist.New(e.P).Get(r.String()); err == nil {
			v.Checklist = &c
		}
		return v, nil
	}
	return KeyView{}, ErrNotFound
}

// Snapshots lists the snapshots, newest first.
func (e AppEngine) Snapshots() ([]SnapshotView, error) {
	list := snapshots.List(e.P.SnapshotsDir())
	out := make([]SnapshotView, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		s := list[i]
		v := SnapshotView{ID: filepath.Base(s), KeyBearing: snapshots.HoldsKeyMaterial(s)}
		if fi, err := os.Stat(s); err == nil {
			v.Size, v.Taken = fi.Size(), fi.ModTime()
		}
		out = append(out, v)
	}
	return out, nil
}

// BundleView is one encrypted bundle in the state home's dist/.
type BundleView struct {
	Name    string
	Written time.Time
	Size    int64
}

// Bundles lists the bundles in dist/, newest first.
func (e AppEngine) Bundles() ([]BundleView, error) {
	list := bundler.Sorted(e.P.DistDir())
	out := make([]BundleView, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		v := BundleView{Name: filepath.Base(list[i])}
		if fi, err := os.Stat(list[i]); err == nil {
			v.Size, v.Written = fi.Size(), fi.ModTime()
		}
		out = append(out, v)
	}
	return out, nil
}

// snapshotPath resolves a snapshot id to its file, only if it is one of the
// snapshots List returns. An id is never joined onto a path unchecked.
func (e AppEngine) snapshotPath(id string) (string, bool) {
	for _, s := range snapshots.List(e.P.SnapshotsDir()) {
		if filepath.Base(s) == id {
			return s, true
		}
	}
	return "", false
}

// SnapshotDiff compares snapshot id with another snapshot, or with ~/.ssh as
// it is now when against is "". A snapshot holding key material is refused,
// not opened: showing a diff would put the key on the page.
func (e AppEngine) SnapshotDiff(id, against string) (SnapshotDiff, error) {
	path, ok := e.snapshotPath(id)
	if !ok {
		return SnapshotDiff{}, ErrNotFound
	}
	d := SnapshotDiff{ID: id, Against: against}
	old, err := snapshots.Files(path)
	if errors.Is(err, snapshots.ErrHoldsKeyMaterial) {
		d.Refused = err.Error()
		return d, nil
	}
	if err != nil {
		return d, err
	}
	var cur map[string]string
	if against == "" {
		cur, err = snapshots.CurrentFiles(e.P.SSHDir)
	} else {
		other, ok := e.snapshotPath(against)
		if !ok {
			return d, ErrNotFound
		}
		cur, err = snapshots.Files(other)
		if errors.Is(err, snapshots.ErrHoldsKeyMaterial) {
			d.Refused = against + ": " + err.Error()
			return d, nil
		}
	}
	if err != nil {
		return d, err
	}
	d.Files = diffTrees(old, cur)
	return d, nil
}

func diffTrees(old, cur map[string]string) []FileDiff {
	names := map[string]bool{}
	for k := range old {
		names[k] = true
	}
	for k := range cur {
		names[k] = true
	}
	sorted := make([]string, 0, len(names))
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var out []FileDiff
	for _, n := range sorted {
		a, inOld := old[n]
		b, inCur := cur[n]
		switch {
		case inOld && !inCur:
			out = append(out, FileDiff{Path: n, State: "removed", Lines: history.Diff(a, "", 3)})
		case !inOld && inCur:
			out = append(out, withCheck(FileDiff{Path: n, State: "added", Lines: history.Diff("", b, 3)}, b))
		case a != b:
			out = append(out, withCheck(FileDiff{Path: n, State: "changed", Lines: history.Diff(a, b, 3)}, b))
		}
	}
	return out
}

// History reads the operation history without writing to it: a page view
// must not create the database or run the first import.
func (e AppEngine) History(f history.Filter) ([]history.Op, error) {
	s, err := e.openHistory()
	if err != nil || s == nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	return s.Query(f)
}

func (e AppEngine) openHistory() (*history.Store, error) {
	if _, err := os.Stat(e.P.HistoryDB()); err != nil {
		return nil, nil
	}
	s, err := history.Open(e.P.HistoryDB())
	if errors.Is(err, history.ErrNewerSchema) {
		return s, nil
	}
	return s, err
}

// Op is one operation and the manifest diff across it.
func (e AppEngine) Op(id string) (OpView, error) {
	s, err := e.openHistory()
	if err != nil {
		return OpView{}, err
	}
	if s == nil {
		return OpView{}, ErrNotFound
	}
	defer func() { _ = s.Close() }()
	op, ok, err := s.Get(id)
	if err != nil {
		return OpView{}, err
	}
	if !ok {
		return OpView{}, ErrNotFound
	}
	v := OpView{Op: op}
	switch {
	case op.ManifestBefore == "" || op.ManifestAfter == "":
		v.DiffNote = "No manifest was recorded for this operation."
	case op.ManifestBefore == op.ManifestAfter:
		v.DiffNote = "The manifest did not change."
	default:
		before, _ := s.Manifest(op.ManifestBefore)
		after, _ := s.Manifest(op.ManifestAfter)
		v.Diff = history.Diff(string(before), string(after), 3)
	}
	return v, nil
}

// Doctor runs the same checks as `authama doctor`, without --fix.
func (e AppEngine) Doctor() (doctor.Report, error) {
	m, _ := manifest.Load(e.P.Manifest())
	return doctor.New(e.P, m, platform.EmitUseKeychain()).Run(false), nil
}

// Audit is `authama ssh audit` as data: deployments, expiry, recent activity.
func (e AppEngine) Audit(now time.Time) (AuditView, error) {
	m, err := e.manifest()
	if err != nil {
		return AuditView{}, err
	}
	inv := e.inventory()
	var v AuditView
	fps := make([]string, 0, len(inv.Keys))
	for fp := range inv.Keys {
		fps = append(fps, fp)
	}
	sort.Slice(fps, func(i, j int) bool {
		a, b := inv.Keys[fps[i]], inv.Keys[fps[j]]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return fps[i] < fps[j]
	})
	for _, fp := range fps {
		rec := inv.Keys[fp]
		status := rec.DeployStatus()
		v.Keys = append(v.Keys, AuditKey{Path: rec.Path, Fingerprint: fp, Status: status, Deployments: rec.Deployments})
	}
	v.Expiry, _ = notifier.New(e.P, m).States(now)
	v.Recent, _ = e.History(history.Filter{Limit: 10})
	return v, nil
}

// splitRef parses "profile/key" from a URL.
func splitRef(s string) (string, bool) {
	prof, key, ok := strings.Cut(s, "/")
	return s, ok && prof != "" && key != "" && !strings.Contains(key, "/")
}

// Exports a page can download. Each is rendered as `authama ssh config export
// --flat` and `authama export hosts --format termius` render it, and none is
// written anywhere.
const (
	ExportFlat    = "ssh_config"
	ExportTermius = "termius-hosts.csv"
)

// Export renders one of the exports for download.
func (e AppEngine) Export(name string, includeGit bool) (string, error) {
	m, err := e.manifest()
	if err != nil {
		return "", err
	}
	ep := renderer.ExportPathsFor(e.P.SSHDir)
	switch name {
	case ExportFlat:
		return renderer.RenderFlatConfig(m, ep, platform.EmitUseKeychain())
	case ExportTermius:
		exp, err := renderer.RenderTermiusCSV(m, ep, renderer.TermiusOptions{IncludeGit: includeGit})
		if err != nil {
			return "", err
		}
		return exp.CSV, nil
	}
	return "", ErrNotFound
}

// TokensView is the token page: what is installed, secrets masked, and the
// checkup of the files that hold it.
type TokensView struct {
	Items    []tokens.Installed
	Findings []tokensvc.Finding
	Failed   bool
	Config   string // the token config `token apply` would read, or "" for the built-in default
	Legacy   bool
	// Status is `authama token status`: per tool its file, mode, and how many
	// entries are installed, configured and resolvable now.
	Status tokensvc.Status
	// HasConfig says the config home already holds a tokens.json, which
	// `token init` and `token import` never overwrite.
	HasConfig bool
	// Targets are the tools a token can be set for; Services the Composer
	// services the token form offers (those taking one secret).
	Targets  []string
	Services []TokenService
	// ConfigText is the config home's tokens.json, for the editor, when it
	// holds variable names only; ConfigRefused says why it is not shown
	// otherwise. A file that holds a value never reaches a page.
	ConfigText, ConfigRefused string
}

// TokenService is a Composer service and the kind of secret it takes.
type TokenService struct {
	Name, Mechanism string
	Basic           bool // http-basic: a user name and a password
}

// Tokens is `authama token list` and `authama token audit` as data.
// The secrets never leave the service: Inventory is asked not to reveal them,
// and the view drops them again in case it ever does.
func (e AppEngine) Tokens() (TokensView, error) {
	env := tokensvc.NewEnv(e.P, nil, nil, "")
	v := TokensView{Items: env.Inventory("", false)}
	for i := range v.Items {
		v.Items[i].Secret = ""
	}
	v.Findings, v.Failed = env.Checkup(tokens.NewSet(), "")
	if src, err := env.FindConfig(""); err == nil {
		v.Config, v.Legacy = src.Path, src.Legacy
	}
	if st, err := tokensvc.StatusOf(env, "", ""); err == nil {
		v.Status = st
	}
	_, err := os.Stat(e.P.Tokens())
	v.HasConfig = err == nil
	if v.HasConfig {
		v.ConfigText, v.ConfigRefused = editableTokenConfig(e.P.Tokens())
	}
	for _, t := range tokens.Targets() {
		if t != tokens.Composer {
			v.Targets = append(v.Targets, t)
		}
	}
	for name, mech := range tokens.LoadCatalogServices() {
		if tokens.IsTokenMechanism(mech) || mech == "http-basic" {
			v.Services = append(v.Services, TokenService{Name: name, Mechanism: mech, Basic: mech == "http-basic"})
		}
	}
	sort.Slice(v.Services, func(i, j int) bool { return v.Services[i].Name < v.Services[j].Name })
	return v, nil
}

// editableTokenConfig reads a tokens.json for the editor: its text when it
// holds names only, or why it is not shown.
func editableTokenConfig(path string) (text, refused string) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err.Error()
	}
	if fi.Size() > tokens.MaxConfigBytes {
		return "", "it is larger than the editor accepts"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err.Error()
	}
	if err := tokens.CheckNamesOnly(string(b)); err != nil {
		return "", err.Error()
	}
	return string(b), ""
}

// publicKeyLine reads an OpenSSH public key file for the page: its one line,
// when that line is a public key - a known type, base64, an optional comment -
// and "" otherwise. A .pub that holds a private key, or anything else, is
// never shown.
func publicKeyLine(path string) string {
	if path == "" {
		return ""
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 16<<10 {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(b))
	if strings.ContainsAny(line, "\n\r") || strings.Contains(line, "PRIVATE KEY") {
		return ""
	}
	if !isPublicKeyLine(line) {
		return ""
	}
	return line
}

// publicKeyTypes are the OpenSSH public key algorithms a .pub can name.
var publicKeyTypes = map[string]bool{
	"ssh-ed25519": true, "ssh-rsa": true, "ssh-dss": true,
	"ecdsa-sha2-nistp256": true, "ecdsa-sha2-nistp384": true, "ecdsa-sha2-nistp521": true,
	"sk-ssh-ed25519@openssh.com": true, "sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// isPublicKeyLine reports whether line is "<type> <base64> [comment]" with a
// known type whose blob, decoded, names that same type: the wire format's
// first field is the algorithm, as a length-prefixed string.
func isPublicKeyLine(line string) bool {
	f := strings.Fields(line)
	if len(f) < 2 || !publicKeyTypes[f[0]] {
		return false
	}
	blob, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil || len(blob) < 4 {
		return false
	}
	n := int(binary.BigEndian.Uint32(blob))
	return n == len(f[0]) && len(blob) >= 4+n && string(blob[4:4+n]) == f[0]
}

// withCheck validates the file's new content, where its kind can be checked.
func withCheck(d FileDiff, after string) FileDiff {
	d.Checked, d.Problem = highlight.Check(highlight.For(d.Path), after)
	return d
}
