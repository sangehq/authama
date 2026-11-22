package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/services/apitokens"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/services/webhooks"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/log"
)

// integrationsPage serves /api-tokens and /webhooks: `authama api token ...`
// and `authama webhook ...`. Each change asks for the owner PIN (within the
// step-up window). A new token or signing secret is in the response to the
// form that made it, once, with no-store caching: it is never kept in the
// session, never in a URL, and never rendered again.
type integrationsPage struct{ s *Server }

// onceSecret is a secret shown on the page that made it, and never again.
type onceSecret struct {
	Name, Secret, What string
}

// apiTokenRow is one API token's record; it has no secret, only the hash's
// owner does.
type apiTokenRow struct {
	Name, Scopes, Created, Expires, LastUsed string
	Expired                                  bool
}

type apiTokensView struct {
	Tokens  []apiTokenRow
	On      bool
	Once    *onceSecret
	Done    string
	Problem string
}

// webhookRow is an endpoint as the page shows it: never its secret.
type webhookRow struct {
	Name, URL, Events, State, LastSent, LastError string
	Disabled                                      bool
}

type webhooksView struct {
	Endpoints    []webhookRow
	On           bool
	AllowPrivate bool
	Once         *onceSecret
	Done         string
	Results      []string
	Problem      string
}

// boolSetting is a boolean setting in effect: a value set in authama.json
// reaches the environment as 1 or 0, one set by hand may be true, yes or on.
func boolSetting(name string, def bool) bool {
	v := strings.TrimSpace(envnames.Get(nil, name))
	if v == "" {
		return def
	}
	return settings.Truthy(v)
}

func shortDay(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02")
}

func (ip integrationsPage) apiView() apiTokensView {
	v := apiTokensView{On: boolSetting(envnames.API, false)}
	toks, err := apitokens.List(ip.s.opts.Engine.Home())
	if err != nil {
		v.Problem = err.Error()
	}
	now := ip.s.now()
	for _, t := range toks {
		v.Tokens = append(v.Tokens, apiTokenRow{Name: t.Name, Scopes: strings.Join(t.Scopes, ", "), Created: shortDay(t.Created),
			Expires: shortDay(t.Expires), LastUsed: shortDay(t.LastUsed), Expired: t.Expired(now)})
	}
	return v
}

func (ip integrationsPage) webhooksView() webhooksView {
	v := webhooksView{On: boolSetting(envnames.Webhooks, false), AllowPrivate: boolSetting(envnames.WebhooksAllowPrivate, false)}
	eps, err := webhooks.List(ip.s.opts.Engine.Home())
	if err != nil {
		v.Problem = err.Error()
	}
	for _, ep := range eps {
		state := "on"
		switch {
		case ep.Disabled && ep.DisabledWhy != "":
			state = "off: " + ep.DisabledWhy
		case ep.Disabled:
			state = "off"
		case ep.Failures > 0:
			state = "retrying (" + strconv.Itoa(ep.Failures) + " failed)"
		}
		evs := "every event"
		if len(ep.Events) > 0 {
			evs = strings.Join(ep.Events, ", ")
		}
		v.Endpoints = append(v.Endpoints, webhookRow{Name: ep.Name, URL: ep.URL, Events: evs, State: state,
			LastSent: shortDay(ep.LastSuccess), LastError: ep.LastError, Disabled: ep.Disabled})
	}
	return v
}

func (ip integrationsPage) renderAPI(w http.ResponseWriter, r *http.Request, status int, v apiTokensView) {
	if v.Once != nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	ip.s.render(w, status, "apitokens", ip.s.newView(r, "apitokens", "API tokens", v))
}

func (ip integrationsPage) renderWebhooks(w http.ResponseWriter, r *http.Request, status int, v webhooksView) {
	if v.Once != nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	ip.s.render(w, status, "webhooks", ip.s.newView(r, "webhooks", "Webhooks", v))
}

func (ip integrationsPage) handleAPITokens(w http.ResponseWriter, r *http.Request) {
	ip.renderAPI(w, r, http.StatusOK, ip.apiView())
}

func (ip integrationsPage) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	ip.renderWebhooks(w, r, http.StatusOK, ip.webhooksView())
}

// handleAPITokenCreate is `authama api token create <name> [--expires N]`:
// the token is in this response only.
func (ip integrationsPage) handleAPITokenCreate(w http.ResponseWriter, r *http.Request) {
	if !ownerGate(ip).stepUp(w, r) {
		return
	}
	p := ip.s.opts.Engine.Home()
	name := strings.TrimSpace(r.PostFormValue("name"))
	days := 0
	if d := strings.TrimSpace(r.PostFormValue("expires")); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < 0 || n > 3650 {
			v := ip.apiView()
			v.Problem = "Expires is a number of days from 0 (never) to 3650."
			ip.renderAPI(w, r, http.StatusBadRequest, v)
			return
		}
		days = n
	}
	secret, t, err := apitokens.Create(p, name, days, ip.s.now())
	if err != nil {
		v := ip.apiView()
		v.Problem = err.Error()
		ip.renderAPI(w, r, http.StatusBadRequest, v)
		return
	}
	log.Audit(p.AuditLog(), "api.token.create", log.Field{Key: "name", Value: t.Name}, log.Field{Key: "source", Value: "web"})
	v := ip.apiView()
	v.Once = &onceSecret{Name: t.Name, Secret: secret, What: "API token"}
	ip.renderAPI(w, r, http.StatusOK, v)
}

