package tokensvc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/tokens"
)

// SetSpec is one credential to add or replace, as `authama token set` takes
// it: a Composer service or a target, the server, and the user name and npm
// scope where they apply. The secret is not part of it, so a spec can be
// checked, shown and confirmed before anyone is asked for the secret.
type SetSpec struct {
	Target, Service, Server, Username, Scope string
}

// SetPlan is a checked SetSpec: the tool it writes, the Composer mechanism,
// and what the secret is.
type SetPlan struct {
	Spec      SetSpec
	Target    string
	Mechanism string
	// Token is true when the secret is a token, false when it is a password
	// (or a token used as one) sent with the user name.
	Token bool
}

// SecretLabel is what to call the secret when asking for it.
func (p SetPlan) SecretLabel() string {
	if p.Token {
		return "Token"
	}
	return "Password / token"
}

// Check validates the spec the way `token set` does, before any secret is read.
func (s SetSpec) Check() (SetPlan, error) {
	pl := SetPlan{Spec: s, Target: s.Target}
	if s.Service != "" {
		// A service is a Composer one: a second tool beside it is a
		// contradiction, and taking either silently writes the wrong file.
		if s.Target != "" && s.Target != tokens.Composer {
			return pl, fmt.Errorf("--service is for Composer, not %s: drop --service, or --target", s.Target)
		}
		m, ok := tokens.LoadCatalogServices()[s.Service]
		if !ok {
			return pl, fmt.Errorf("unknown service %q (see `%s token providers`)", s.Service, brand.Command)
		}
		pl.Target, pl.Mechanism = tokens.Composer, m
	} else {
		if s.Target == "" {
			return pl, errors.New("give --service (Composer) or --target")
		}
		if !tokens.IsTarget(s.Target) {
			return pl, fmt.Errorf("unknown target %q (choose from %s)", s.Target, strings.Join(tokens.Targets(), ", "))
		}
		if s.Target == tokens.Composer {
			return pl, errors.New("a Composer token needs --service (see `" + brand.Command + " token providers`)")
		}
	}
	if strings.TrimSpace(s.Server) == "" {
		return pl, errors.New("--server is required")
	}
	switch {
	case tokens.TargetKind(pl.Target) == tokens.KindToken || pl.Target == tokens.Composer && tokens.IsTokenMechanism(pl.Mechanism):
		pl.Token = true
	case pl.Target == tokens.Composer && pl.Mechanism != "http-basic":
		return pl, fmt.Errorf("%s (%s) takes more than one secret; add it to the token config instead", s.Service, pl.Mechanism)
	default:
		if s.Username == "" {
			return pl, errors.New("--username is required for " + pl.Target)
		}
	}
	return pl, nil
}

// Credential is the set holding the one credential, with its secret.
func (p SetPlan) Credential(secret string) (*tokens.Set, error) {
	cr := tokens.Credential{Username: p.Spec.Username, Password: secret}
	if p.Token {
		cr = tokens.Credential{Token: secret, Scope: p.Spec.Scope}
	}
	set := tokens.NewSet()
	if !set.Apply(p.Target, p.Mechanism, strings.TrimSpace(p.Spec.Server), cr) {
		return nil, errors.New("nothing to save: the credential is incomplete")
	}
	return set, nil
}
