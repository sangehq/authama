package checklist

import (
	"os"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/tokens"
	"github.com/sangehq/authama/src/internal/services/tokensvc"
)

// Groups a token's steps are shown under.
const (
	gIssuer = "Where it was issued"
	gFiles  = "Tools' credential files"
	gSource = "Where authama reads it"
)

// tokenItem is one credential: a target (a tool) and the host it signs in to.
type tokenItem struct {
	target, host string
	installed    []tokens.Installed
	spec         *tokens.Spec
}

// issuers is where a token for a target is managed, when that is one place.
var issuers = map[string]string{
	"npm":    "https://www.npmjs.com/settings/~/tokens",
	"yarn":   "https://www.npmjs.com/settings/~/tokens",
	"gh":     "https://github.com/settings/tokens",
	"pypi":   "https://pypi.org/manage/account/token/",
	"gem":    "https://rubygems.org/profile/api_keys",
	"cargo":  "https://crates.io/settings/tokens",
	"docker": "https://app.docker.com/settings/personal-access-tokens",
}

// issuerFor is where the token for target on host is revoked, "" when
// authama cannot say.
func issuerFor(target, host string) string {
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return "https://github.com/settings/tokens"
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		return "https://" + host + "/-/user_settings/personal_access_tokens"
	case host == "registry.npmjs.org":
		return issuers["npm"]
	case host == "pypi.org" || host == "upload.pypi.org":
		return issuers["pypi"]
	}
	return issuers[target]
}

// tokenChecklists is a checklist per credential installed in a tool's file or
// declared in tokens.json.
func (s *Service) tokenChecklists(st store) []Checklist {
	env := tokensvc.NewEnv(s.P, s.Getenv, nil, "")
	items := map[string]*tokenItem{}
	get := func(target, host string) *tokenItem {
		k := target + "/" + host
		if items[k] == nil {
			items[k] = &tokenItem{target: target, host: host}
		}
		return items[k]
	}
	for _, in := range env.Inventory("", false) {
		in.Secret = ""
		it := get(in.Target, in.Host)
		it.installed = append(it.installed, in)
	}
	if src, err := env.FindConfig(""); err == nil && src.Path != "" {
		if c, err := tokensvc.LoadConfig(src.Path); err == nil {
			for _, sp := range c.Specs() {
				spec := sp
				get(sp.Target, sp.Host).spec = &spec
			}
		}
	}
	dotenv := map[string]bool{}
	if b, err := os.ReadFile(s.P.EnvFile()); err == nil {
		for _, kv := range tokens.ParseEnvFile(string(b)) {
			if kv[1] != "" {
				dotenv[kv[0]] = true
			}
		}
	}
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Checklist, 0, len(keys))
	for _, k := range keys {
		out = append(out, s.tokenChecklist(items[k], st, dotenv))
	}
	return out
}

func (s *Service) tokenChecklist(it *tokenItem, st store, dotenv map[string]bool) Checklist {
	id := "token:" + it.target + "/" + it.host
	c := Checklist{Kind: KindToken, ID: id, Name: it.target + " " + it.host, Mode: InUse}
	if since := st[id].Revoking; since != "" {
		c.Mode, c.Since = Revoking, since
	}
	revoking := c.Mode == Revoking
	names := envNames(it.spec)
	if revoking {
		c.Steps = append(c.Steps, Step{ID: "issuer", Group: gIssuer, Manual: true, Link: issuerFor(it.target, it.host),
			Title:  "Revoked where it was issued",
			Detail: "Only the service that issued the token can make it stop working; deleting it here does not."})
	}
	// The tools' files that hold it.
	for _, in := range it.installed {
		stID := "file:" + in.File
		if revoking {
			c.Steps = append(c.Steps, Step{ID: stID, Group: gFiles, State: Todo, Title: "Remove it from " + in.File,
				Fix: "authama token delete " + it.target + " " + it.host})
			continue
		}
		state, title := Done, "In "+in.File
		if !in.SecretPresent {
			state, title = Todo, "In "+in.File+", but with no secret"
		}
		c.Steps = append(c.Steps, Step{ID: stID, Group: gFiles, State: state, Title: title})
	}
	if revoking && len(it.installed) == 0 {
		c.Steps = append(c.Steps, Step{ID: "files", Group: gFiles, State: Done, Title: "In no tool's credential file"})
	}
	if !revoking && len(it.installed) == 0 {
		c.Steps = append(c.Steps, Step{ID: "files", Group: gFiles, State: Todo, Title: "Not installed in any tool's file yet",
			Fix: "authama token apply"})
	}
	if it.target == tokens.Docker {
		c.Steps = append(c.Steps, Step{ID: "docker-store", Group: gFiles, Manual: true,
			Title:  map[bool]string{true: "Logged out of Docker's credential store", false: "Docker's credential store holds no stale copy"}[revoking],
			Detail: "Docker may keep this login in a credential helper authama cannot read.",
			Fix:    "docker logout " + it.host})
	}
	// Where authama reads the secret from.
	for _, n := range names {
		set := dotenv[n] || s.Getenv(n) != ""
		switch {
		case revoking && set:
			c.Steps = append(c.Steps, Step{ID: "env:" + n, Group: gSource, State: Todo, Title: "Clear $" + n,
				Detail: "Still set in authama's .env or this environment, so `token apply` would write it back."})
		case revoking:
			c.Steps = append(c.Steps, Step{ID: "env:" + n, Group: gSource, State: Done, Title: "$" + n + " is not set"})
		case set:
			c.Steps = append(c.Steps, Step{ID: "env:" + n, Group: gSource, State: Done, Title: "$" + n + " is set"})
		default:
			c.Steps = append(c.Steps, Step{ID: "env:" + n, Group: gSource, State: Todo, Title: "Set $" + n,
				Detail: "tokens.json reads the secret from it, and it is empty here."})
		}
	}
	if it.spec != nil {
		if revoking {
			c.Steps = append(c.Steps, Step{ID: "config", Group: gSource, Manual: true,
				Title:  "Entry removed from tokens.json, or kept for its successor",
				Detail: "The entry holds only variable names. Keep it if a new token will use the same names."})
		} else {
			c.Steps = append(c.Steps, Step{ID: "config", Group: gSource, State: Done, Title: "Declared in tokens.json"})
		}
	}
	c.Steps = append(c.Steps, Step{ID: "copies", Group: gElse, Manual: true,
		Title:  map[bool]string{true: "Removed from CI secrets and other machines", false: "Every copy outside authama is known"}[revoking],
		Detail: "CI secrets, other machines, project .npmrc files in repositories."})
	return c
}

// envNames is the variables a spec reads its secret from.
func envNames(sp *tokens.Spec) []string {
	if sp == nil {
		return nil
	}
	var out []string
	for _, n := range []string{sp.Env, sp.UserEnv, sp.PassEnv, sp.KeyEnv, sp.SecretEnv} {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}
