package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/platform"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/tokensvc"
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

func (t tokenMenu) tokSet() {
	choices := append(tokens.Targets(), tuiBack)
	target, ok := t.pr.Select("Which tool", choices)
	if !ok || target == tuiBack {
		return
	}
	mech := ""
	if target == tokens.Composer {
		svcs := tokens.LoadCatalogServices()
		names := make([]string, 0, len(svcs))
		for n := range svcs {
			names = append(names, n)
		}
		sort.Strings(names)
		svc, ok := t.pr.Select("Which Composer service", append(names, tuiBack))
		if !ok || svc == tuiBack {
			return
		}
		mech = svcs[svc]
		if !tokens.IsTokenMechanism(mech) && mech != "http-basic" {
			t.print(fmt.Sprintf("%s (%s) takes more than one secret; add it to the token config instead", svc, mech))
			return
		}
	}
	server, ok := t.pr.Line("Server (e.g. github.com, registry.npmjs.org): ")
	if !ok || strings.TrimSpace(server) == "" {
		t.print("nothing saved: a server is required")
		return
	}
	var cr tokens.Credential
	if target == tokens.NPM || target == tokens.Composer && tokens.IsTokenMechanism(mech) {
		scope := ""
		if target == tokens.NPM {
			if scope, ok = t.pr.Line("npm scope to bind (optional, e.g. @org): "); !ok {
				return
			}
		}
		s, ok := t.secret("Token: ")
		if !ok || s == "" {
			t.print("nothing saved: the token is empty")
			return
		}
		cr = tokens.Credential{Token: s, Scope: strings.TrimSpace(scope)}
	} else {
		user, ok := t.pr.Line("User name: ")
		if !ok || strings.TrimSpace(user) == "" {
			t.print("nothing saved: " + target + " needs a user name")
			return
		}
		s, ok := t.secret("Password / token: ")
		if !ok || s == "" {
			t.print("nothing saved: the password is empty")
			return
		}
		cr = tokens.Credential{Username: strings.TrimSpace(user), Password: s}
	}
	set := tokens.NewSet()
	if !set.Apply(target, mech, strings.TrimSpace(server), cr) {
		t.print("nothing saved: the credential is incomplete")
		return
	}
	env := t.tokenEnv()
	if !t.pr.Confirm(fmt.Sprintf("Save it to %s?", env.TargetPath(target, ""))) {
		t.print("nothing changed")
		return
	}
	lockForMutation(t.p)
	if err := writeTokenTargets(t.tokenIO(), env, set, []string{target}, "", "", nil); err != nil {
		t.print("error: " + err.Error())
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
