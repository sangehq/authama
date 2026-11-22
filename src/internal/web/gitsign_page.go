package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/gitremote"
	"github.com/sangehq/authama/src/internal/services/gitsign"
)

// gitSignPage is /git-sign: `authama ssh git sign` and `ssh git clone` in the
// browser. Each profile's git identity (email, name, the key that signs, its
// directories) is set through the change preview (action git-sign-set), which
// shows the manifest diff and every line of the git files it writes - the
// fragments, allowed_signers and the managed block of the global git config -
// before anything is written. `sign use` (git-sign-use) and `sign apply`
// (git-sign-apply) go the same way. Checking a checkout's signatures and
// working out a clone URL only read, so they are GET forms. No private key is
// read: the fragments and allowed_signers name public keys only.
type gitSignPage struct{ s *Server }

// GitSignView is the /git-sign page.
type GitSignView struct {
	NoManifest     bool
	NoIdentities   bool // no profile has one and none is written: nothing to apply
	Global         string
	UpToDate       bool
	Problems       []string
	Pending        []string
	Profiles       []GitSignProfile
	AllowedPath    string
	AllowedSigners string // public keys only
	Modes          []string
	DefaultMode    string

	// A checkout checked from the page (?check=).
	Check      string
	Checkout   *gitsign.CheckoutStatus
	Commits    []GitSignCommit
	CheckN     int
	CheckError string

	// Checkouts' remote modes read from the page (?remotes=), as `ssh git
	// status` shows them.
	Remotes      string
	RemoteRows   []gitremote.Checkout
	RemoteError  string
	RemoteUseCmd string

	// A clone worked out from the page (?repo=).
	Repo, RepoMode, RepoProfile string
	Clone                       *gitremote.Resolution
	CloneCmd                    string
	CloneError                  string

	StatusCmd, ApplyCmd, CloneExample string
}

// GitSignProfile is one profile's row and form.
type GitSignProfile struct {
	Name      string
	HasGit    bool
	Email     string
	UserName  string
	Key       string   // the key that signs (resolved)
	KeyChoice string   // git.signing_key as set ("" = the github.com host's key)
	Keys      []string // the profile's keys, for the select
	Sign      bool     // signing asked for
	Signs     bool     // asked for, and the key has a public half
	Dirs      []string
	DirsText  string
	Fragment  string
	Applied   bool // its fragment is written
	HasGitHub bool // a github.com host whose key signs by default
}

// GitSignCommit is one checked commit.
type GitSignCommit struct {
	gitsign.Commit
	Short, Meaning string
}

