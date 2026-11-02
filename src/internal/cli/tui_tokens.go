package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
)

// secretPrompter is the optional half of the prompter seam that reads a value
// without echoing it. The stdin prompter has it; a scripted test prompter
// does not, and then the secret is read as an ordinary line.
type secretPrompter interface {
	Secret(prompt string) (string, bool)
}

// Secret reads without echo at a terminal. Piped input has nothing to hide
// from, and must not be bypassed by a direct terminal read, so it is a line.
func (s *stdinPrompter) Secret(prompt string) (string, bool) {
	if !stdinIsTerminal() {
		return s.Line(prompt)
	}
	v, err := platform.ReadSecret(prompt)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(v), true
}

func (t tokenMenu) secret(prompt string) (string, bool) {
	if sp, ok := t.pr.(secretPrompter); ok {
		return sp.Secret(prompt)
	}
	return t.pr.Line(prompt)
}

func (t tokenMenu) tokenEnv() tokensvc.Env {
	return tokensvc.NewEnv(t.p, tokenGetenv, tokenLookup, "")
}

// tokenIO answers the docker credential-store question with the menu's own
// confirm, so a TUI write asks where the CLI would.
func (t tokenMenu) tokenIO() tokenIO {
	return tokenIO{out: t.out, errw: t.out, ask: t.pr.Confirm}
}

// tokenMenu holds the "Auth tokens" entries. It is its own type so the menu
// does not grow back into one type with every verb hung off it.
type tokenMenu struct{ *tui }

func (t *tui) tokenAction(handler string) { tokenMenu{t}.run(handler) }

func (t tokenMenu) run(handler string) {
	switch handler {
	case "tok_list":
		t.tokList()
	case "tok_audit":
		t.tokAudit()
	case "tok_copy":
		t.tokCopy()
	case "tok_set":
		t.tokSet()
	case "tok_apply":
		t.tokApply()
	case "tok_delete":
		t.tokDelete()
	case "tok_status":
		t.tokStatus()
	}
}

func (t tokenMenu) tokList() {
	items := t.tokenEnv().Inventory("", false)
	if len(items) == 0 {
		t.print("No tokens installed.")
		return
	}
	_ = writeTokenTable(t.out, items, false)
}

func (t tokenMenu) tokStatus() {
	st, err := buildTokenStatus(t.tokenEnv(), "", "")
	if err != nil {
		t.print("error: " + err.Error())
		return
	}
	_ = printTokenStatus(t.out, st)
}

func (t tokenMenu) tokAudit() {
	findings, _ := t.tokenEnv().Checkup(tokens.NewSet(), "")
	printFindings(t.out, findings)
}

// pickInstalled lists what is installed and returns the chosen one.
func (t tokenMenu) pickInstalled(message string) (tokens.Installed, bool) {
	items := t.tokenEnv().Inventory("", false)
	if len(items) == 0 {
		t.print("No tokens installed.")
		return tokens.Installed{}, false
	}
	labels := make([]string, 0, len(items)+1)
	byLabel := map[string]tokens.Installed{}
	for _, it := range items {
		l := it.Target + ": " + it.Host
		if it.Username != "" {
			l += " (" + it.Username + ")"
		}
		labels = append(labels, l)
		byLabel[l] = it
	}
	labels = append(labels, tuiBack)
	choice, ok := t.pr.Select(message, labels)
	if !ok || choice == tuiBack {
		return tokens.Installed{}, false
	}
	return byLabel[choice], true
}

func (t tokenMenu) tokCopy() {
	it, ok := t.pickInstalled("Copy which token")
	if !ok {
		return
	}
	found, ok := t.tokenEnv().Find(it.Target, it.Host, "")
	if !ok || found.Secret == "" {
		t.print(fmt.Sprintf("no token found for %s: %s", it.Target, it.Host))
		return
	}
	if err := clipboardWrite(found.Secret); err != nil {
		t.print("error: couldn't reach a clipboard tool (pbcopy/wl-copy/xclip/xsel/clip): " + err.Error())
		return
	}
	t.print(fmt.Sprintf("✓ copied %s: %s token to the clipboard", it.Target, it.Host))
}

