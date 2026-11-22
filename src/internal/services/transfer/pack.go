// Package transfer is the Export / Import hub's export half: a pack of the
// files the home can be exported as, chosen by part, as a zip. The same
// pack is `authama export pack` and the web UI's Export / Import page. No part
// holds a secret: tokens go out by name and where they are read from, keys
// by path and fingerprint, and the encrypted full export (private keys and
// token values) is `setup export --encrypt`, made apart from a pack.
package transfer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/dataset"
	"github.com/sangehq/authama/src/internal/services/gitsign"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Part is one thing a pack can hold.
type Part struct {
	ID    string // what --include and the page's checkbox name it by
	Title string
	Files string // the file(s) it adds to the pack
	About string
}

// The parts, in the order a pack and the page list them.
var Parts = []Part{
	{"termius", "Hosts for Termius", "termius-hosts.csv", "The Termius CSV import, grouped as chosen. It carries no key: attach each with `ssh termius keys`."},
	{"ssh-clients", "Hosts for other SSH clients", "clients/", "The hosts for each client `export hosts --format` writes - a generic CSV, Windows Terminal, iTerm2, PuTTY, Remmina, SecureCRT, MobaXterm, Royal TS, Remote Desktop Manager, Tabby, mRemoteNG, Xshell, Blink, Prompt 3, WinSCP, FileZilla, Cyberduck, Transmit and an Ansible inventory, or only the clients chosen - with clients/NOTES.txt on finishing each import. No key is in any of them."},
	{"ssh-config", "SSH config, one file", "ssh_config", "Every host inline with absolute key paths, for a tool that cannot follow Include."},
	{"setup", "Setup document", "setup.json", "Profiles, hosts, key names, token names and settings: what `setup import` makes another machine from. No secret."},
	{"structure", "SSH folder structure", "structure.json", "Categories, profiles, where each key lives and which hosts use it, and the moves suggested now."},
	{"git-signing", "Git identities and signing", "git-signing.json, allowed_signers", "Each profile's git identity, signing key (public half) and directories, and the allowed_signers file."},
	{"tokens", "Tokens", "tokens.csv", "Each token's name, tool and where its value is read from - never the value."},
	{"hosts", "Hosts list", "hosts.csv", "Every host with its profile, key and status."},
	{"keys", "Keys list", "keys.csv", "Every key's path, fingerprint and expiry - never the private half."},
	{"profiles", "Profiles list", "profiles.csv", "Every profile with its category, key scope and counts."},
	{"deployments", "Deployments", "deployments.csv", "Where each key was deployed, and when."},
	{"history", "History", "history.csv", "Every change made, from the command line and the web."},
	{"events", "File events", "events.csv", "What the watcher saw change in ~/.ssh and the config home."},
	{"snapshots", "Snapshots", "snapshots.csv", "The snapshots kept, newest first."},
}

// Default is what a pack holds when no part is named: the files another tool
// or machine takes.
var Default = []string{"termius", "ssh-config", "setup", "structure", "git-signing", "tokens"}

// IDs are every part's ID, in order.
func IDs() []string {
	out := make([]string, len(Parts))
	for i, p := range Parts {
		out[i] = p.ID
	}
	return out
}

// Options choose what a pack holds.
type Options struct {
	Include       []string // part IDs; empty is Default
	TermiusGroups string   // the Termius Groups scheme (encode.TermiusGroupSchemes)
	IncludeGit    bool     // host files also list git hosts
	Clients       []string // the ssh-clients part's formats (ClientFormats); empty is every one
	Version       string
	Now           time.Time
}

// File is one file of a pack.
type File struct {
	Name string
	Body []byte
}