func (gp gitSignPage) handle(w http.ResponseWriter, r *http.Request) {
	v, err := gp.view(r.URL.Query())
	if err != nil {
		gp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	gp.s.render(w, http.StatusOK, "gitsign", gp.s.newView(r, "gitsign", "Git identities", v))
}

func (gp gitSignPage) view(q url.Values) (GitSignView, error) {
	p := gp.s.opts.Engine.Home()
	v := GitSignView{Modes: manifest.GitModes,
		StatusCmd:    brand.Command + " ssh git sign status",
		ApplyCmd:     brand.Command + " ssh git sign apply --dry-run",
		CloneExample: brand.Command + " ssh git clone <owner>/<repo>"}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		if os.IsNotExist(err) {
			v.NoManifest = true
			return v, nil
		}
		return v, err
	}
	v.DefaultMode = m.Defaults.Git.EffectiveMode()
	pl, err := gitsign.Render(p, m, nil)
	if err != nil {
		return v, err
	}
	v.Global, v.UpToDate, v.Problems = pl.Global, pl.UpToDate(), pl.Problems
	for _, w := range pl.Pending() {
		v.Pending = append(v.Pending, w.Path)
	}
	v.Pending = append(v.Pending, pl.Removes...)
	v.AllowedPath = gitsign.AllowedSignersPath(p)
	if b, err := os.ReadFile(v.AllowedPath); err == nil {
		v.AllowedSigners = string(b)
	}
	signers := map[string]gitsign.Signer{}
	for _, s := range pl.Signers {
		signers[s.Profile] = s
	}
	keys := map[string][]string{}
	if refs, err := m.KeyRefs(); err == nil {
		for _, ref := range refs {
			keys[ref.Profile] = append(keys[ref.Profile], ref.KeyName)
		}
	}
	for _, name := range m.ProfileNames() {
		prof := m.Profiles[name]
		row := GitSignProfile{Name: name, Keys: keys[name], Sign: true,
			Fragment: gitsign.FragmentPath(p, name)}
		if _, err := os.Stat(row.Fragment); err == nil {
			row.Applied = true
		}
		_, row.HasGitHub = manifest.GitHubHostOf(m, name)
		if !row.HasGitHub && row.KeyChoice == "" && len(row.Keys) > 0 && prof.Git == nil {
			row.KeyChoice = row.Keys[0] // no github.com host to default to
		}
		if g := prof.Git; g != nil {
			row.HasGit, row.Email, row.UserName, row.KeyChoice, row.Sign = true, g.Email, g.Name, g.SigningKey, g.Sign
			row.Dirs = g.Dirs
			row.DirsText = strings.Join(g.Dirs, "\n")
			if s, ok := signers[name]; ok {
				row.Key, row.Signs = s.Key.KeyName, s.Git.Sign && s.PubLine != ""
			}
		}
		v.Profiles = append(v.Profiles, row)
	}
	v.NoIdentities = !anyGit(m) && len(pl.Removes) == 0 && v.AllowedSigners == ""

	if dir := strings.TrimSpace(q.Get("check")); dir != "" {
		v.Check = dir
		v.CheckN = 10
		if n, err := strconv.Atoi(q.Get("n")); err == nil && n > 0 && n <= 200 {
			v.CheckN = n
		}
		dir = expandHome(dir)
		st := gitsign.Status(dir)
		v.Checkout = &st
		if st.Checkout {
			allowed := v.AllowedPath
			if _, err := os.Stat(allowed); err != nil {
				allowed = ""
			}
			commits, err := gitsign.Verify(dir, allowed, v.CheckN)
			if err != nil {
				v.CheckError = err.Error()
			}
			for _, c := range commits {
				short := c.Hash
				if len(short) > 10 {
					short = short[:10]
				}
				v.Commits = append(v.Commits, GitSignCommit{Commit: c, Short: short, Meaning: gitsign.Meaning(c.Status)})
			}
		}
	}

	if text := strings.TrimSpace(q.Get("remotes")); text != "" {
		v.Remotes = text
		if dirs, err := checkoutPaths(text); err != nil {
			v.RemoteError = err.Error()
		} else {
			v.RemoteRows = gitremote.Status(change.RemoteGit, m, dirs)
			v.RemoteUseCmd = brand.Command + " ssh git use <profile|ssh|https> " + strings.Join(dirs, " ")
		}
	}

	if repo := strings.TrimSpace(q.Get("repo")); repo != "" {
		v.Repo, v.RepoMode, v.RepoProfile = repo, q.Get("mode"), q.Get("profile")
		owner, name, ok := strings.Cut(strings.TrimSuffix(repo, ".git"), "/")
		mode := v.RepoMode
		if mode == "" {
			mode = v.DefaultMode
		}
		switch {
		case !ok || owner == "" || name == "" || strings.Contains(name, "/"):
			v.CloneError = "want owner/repo, as GitHub writes it"
		case !knownGitMode(mode):
			v.CloneError = "mode " + strconv.Quote(mode) + " is not one of " + strings.Join(manifest.GitModes, ", ")
		default:
			res, err := gitremote.Resolve(m, owner, name, mode, v.RepoProfile)
			if err != nil {
				v.CloneError = err.Error()
				break
			}
			v.Clone = &res
			cmd := brand.Command + " ssh git clone " + owner + "/" + name
			if v.RepoMode != "" {
				cmd += " --mode " + v.RepoMode
			}
			if v.RepoProfile != "" {
				cmd += " --profile " + v.RepoProfile
			}
			v.CloneCmd = cmd
		}
	}
	return v, nil
}