func (t tokenMenu) tokDelete() {
	it, ok := t.pickInstalled("Delete which token")
	if !ok {
		return
	}
	if !t.pr.Confirm(fmt.Sprintf("Remove %s: %s from %s?", it.Target, it.Host, it.File)) {
		t.print("nothing changed")
		return
	}
	env := t.tokenEnv()
	lockForMutation(t.p)
	removed, err := env.Forget(it.Target, it.Host, "")
	switch {
	case err != nil:
		t.print("error: " + err.Error())
	case !removed:
		t.print(fmt.Sprintf("nothing removed — %s: %s not found", it.Target, it.Host))
	default:
		t.print(fmt.Sprintf("✓ removed %s: %s", it.Target, it.Host))
	}
}

// tokenProvider is one answer to "which provider": a Composer service or a
// catalog provider, with where it is hosted. As in auth-token-manager's
// menu, a provider with a cloud host that can also be self-hosted asks which;
// one with only a cloud host uses it; one with neither asks for the host.
type tokenProvider struct {
	label, mech, cloud, domains string
	hostable                    bool
}

func (p tokenProvider) choice() string {
	if p.cloud != "" {
		return p.label + "  (" + p.cloud + ")"
	}
	return p.label
}

// tokenProviders lists the providers for target, from the catalog.
func tokenProviders(target string) []tokenProvider {
	c, err := tokens.LoadCatalog()
	if err != nil {
		return nil
	}
	var out []tokenProvider
	if target == tokens.Composer {
		names := make([]string, 0, len(c.Services))
		for n := range c.Services {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			e, ent := c.Enterprise[n]
			out = append(out, tokenProvider{label: n, mech: c.Services[n], cloud: e.Cloud, domains: e.Domains, hostable: ent})
		}
		return out
	}
	for _, p := range c.Providers[target] {
		out = append(out, tokenProvider{label: p.Label, cloud: p.Cloud, hostable: p.Hostable})
	}
	return out
}

// normHost trims a typed host the way auth-token-manager's menu did: no
// scheme, no trailing slash.
func normHost(raw string) string {
	h := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, scheme := range []string{"https://", "http://"} {
		h = strings.TrimPrefix(h, scheme)
	}
	return h
}

func (t tokenMenu) resolveHost(p tokenProvider) (string, bool) {
	if p.cloud != "" && p.hostable {
		cloud := "Cloud  (" + p.cloud + ")"
		where, ok := t.pr.Select("Where is it hosted", []string{cloud, "Self-hosted / Enterprise (custom URL)", tuiBack})
		if !ok || where == tuiBack {
			return "", false
		}
		if where == cloud {
			return p.cloud, true
		}
	} else if p.cloud != "" {
		return p.cloud, true
	}
	raw, ok := t.pr.Line("Host / URL (e.g. git.acme.corp): ")
	if h := normHost(raw); ok && h != "" {
		return h, true
	}
	t.print("nothing saved: a host is required")
	return "", false
}

// tokenCredential asks for what target's mechanism needs. user is the
// installed user name when replacing an entry; Enter keeps it.
func (t tokenMenu) tokenCredential(target, mech, user string) (tokens.Credential, bool) {
	line := func(prompt string) (string, bool) {
		if user != "" {
			prompt = strings.TrimSuffix(prompt, ": ") + " [" + user + "]: "
		}
		v, ok := t.pr.Line(prompt)
		if v = strings.TrimSpace(v); ok && v == "" {
			v = user
		}
		return v, ok && v != ""
	}
	need := func(v string, ok bool, what string) bool {
		if !ok || v == "" {
			t.print("nothing saved: " + what + " is empty")
			return false
		}
		return true
	}
	switch {
	case target == tokens.NPM:
		scope, ok := t.pr.Line("npm scope to bind (optional, e.g. @org): ")
		if !ok {
			return tokens.Credential{}, false
		}
		s, ok := t.secret("Registry auth token: ")
		return tokens.Credential{Token: s, Scope: strings.TrimSpace(scope)}, need(s, ok, "the token")
	case target == tokens.Composer && tokens.IsTokenMechanism(mech):
		s, ok := t.secret("Personal access token: ")
		return tokens.Credential{Token: s}, need(s, ok, "the token")
	case target == tokens.Composer && mech == "bitbucket-oauth":
		k, ok := line("Consumer key: ")
		if !need(k, ok, "the consumer key") {
			return tokens.Credential{}, false
		}
		s, ok := t.secret("Consumer secret: ")
		return tokens.Credential{Key: k, Secret: s}, need(s, ok, "the consumer secret")
	case target == tokens.Composer && mech == "custom-headers":
		raw, ok := t.secret("Headers (comma-separated, e.g. 'API-TOKEN: xxx'): ")
		var hs []string
		for _, h := range strings.Split(raw, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hs = append(hs, h)
			}
		}
		return tokens.Credential{Headers: hs}, need(strings.Join(hs, ","), ok, "the header list")
	}
	label := "User name: "
	if target == tokens.Netrc {
		label = "Login: "
	}
	u, ok := line(label)
	if !need(u, ok, "the user name") {
		return tokens.Credential{}, false
	}
	s, ok := t.secret("Password / token: ")
	return tokens.Credential{Username: u, Password: s}, need(s, ok, "the password")
}