// Check refuses an unknown part or Termius groups scheme, and returns the
// parts in pack order without repeats.
func Check(o Options) ([]string, error) {
	want := o.Include
	if len(want) == 0 {
		want = Default
	}
	known := map[string]bool{}
	for _, p := range Parts {
		known[p.ID] = true
	}
	chosen := map[string]bool{}
	for _, id := range want {
		for _, one := range strings.Split(id, ",") {
			one = strings.TrimSpace(one)
			if one == "" {
				continue
			}
			if one == "all" {
				for _, p := range Parts {
					chosen[p.ID] = true
				}
				continue
			}
			if !known[one] {
				return nil, fmt.Errorf("no part %q: choose from %s, or all", one, strings.Join(IDs(), ", "))
			}
			chosen[one] = true
		}
	}
	if err := encode.CheckTermiusGroups(o.TermiusGroups); err != nil {
		return nil, fmt.Errorf("termius groups: %w", err)
	}
	if _, err := ChosenClients(o.Clients); err != nil {
		return nil, err
	}
	var out []string
	for _, p := range Parts {
		if chosen[p.ID] {
			out = append(out, p.ID)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("choose at least one part: %s", strings.Join(IDs(), ", "))
	}
	return out, nil
}

// Build makes the files of a pack, in part order, plus a README naming them.
func Build(p paths.Paths, o Options) ([]File, error) {
	ids, err := Check(o)
	if err != nil {
		return nil, err
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	var files []File
	for _, id := range ids {
		fs, err := build(p, id, o)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		files = append(files, fs...)
	}
	return append([]File{readme(ids, o)}, files...), nil
}

func build(p paths.Paths, id string, o Options) ([]File, error) {
	switch id {
	case "termius":
		body, err := hostFile(p, "termius", o)
		return []File{{"termius-hosts.csv", body}}, err
	case "ssh-clients":
		return clientFiles(p, o)
	case "ssh-config":
		m, err := manifest.Load(p.Manifest())
		if err != nil {
			return nil, err
		}
		text, err := renderer.RenderFlatConfig(m, renderer.ExportPathsFor(p.SSHDir), platform.EmitUseKeychain())
		return []File{{"ssh_config", []byte(text)}}, err
	case "setup":
		d, err := setupxfer.Build(p, o.Version, o.Now)
		if err != nil {
			return nil, err
		}
		body, err := d.Marshal()
		return []File{{"setup.json", body}}, err
	case "structure":
		body, err := structureJSON(p, o.Now)
		return []File{{"structure.json", body}}, err
	case "git-signing":
		return gitSigning(p)
	}
	sel, ok := selection.Presets[id]
	if !ok {
		return nil, fmt.Errorf("no part %q", id)
	}
	ds, err := dataset.Build(p, dataset.Options{Parts: selection.Parts(sel.Grain)})
	if err != nil {
		return nil, err
	}
	res, err := selection.Run(ds, sel)
	if err != nil {
		return nil, err
	}
	f, err := encode.Get("csv")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = f.Write(&buf, encode.Table{Columns: res.Columns, Rows: res.Rows}, encode.Options{})
	return []File{{id + ".csv", buf.Bytes()}}, err
}

// hostFile is the hosts in one of the SSH client formats, as `export hosts
// --format` writes it.
func hostFile(p paths.Paths, format string, o Options) ([]byte, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, err
	}
	sel := selection.Presets["hosts"]
	ds, err := dataset.Build(p, dataset.Options{Parts: selection.Parts(sel.Grain)})
	if err != nil {
		return nil, err
	}
	f, err := encode.Get(format)
	if err != nil {
		return nil, err
	}
	src := renderer.Source{M: m, P: renderer.ExportPathsFor(p.SSHDir), EmitUseKeychain: platform.EmitUseKeychain()}
	exp, _, err := selection.HostExport(ds, sel, f, src, encode.Options{IncludeGit: o.IncludeGit, TermiusGroups: o.TermiusGroups})
	if err != nil {
		return nil, err
	}
	return encode.Bundle(f, exp)
}

// ClientFormats are the SSH client formats the ssh-clients part writes, as
// `export hosts --format` names them; Termius has its own part.
var ClientFormats = []string{encode.ClientCSV, "windows-terminal", "iterm2", "putty", "remmina", "securecrt", "mobaxterm",
	"royalts", "rdm", "tabby", "mremoteng", "xshell", "blink", "prompt3", "winscp", "filezilla", "cyberduck", "transmit", "ansible"}

// ChosenClients is the client formats a pack's ssh-clients part writes, in
// ClientFormats order: every one when none is named. Each name may hold
// several, comma-separated, as --clients takes them.
func ChosenClients(names []string) ([]string, error) {
	want := map[string]bool{}
	for _, n := range names {
		for _, one := range strings.Split(n, ",") {
			one = strings.TrimSpace(one)
			if one == "" {
				continue
			}
			if one == "all" {
				return ClientFormats, nil
			}
			if !slices.Contains(ClientFormats, one) {
				return nil, fmt.Errorf("no client format %q: choose from %s, or all", one, strings.Join(ClientFormats, ", "))
			}
			want[one] = true
		}
	}
	if len(want) == 0 {
		return ClientFormats, nil
	}
	var out []string
	for _, f := range ClientFormats {
		if want[f] {
			out = append(out, f)
		}
	}
	return out, nil
}

// clientFiles is every client format's files under clients/<format>/, and
// clients/NOTES.txt with what each needs to finish its import and the hosts
// each left out.
func clientFiles(p paths.Paths, o Options) ([]File, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, err
	}
	sel := selection.Presets["hosts"]
	ds, err := dataset.Build(p, dataset.Options{Parts: selection.Parts(sel.Grain)})
	if err != nil {
		return nil, err
	}
	src := renderer.Source{M: m, P: renderer.ExportPathsFor(p.SSHDir), EmitUseKeychain: platform.EmitUseKeychain()}
	var files []File
	var notes strings.Builder
	notes.WriteString("Hosts for other SSH clients. No file here holds a private key: each names the key's path on this machine.\n")
	chosen, err := ChosenClients(o.Clients)
	if err != nil {
		return nil, err
	}
	for _, name := range chosen {
		f, err := encode.Get(name)
		if err != nil {
			return nil, err
		}
		exp, _, err := selection.HostExport(ds, sel, f, src, encode.Options{IncludeGit: o.IncludeGit, TermiusGroups: o.TermiusGroups})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		fmt.Fprintf(&notes, "\n%s/\n", name)
		for _, cf := range exp.Files {
			if cf.Name != filepath.Base(cf.Name) || strings.ContainsAny(cf.Name, `\:`) || cf.Name == ".." {
				return nil, fmt.Errorf("%s: unsafe file name %q", name, cf.Name)
			}
			files = append(files, File{"clients/" + name + "/" + cf.Name, cf.Data})
		}
		for _, n := range exp.Notes {
			fmt.Fprintf(&notes, "  %s\n", n)
		}
		for _, sk := range exp.Skipped {
			fmt.Fprintf(&notes, "  left out: %s\n", sk)
		}
	}
	return append(files, File{"clients/NOTES.txt", []byte(notes.String())}), nil
}