func anyGit(m *manifest.Manifest) bool {
	for _, prof := range m.Profiles {
		if prof.Git != nil {
			return true
		}
	}
	return false
}

func knownGitMode(mode string) bool {
	for _, m := range manifest.GitModes {
		if m == mode {
			return true
		}
	}
	return false
}

// gitSignSetChange is the git-sign-set action: one profile's identity, or
// (clear) none.
func gitSignSetChange(f url.Values, errs []error) (change.Change, []error) {
	profile, err := name(f, "profile")
	if err != nil {
		return change.Change{}, append(errs, err)
	}
	if checked(f, "clear") {
		return change.GitSignSet(profile, nil), errs
	}
	g := &manifest.ProfileGit{
		Email: strings.TrimSpace(f.Get("email")),
		Name:  strings.TrimSpace(f.Get("user_name")),
		Sign:  checked(f, "sign"),
	}
	if g.Email == "" {
		errs = append(errs, badForm("an email is needed: it is user.email, and the signer in allowed_signers"))
	}
	if k := strings.TrimSpace(f.Get("key")); k != "" {
		if !nameRe.MatchString(k) {
			errs = append(errs, badForm("key %q is not a key name", k))
		}
		g.SigningKey = k
	}
	for _, d := range strings.Split(f.Get("dirs"), "\n") {
		if d = strings.TrimSpace(d); d != "" {
			// git matches gitdir: with forward slashes on every system, and a
			// backslash would end the config value early (C:\x is C:/x).
			g.Dirs = append(g.Dirs, filepath.ToSlash(d))
		}
	}
	for _, s := range append([]string{g.Email, g.Name}, g.Dirs...) {
		if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			errs = append(errs, badForm("a field holds a control character"))
			break
		}
	}
	return change.GitSignSet(profile, g), errs
}

// gitSignUseChange is the git-sign-use action: checkouts, one per line, made
// to commit as the profile.
func gitSignUseChange(f url.Values, errs []error) (change.Change, []error) {
	profile, err := name(f, "profile")
	if err != nil {
		return change.Change{}, append(errs, err)
	}
	dirs, err := checkoutPaths(f.Get("checkouts"))
	if err != nil {
		return change.Change{}, append(errs, err)
	}
	return change.GitSignUse(profile, dirs), errs
}

// gitRemoteUseChange is the git-remote-use action: `ssh git use`, each
// checkout's origin pointed at a mode (empty: the default), through an
// optional profile.
func gitRemoteUseChange(f url.Values, errs []error) (change.Change, []error) {
	mode := strings.TrimSpace(f.Get("mode"))
	if mode != "" && !knownGitMode(mode) {
		errs = append(errs, badForm("mode %q is not one of %s", mode, strings.Join(manifest.GitModes, ", ")))
	}
	profile := strings.TrimSpace(f.Get("profile"))
	if profile != "" && !nameRe.MatchString(profile) {
		errs = append(errs, badForm("profile %q is not a profile name", profile))
	}
	dirs, err := checkoutPaths(f.Get("checkouts"))
	if err != nil {
		return change.Change{}, append(errs, err)
	}
	return change.GitRemoteUse(mode, profile, dirs), errs
}

// checkoutPaths reads checkouts, one full path (or ~/…) per line.
func checkoutPaths(text string) ([]string, error) {
	var dirs []string
	for _, d := range strings.Split(text, "\n") {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if strings.ContainsFunc(d, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return nil, badForm("a checkout path holds a control character")
		}
		if !strings.HasPrefix(d, "/") && !strings.HasPrefix(d, "~/") && (len(d) < 3 || d[1] != ':') {
			return nil, badForm("%q: give a checkout's full path (or ~/…); the server has no current directory of yours", d)
		}
		dirs = append(dirs, expandHome(d))
	}
	if len(dirs) == 0 {
		return nil, badForm("name at least one checkout")
	}
	return dirs, nil
}

// expandHome turns a leading ~/ into the home directory, for paths the server
// opens itself.
func expandHome(d string) string {
	if rest, ok := strings.CutPrefix(d, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return home + string(os.PathSeparator) + rest
		}
	}
	return d
}
