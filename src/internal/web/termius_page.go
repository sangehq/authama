package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/dataset"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/util/csvout"
	"github.com/sangehq/authama/src/internal/util/encode"
)

// termiusPage is /termius: everything `export hosts --format termius` and
// `ssh termius keys` do, on one page. The hosts are chosen by the export's
// own filters, the Groups column by --termius-groups, and the page shows the
// CSV as Termius will read it, what was left out and why, and the key to
// attach to each host. It only reads, so --read-only keeps it.
//
//	GET /termius?termius_groups=profile/env&include_git=1&profile=a&env=prd&include=web
//
// The file itself downloads from /exports/run with the same selection, so the
// page, the download and `export hosts` are one pipeline and one set of bytes.
type termiusPage struct{ s *Server }

// TermiusView is the /termius page.
type TermiusView struct {
	// Schemes are the group schemes, each with its Groups cell for one of
	// this home's hosts, so the choice shows what it does.
	Schemes    []TermiusScheme
	Groups     string // the scheme chosen
	IncludeGit bool
	// Facets are the filters the hosts can take (profile, env, role, tag,
	// status), each value with its host count and whether it is ticked.
	Facets           []TermiusFacet
	Include, Exclude string
	Filtered         bool // any filter or option differs from the default
	// Header and Rows are the CSV as written; CSV is its text.
	Header []string
	Rows   [][]string
	CSV    string
	// Chosen is how many hosts the filters kept; Written how many the file
	// holds (Chosen less Skipped).
	Chosen, Written int
	Skipped         []string
	// Keys are the hosts in the file, each with the key to attach in Termius.
	Keys []TermiusKeyRow
	// Jumps are hosts behind a ProxyJump: Termius needs a Host Chain for them.
	Jumps    []TermiusJump
	Download string // the file, from /exports/run
	Command  string // the same export at a terminal
	ClearCmd string // empties the clipboard after `ssh termius keys`
}

// TermiusScheme is one --termius-groups value.
type TermiusScheme struct {
	Value, Label, Help, Example string
	Checked                     bool
}

// TermiusFacet is one filter: its parameter, its heading and its values.
type TermiusFacet struct {
	Name, Label string
	Values      []TermiusFacetValue
}

// TermiusFacetValue is one value a filter can take.
type TermiusFacetValue struct {
	Value   string
	Rows    int
	Checked bool
}

// TermiusKeyRow is one host's key, as `ssh termius keys` lists it.
type TermiusKeyRow struct {
	Alias, Group, Label, Path string
	OnePassword               bool   // the key is in 1Password: there is no file
	Command                   string // copies the private key at a terminal
}

// TermiusJump is a host Termius reaches only through a jump host.
type TermiusJump struct{ Alias, Jump string }

// termiusSchemeText words each scheme for the page.
var termiusSchemeText = map[string][2]string{
	encode.TermiusGroupsProfile:            {"Profile", "One group per profile, as before."},
	encode.TermiusGroupsCategoryProfile:    {"Category, then profile", "Profiles under their category; an uncategorized profile stays at the top."},
	encode.TermiusGroupsProfileEnv:         {"Profile, then environment", "A subgroup per environment the naming rules read from the alias."},
	encode.TermiusGroupsCategoryProfileEnv: {"Category, profile, environment", "Both levels: the fullest tree."},
	encode.TermiusGroupsNone:               {"No groups", "Every host at the top level of Termius."},
}

// termiusFacets are the filters the page offers, in order, with the export
// flag each one is.
var termiusFacets = []struct{ Name, Label string }{
	{"profile", "Profiles"}, {"env", "Environments"}, {"role", "Roles"}, {"tag", "Tags"}, {"status", "Status"},
}