// handleAPITokenRevoke is `authama api token revoke <name>`.
func (ip integrationsPage) handleAPITokenRevoke(w http.ResponseWriter, r *http.Request) {
	if !ownerGate(ip).stepUp(w, r) {
		return
	}
	p := ip.s.opts.Engine.Home()
	name := strings.TrimSpace(r.PostFormValue("name"))
	err := apitokens.Revoke(p, name)
	v := ip.apiView()
	if err != nil {
		v.Problem = err.Error()
		ip.renderAPI(w, r, http.StatusBadRequest, v)
		return
	}
	log.Audit(p.AuditLog(), "api.token.revoke", log.Field{Key: "name", Value: name}, log.Field{Key: "source", Value: "web"})
	v.Done = "Revoked API token " + name + ". A program still holding it is refused from now on."
	ip.renderAPI(w, r, http.StatusOK, v)
}

// handleWebhookAdd is `authama webhook add <name> <url> [--event ...]`: the
// signing secret is in this response only.
func (ip integrationsPage) handleWebhookAdd(w http.ResponseWriter, r *http.Request) {
	if !ownerGate(ip).stepUp(w, r) {
		return
	}
	p := ip.s.opts.Engine.Home()
	var events []string
	for _, e := range strings.FieldsFunc(r.PostFormValue("events"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		events = append(events, strings.TrimSpace(e))
	}
	ep, err := webhooks.Add(p, strings.TrimSpace(r.PostFormValue("name")), strings.TrimSpace(r.PostFormValue("url")), events,
		boolSetting(envnames.WebhooksAllowPrivate, false), ip.s.now())
	if err != nil {
		v := ip.webhooksView()
		v.Problem = err.Error()
		ip.renderWebhooks(w, r, http.StatusBadRequest, v)
		return
	}
	log.Audit(p.AuditLog(), "webhook.add", log.Field{Key: "name", Value: ep.Name}, log.Field{Key: "source", Value: "web"})
	v := ip.webhooksView()
	v.Once = &onceSecret{Name: ep.Name, Secret: ep.Secret, What: "signing secret"}
	ip.renderWebhooks(w, r, http.StatusOK, v)
}

// handleWebhookAct is `authama webhook remove|enable|disable|test <name>` and
// `authama webhook deliver`.
func (ip integrationsPage) handleWebhookAct(w http.ResponseWriter, r *http.Request) {
	if !ownerGate(ip).stepUp(w, r) {
		return
	}
	p := ip.s.opts.Engine.Home()
	act, name := r.PathValue("act"), strings.TrimSpace(r.PostFormValue("name"))
	allow := boolSetting(envnames.WebhooksAllowPrivate, false)
	var done string
	var results []string
	var err error
	switch act {
	case "remove":
		if err = webhooks.Remove(p, name); err == nil {
			done = "Removed webhook " + name + "."
		}
	case "enable", "disable":
		if err = webhooks.SetDisabled(p, name, act == "disable"); err == nil {
			done = "Turned webhook " + name + " " + map[bool]string{true: "off", false: "on"}[act == "disable"] + "."
		}
	case "test":
		var status int
		if status, err = webhooks.Test(r.Context(), p, webhooks.Client(allow), name, ip.s.now()); err == nil {
			done = "Webhook " + name + " answered " + strconv.Itoa(status) + "."
		}
	case "deliver":
		detail := webhooks.DetailRedacted
		if boolSetting(envnames.WebhooksIncludeDetails, false) {
			detail = webhooks.DetailFull
		}
		var res []webhooks.Result
		if res, err = webhooks.Deliver(r.Context(), p, webhooks.Client(allow), ip.s.now(), detail); err == nil {
			done = "Delivered."
			if len(res) == 0 {
				done = "No webhook is on."
			}
			for _, x := range res {
				results = append(results, deliveryLine(x))
			}
		}
	default:
		ip.s.renderError(w, r, http.StatusNotFound, "Not found", "There is no webhook action "+act+".")
		return
	}
	v := ip.webhooksView()
	if err != nil {
		v.Problem = err.Error()
		ip.renderWebhooks(w, r, http.StatusBadRequest, v)
		return
	}
	log.Audit(p.AuditLog(), "webhook."+act, log.Field{Key: "name", Value: name}, log.Field{Key: "source", Value: "web"})
	v.Done, v.Results = done, results
	ip.renderWebhooks(w, r, http.StatusOK, v)
}

// deliveryLine is one endpoint's delivery, as `webhook deliver` prints it.
func deliveryLine(r webhooks.Result) string {
	switch {
	case r.Postponed:
		return r.Name + ": waiting out a backoff after a failure"
	case r.Disabled:
		return r.Name + ": sent " + strconv.Itoa(r.Sent) + ", then turned off: " + r.Err.Error()
	case r.Err != nil:
		return r.Name + ": sent " + strconv.Itoa(r.Sent) + ", then failed (tried again later): " + r.Err.Error()
	}
	return r.Name + ": sent " + strconv.Itoa(r.Sent)
}