// tokSet adds a token, or replaces one that is installed: auth-token-manager's
// "Add a token" and "Edit a token" in one entry, since both are `token set`.
func (t tokenMenu) tokSet() {
	const fresh = "A new token"
	env := t.tokenEnv()
	var target, mech, host, user string
	var prov tokenProvider
	if items := env.Inventory("", false); len(items) > 0 {
		labels := []string{fresh}
		byLabel := map[string]tokens.Installed{}
		for _, it := range items {
			l := "Replace " + it.Target + ": " + it.Host
			if it.Username != "" {
				l += " (" + it.Username + ")"
			}
			labels = append(labels, l)
			byLabel[l] = it
		}
		choice, ok := t.pr.Select("Add a new token, or replace an installed one", append(labels, tuiBack))
		if !ok || choice == tuiBack {
			return
		}
		if it, ok := byLabel[choice]; ok {
			target, host, user = it.Target, it.Host, it.Username
			if target == tokens.Composer {
				mech = it.Kind
			}
		}
	}
	if target == "" {
		choices := append(tokens.Targets(), tuiBack)
		var ok bool
		if target, ok = t.pr.Select("Which tool", choices); !ok || target == tuiBack {
			return
		}
		provs := tokenProviders(target)
		labels := make([]string, 0, len(provs)+1)
		for _, p := range provs {
			labels = append(labels, p.choice())
		}
		pick, ok := t.pr.Select("Which provider", append(labels, tuiBack))
		if !ok || pick == tuiBack {
			return
		}
		for _, p := range provs {
			if p.choice() == pick {
				prov = p
			}
		}
		mech = prov.mech
		if host, ok = t.resolveHost(prov); !ok {
			return
		}
	}
	cr, ok := t.tokenCredential(target, mech, user)
	if !ok {
		return
	}
	set := tokens.NewSet()
	if !set.Apply(target, mech, host, cr) {
		t.print("nothing saved: the credential is incomplete")
		return
	}
	if !t.pr.Confirm(fmt.Sprintf("Save %s: %s to %s?", target, host, env.TargetPath(target, ""))) {
		t.print("nothing changed")
		return
	}
	lockForMutation(t.p)
	if err := writeTokenTargets(t.tokenIO(), env, set, []string{target}, "", "", nil); err != nil {
		t.print("error: " + err.Error())
		return
	}
	if prov.domains != "" && host != prov.cloud {
		t.print(fmt.Sprintf("note: add %s to %q in your composer.json \"config\" so Composer uses this token for the self-hosted instance.",
			host, prov.domains))
	}
}

func (t tokenMenu) tokApply() {
	env := t.tokenEnv()
	cfg, err := loadTokenConfig(t.out, env, "")
	if err != nil {
		t.print("error: " + err.Error())
		return
	}
	set := cfg.Resolve(env.Lookup)
	for _, w := range set.Warnings {
		t.print("  " + w)
	}
	if set.Len("") == 0 {
		t.print("no credentials resolved — set the env vars named in your config, or use Add or replace a token")
		return
	}
	t.print(set.RedactedJSON(nil))
	if !t.pr.Confirm("Write these to their credential files?") {
		t.print("nothing changed")
		return
	}
	lockForMutation(t.p)
	if err := writeTokenTargets(t.tokenIO(), env, set, set.Targets(), "", "", cfg); err != nil {
		t.print("error: " + err.Error())
	}
}