func (t termiusPage) handleTermius(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	groups := q.Get("termius_groups")
	if err := encode.CheckTermiusGroups(groups); err != nil {
		t.s.renderError(w, r, http.StatusBadRequest, "Bad request", err.Error())
		return
	}
	if groups == "" {
		groups = encode.TermiusGroupsProfile
	}
	v, err := t.view(q, groups)
	if err != nil {
		t.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	t.s.render(w, http.StatusOK, "termius", t.s.newView(r, "termius", "Termius import", v))
}

// termiusSelection is the hosts selection the query's filters make: the
// same Where `export hosts --profile … --env …` builds.
func termiusSelection(q url.Values) selection.Selection {
	sel := selection.Presets["hosts"]
	sel.Where = selection.Where{
		Profile: values(q, "profile"), Env: values(q, "env"), Role: values(q, "role"),
		Tag: values(q, "tag"), Status: values(q, "status"),
		Include: strings.TrimSpace(q.Get("include")), Exclude: strings.TrimSpace(q.Get("exclude")),
	}
	return sel
}

// values are a parameter's non-empty values, whether repeated (one per
// ticked box) or comma-separated (as a link or the command line gives them).
func values(q url.Values, name string) []string {
	var out []string
	for _, v := range q[name] {
		out = append(out, splitList(v)...)
	}
	return out
}

func (t termiusPage) view(q url.Values, groups string) (TermiusView, error) {
	home := t.s.opts.Engine.Home()
	m, err := manifest.Load(home.Manifest())
	if err != nil {
		return TermiusView{}, err
	}
	src := renderer.Source{M: m, P: renderer.ExportPathsFor(home.SSHDir), EmitUseKeychain: platform.EmitUseKeychain()}
	ds, err := dataset.Build(home, dataset.Options{Parts: selection.Parts(selection.GrainHost)})
	if err != nil {
		return TermiusView{}, err
	}
	sel := termiusSelection(q)
	includeGit := q.Get("include_git") == "1"
	f, err := encode.Get("termius")
	if err != nil {
		return TermiusView{}, err
	}
	opts := encode.Options{IncludeGit: includeGit, TermiusGroups: groups}
	exp, _, err := selection.HostExport(ds, sel, f, src, opts)
	if err != nil {
		return TermiusView{}, err
	}
	aliases, _, err := selection.HostRows(ds, sel, src)
	if err != nil {
		return TermiusView{}, err
	}
	hosts, _, err := src.ClientHosts(aliases, "Termius", false, includeGit)
	if err != nil {
		return TermiusView{}, err
	}
	v := TermiusView{Groups: groups, IncludeGit: includeGit, Include: sel.Where.Include, Exclude: sel.Where.Exclude,
		Chosen: len(aliases), Written: len(hosts), Skipped: exp.Skipped,
		ClearCmd: brand.Command + " ssh termius keys --clear"}
	if len(exp.Files) == 1 {
		v.CSV = string(exp.Files[0].Data)
		table, err := csvout.Parse(v.CSV)
		if err != nil {
			return TermiusView{}, err
		}
		v.Header, v.Rows = table.Header, table.Rows
	}
	for _, h := range hosts {
		k := TermiusKeyRow{Alias: h.Alias, Group: encode.TermiusGroup(h, groups), Label: renderer.TermiusKeyLabel(h.KeyFile),
			Path: strings.TrimSuffix(h.KeyFile, ".pub"), OnePassword: h.Agent != ""}
		if !k.OnePassword && k.Path != "" {
			k.Command = brand.Command + " ssh termius keys " + h.Alias + " --clear-after 60s"
		}
		v.Keys = append(v.Keys, k)
		if h.ProxyJump != "" {
			v.Jumps = append(v.Jumps, TermiusJump{Alias: h.Alias, Jump: h.ProxyJump})
		}
	}
	v.Schemes = termiusSchemes(hosts, groups)
	if v.Facets, err = termiusFacetsOf(ds, sel.Where); err != nil {
		return TermiusView{}, err
	}
	v.Filtered = includeGit || groups != encode.TermiusGroupsProfile || len(sel.Where.Profile)+len(sel.Where.Env)+
		len(sel.Where.Role)+len(sel.Where.Tag)+len(sel.Where.Status) > 0 || v.Include != "" || v.Exclude != ""
	if v.Download, err = termiusDownload(sel, includeGit, groups); err != nil {
		return TermiusView{}, err
	}
	v.Command = termiusCommand(sel.Where, includeGit, groups)
	return v, nil
}

// termiusSchemes are the schemes with an example each: the Groups cell of the
// host that shows the most levels, or of a made-up one when there is none.
func termiusSchemes(hosts []encode.Host, chosen string) []TermiusScheme {
	sample := encode.Host{Group: "simtabi", Category: "orgs", Env: "prd"}
	best := -1
	for _, h := range hosts {
		score := 0
		if h.Category != "" {
			score++
		}
		if h.Env != "" {
			score += 2
		}
		if score > best {
			sample, best = h, score
		}
	}
	out := make([]TermiusScheme, 0, len(encode.TermiusGroupSchemes))
	for _, s := range encode.TermiusGroupSchemes {
		text := termiusSchemeText[s]
		ex := encode.TermiusGroup(sample, s)
		if ex == "" {
			ex = "(no group)"
		}
		out = append(out, TermiusScheme{Value: s, Label: text[0], Help: text[1], Example: ex, Checked: s == chosen})
	}
	return out
}

// termiusFacetsOf is each filter's values over every host, with the ticked
// ones marked; a filter no host has a value for is left out.
func termiusFacetsOf(ds *dataset.Dataset, w selection.Where) ([]TermiusFacet, error) {
	all, err := selection.Facets(ds, selection.GrainHost)
	if err != nil {
		return nil, err
	}
	chosen := map[string][]string{"profile": w.Profile, "env": w.Env, "role": w.Role, "tag": w.Tag, "status": w.Status}
	var out []TermiusFacet
	for _, f := range termiusFacets {
		vals := all[f.Name]
		if len(vals) == 0 {
			continue
		}
		on := map[string]bool{}
		for _, c := range chosen[f.Name] {
			on[c] = true
		}
		tf := TermiusFacet{Name: f.Name, Label: f.Label}
		for _, fv := range vals {
			tf.Values = append(tf.Values, TermiusFacetValue{Value: fv.Value, Rows: fv.Rows, Checked: on[fv.Value]})
		}
		out = append(out, tf)
	}
	return out, nil
}

// termiusDownload is the file's address: the selection through /exports/run,
// as the Export page would ask for it.
func termiusDownload(sel selection.Selection, includeGit bool, groups string) (string, error) {
	b, err := json.Marshal(sel)
	if err != nil {
		return "", err
	}
	q := url.Values{"s": {base64.RawURLEncoding.EncodeToString(b)}, "format": {"termius"}}
	if includeGit {
		q.Set("include_git", "1")
	}
	if groups != encode.TermiusGroupsProfile {
		q.Set("termius_groups", groups)
	}
	return "/exports/run?" + q.Encode(), nil
}

// termiusCommand is the export the page shows, as one command line.
func termiusCommand(w selection.Where, includeGit bool, groups string) string {
	args := []string{brand.Command, "export", "hosts", "--format", "termius"}
	if groups != encode.TermiusGroupsProfile {
		args = append(args, "--termius-groups", groups)
	}
	if includeGit {
		args = append(args, "--include-git")
	}
	for _, f := range []struct {
		flag string
		vals []string
	}{{"--profile", w.Profile}, {"--env", w.Env}, {"--role", w.Role}, {"--tag", w.Tag}, {"--status", w.Status}} {
		if len(f.vals) > 0 {
			args = append(args, f.flag, shellArg(strings.Join(f.vals, ",")))
		}
	}
	if w.Include != "" {
		args = append(args, "--include", shellArg(w.Include))
	}
	if w.Exclude != "" {
		args = append(args, "--exclude", shellArg(w.Exclude))
	}
	return strings.Join(append(args, "--out", "termius-hosts.csv"), " ")
}

// shellArg quotes a value for a POSIX shell when it needs it.
func shellArg(s string) string {
	safe := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./,:@%+=", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