// StructureDoc is structure.json; fields are only ever added.
type StructureDoc struct {
	Version     int                    `json:"version"`
	Exported    string                 `json:"exported"`
	KeyRoot     string                 `json:"key_root"`
	Categories  []string               `json:"categories"`
	Profiles    []StructureProfile     `json:"profiles"`
	Suggestions []structure.Suggestion `json:"suggestions"`
}

// StructureProfile is one profile in structure.json.
type StructureProfile struct {
	Name     string          `json:"name"`
	Category string          `json:"category,omitempty"`
	Dir      string          `json:"dir"`
	KeyScope string          `json:"key_scope"`
	Store    string          `json:"key_store,omitempty"`
	Keys     []StructureKey  `json:"keys"`
	Hosts    []StructureHost `json:"hosts"`
}

// StructureKey is a declared key and where it lives (~/.ssh-relative).
type StructureKey struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Adopted bool   `json:"adopted,omitempty"`
}

// StructureHost is a host and the key it uses.
type StructureHost struct {
	Alias string `json:"alias"`
	Key   string `json:"key"`
}

func structureJSON(p paths.Paths, now time.Time) ([]byte, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, err
	}
	doc := StructureDoc{Version: 1, Exported: now.UTC().Format(time.RFC3339), KeyRoot: "~/.ssh/" + manifest.KeyRoot(m),
		Categories: m.Categories(), Profiles: []StructureProfile{}, Suggestions: []structure.Suggestion{}}
	keys := map[string][]StructureKey{}
	if refs, err := m.KeyRefs(); err == nil {
		for _, ref := range refs {
			rel, _ := filepath.Rel(p.SSHDir, m.KeyPath(p.SSHDir, ref))
			keys[ref.Profile] = append(keys[ref.Profile], StructureKey{Name: ref.KeyName,
				Path: "~/.ssh/" + filepath.ToSlash(rel), Adopted: manifest.AdoptedPath(m, ref) != ""})
		}
	}
	hosts := map[string][]StructureHost{}
	if res, err := m.IterResolved(); err == nil {
		for _, rk := range res {
			hosts[rk.Profile] = append(hosts[rk.Profile], StructureHost{Alias: rk.Host.Alias, Key: rk.KeyName})
		}
	}
	for _, name := range m.ProfileNames() {
		prof := m.Profiles[name]
		sp := StructureProfile{Name: name, Category: prof.Category, Dir: "~/.ssh/" + m.ProfileRel(name),
			KeyScope: string(prof.KeyScope), Keys: keys[name], Hosts: hosts[name]}
		if prof.InOnePassword() {
			sp.Store = "1password"
		}
		if sp.Keys == nil {
			sp.Keys = []StructureKey{}
		}
		if sp.Hosts == nil {
			sp.Hosts = []StructureHost{}
		}
		doc.Profiles = append(doc.Profiles, sp)
	}
	if ss, err := structure.Suggest(p); err == nil && ss != nil {
		doc.Suggestions = ss
	}
	return json.MarshalIndent(doc, "", "  ")
}

// GitSigningDoc is git-signing.json; fields are only ever added.
type GitSigningDoc struct {
	Version  int             `json:"version"`
	Profiles []GitSigningRow `json:"profiles"`
	Problems []string        `json:"problems"`
}

// GitSigningRow is one profile's git identity. The key is its public half.
type GitSigningRow struct {
	Profile     string   `json:"profile"`
	Email       string   `json:"email"`
	Name        string   `json:"name,omitempty"`
	Key         string   `json:"key"`
	PublicKey   string   `json:"public_key,omitempty"`
	Sign        bool     `json:"sign"`
	Dirs        []string `json:"dirs"`
	OnePassword bool     `json:"one_password,omitempty"`
}

func gitSigning(p paths.Paths) ([]File, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, err
	}
	signers, problems := gitsign.Signers(m, p.SSHDir)
	doc := GitSigningDoc{Version: 1, Profiles: []GitSigningRow{}, Problems: problems}
	if doc.Problems == nil {
		doc.Problems = []string{}
	}
	for _, s := range signers {
		dirs := s.Git.Dirs
		if dirs == nil {
			dirs = []string{}
		}
		doc.Profiles = append(doc.Profiles, GitSigningRow{Profile: s.Profile, Email: s.Git.Email, Name: s.Git.Name,
			Key: s.Key.Profile + "/" + s.Key.KeyName, PublicKey: s.PubLine, Sign: s.Git.Sign, Dirs: dirs, OnePassword: s.OnePassword})
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return []File{{"git-signing.json", body}, {"allowed_signers", []byte(gitsign.AllowedSigners(signers))}}, nil
}

func readme(ids []string, o Options) File {
	var b strings.Builder
	fmt.Fprintf(&b, "authama export pack, %s (authama %s)\n\n", o.Now.UTC().Format(time.RFC3339), o.Version)
	b.WriteString("No file here holds a private key or a token value.\n\n")
	byID := map[string]Part{}
	for _, p := range Parts {
		byID[p.ID] = p
	}
	for _, id := range ids {
		p := byID[id]
		fmt.Fprintf(&b, "%s - %s: %s\n", p.Files, p.Title, p.About)
	}
	if o.TermiusGroups != "" {
		fmt.Fprintf(&b, "\nTermius groups: %s\n", o.TermiusGroups)
	}
	b.WriteString("\nImport setup.json on another machine with `authama setup import setup.json`, or the zip itself.\n")
	return File{"README.txt", []byte(b.String())}
}

// Zip writes files as one zip, every member 0600, dated now.
func Zip(files []File, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: now}
		h.SetMode(0o600)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.Body); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Names is the files' names, sorted, for a report.
func Names(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Name
	}
	sort.Strings(out)
	return out
}
