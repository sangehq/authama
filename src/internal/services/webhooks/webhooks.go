// Package webhooks sends authama's event feed to endpoints you register, as
// Standard Webhooks (https://www.standardwebhooks.com, spec 1.0.0) describes:
// each event is a POST of the CloudEvent itself, in structured mode
// (Content-Type: application/cloudevents+json), with the headers webhook-id
// (the event's id), webhook-timestamp and webhook-signature, the signature
// being "v1," and the base64 HMAC-SHA256 of "<id>.<timestamp>.<body>" under
// the endpoint's whsec_ secret. The endpoint's name is in the signed body
// (authamasub), so a delivery cannot be replayed to another endpoint that
// checks it. This is sange's wire format, so one receiver reads both.
//
// A webhook is a request authama makes to an address someone chose, so the
// sender refuses what would let it be aimed inward: only https, never a
// loopback, private, link-local, unspecified or multicast address (checked on
// the address actually dialled, so DNS cannot rebind around it) unless the
// owner allows private endpoints, and never a redirect.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sangehq/authama/src/internal/services/feed"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// SecretPrefix starts every endpoint secret, as Standard Webhooks names it.
const SecretPrefix = "whsec_"

// Endpoint is one registered receiver and where its delivery has got to.
type Endpoint struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Secret string   `json:"secret"` // whsec_ and base64 of 32 random bytes
	Events []string `json:"events,omitempty"`
	// Cursor is the feed position of the last event delivered.
	Cursor   string    `json:"cursor"`
	Disabled bool      `json:"disabled,omitempty"`
	Created  time.Time `json:"created"`
	// Failures counts failed attempts in a row; NextAttempt is when the next
	// may be made after one.
	Failures    int       `json:"failures,omitempty"`
	NextAttempt time.Time `json:"next_attempt,omitzero"`
	LastStatus  int       `json:"last_status,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	LastSuccess time.Time `json:"last_success,omitzero"`
	// DisabledWhy says why delivery stopped by itself (a 410, too many
	// failures); "" when it was disabled by hand or is on.
	DisabledWhy string `json:"disabled_why,omitempty"`
}

// File is where endpoints are kept: owner-only, as it holds their secrets.
func File(p paths.Paths) string { return filepath.Join(p.StateDir(), "webhooks.json") }

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ErrNotFound is a name no endpoint has.
var ErrNotFound = errors.New("no webhook by that name")

// List is every endpoint, in the order they were added.
func List(p paths.Paths) ([]Endpoint, error) {
	b, err := os.ReadFile(File(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Endpoint
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", File(p), err)
	}
	return out, nil
}

func save(p paths.Paths, eps []Endpoint) error {
	if err := fs.EnsureDir(p.StateDir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(eps, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteTextAtomic(File(p), string(b)+"\n", 0o600)
}

// Add registers an endpoint that is sent events whose type starts with one of
// events (all of them when there are none), from now on: it is not sent the
// history. It returns the endpoint, whose secret is shown only then.
func Add(p paths.Paths, name, rawURL string, events []string, allowPrivate bool, now time.Time) (Endpoint, error) {
	if !nameRe.MatchString(name) {
		return Endpoint{}, fmt.Errorf("%q is not a webhook name: use lowercase letters, digits, '.', '_' and '-'", name)
	}
	if err := CheckURL(rawURL, allowPrivate); err != nil {
		return Endpoint{}, err
	}
	eps, err := List(p)
	if err != nil {
		return Endpoint{}, err
	}
	if slices.ContainsFunc(eps, func(e Endpoint) bool { return e.Name == name }) {
		return Endpoint{}, fmt.Errorf("a webhook named %q exists: remove it first, or choose another name", name)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Endpoint{}, err
	}
	ep := Endpoint{Name: name, URL: rawURL, Secret: SecretPrefix + base64.StdEncoding.EncodeToString(raw),
		Events: events, Cursor: feed.Now(now), Created: now.UTC()}
	return ep, save(p, append(eps, ep))
}

// Remove forgets an endpoint and its secret.
func Remove(p paths.Paths, name string) error {
	return update(p, name, func(eps []Endpoint, i int) []Endpoint { return slices.Delete(eps, i, i+1) })
}

// SetDisabled turns delivery to an endpoint off or back on. Turned on, its
// failures are forgotten, and it resumes from where it stopped.
func SetDisabled(p paths.Paths, name string, off bool) error {
	return update(p, name, func(eps []Endpoint, i int) []Endpoint {
		eps[i].Disabled, eps[i].DisabledWhy = off, ""
		if !off {
			eps[i].Failures, eps[i].NextAttempt = 0, time.Time{}
		}
		return eps
	})
}

func update(p paths.Paths, name string, f func([]Endpoint, int) []Endpoint) error {
	eps, err := List(p)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(eps, func(e Endpoint) bool { return e.Name == name })
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return save(p, f(eps, i))
}

// CheckURL says why rawURL cannot be an endpoint: it must be https with a
// host, and, unless allowPrivate, not name a loopback or private address
// literally (a name is checked again when it is dialled). With allowPrivate,
// plain http is allowed to a loopback address, for a receiver on this machine.
func CheckURL(rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not an address a webhook can be sent to", rawURL)
	}
	if u.User != nil {
		return errors.New("a webhook address cannot carry a user name or password: the secret signs each request instead")
	}
	host := u.Hostname()
	ip, perr := netip.ParseAddr(host)
	isIP := perr == nil
	loopback := host == "localhost" || isIP && ip.IsLoopback()
	switch u.Scheme {
	case "https":
	case "http":
		if !allowPrivate || !loopback {
			return errors.New("a webhook is sent over https; plain http only to this machine, with webhooks.allow_private on")
		}
	default:
		return fmt.Errorf("a webhook is sent over https, not %q", u.Scheme)
	}
	if !allowPrivate && (host == "localhost" || strings.HasSuffix(host, ".localhost") || isIP && !public(ip)) {
		return errors.New("that address is on this machine or a private network: turn on webhooks.allow_private to send there")
	}
	return nil
}

// public reports whether ip is an address on the public internet.
func public(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsInterfaceLocalMulticast() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(ip) && // carrier-grade NAT
		!netip.MustParsePrefix("169.254.0.0/16").Contains(ip)
}

// Client is the sender's HTTP client: 10 s per request, no redirect followed
// (a 3xx is a failure, as the spec says), and, unless allowPrivate, a dialer
// that refuses any address that is not public, after the name is resolved.
func Client(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !public(ip) {
				return fmt.Errorf("refused to connect to %s: not a public address (webhooks.allow_private is off)", host)
			}
			return nil
		}
	}
	tr := &http.Transport{DialContext: d.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		Proxy: nil, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	return &http.Client{Timeout: 10 * time.Second, Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Sign is the webhook-signature value for a message: "v1," and the base64
// HMAC-SHA256, under the secret's key, of "<id>.<timestamp>.<body>".
func Sign(secret, id string, ts int64, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, SecretPrefix))
	if err != nil {
		return "", errors.New("not a whsec_ secret")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%d.", id, ts)
	_, _ = mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify checks a webhook-signature header as a receiver would: one of its
// space-separated signatures matches, compared in constant time, and the
// timestamp is within tolerance of now. Tests and `webhook test` use it.
func Verify(secret, id, tsHeader, sigHeader string, body []byte, now time.Time, tolerance time.Duration) bool {
	ts, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil || now.Sub(time.Unix(ts, 0)).Abs() > tolerance {
		return false
	}
	want, err := Sign(secret, id, ts, body)
	if err != nil {
		return false
	}
	for _, s := range strings.Fields(sigHeader) {
		if hmac.Equal([]byte(s), []byte(want)) {
			return true
		}
	}
	return false
}

// ContentType is a delivery's: a CloudEvent in structured mode.
const ContentType = "application/cloudevents+json; charset=utf-8"

// backoff is the wait after each failure in a row, after Standard Webhooks'
// schedule; the last repeats.
var backoff = []time.Duration{5 * time.Second, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 5 * time.Hour, 10 * time.Hour, 24 * time.Hour}

// MaxFailures in a row disables an endpoint.
const MaxFailures = 10

// Result is one endpoint's outcome from a Deliver.
type Result struct {
	Name      string
	Sent      int
	Err       error
	Disabled  bool
	Postponed bool // a failure's backoff has not run out
}

// Deliver sends each enabled endpoint the events after its cursor, in order,
// stopping at its first failure: it is tried again after a backoff, from the
// same event, so an endpoint sees each event once and in order.
func Deliver(ctx context.Context, p paths.Paths, client *http.Client, now time.Time) ([]Result, error) {
	eps, err := List(p)
	if err != nil {
		return nil, err
	}
	var out []Result
	for i := range eps {
		ep := &eps[i]
		if ep.Disabled {
			continue
		}
		res := Result{Name: ep.Name}
		if now.Before(ep.NextAttempt) {
			res.Postponed = true
			out = append(out, res)
			continue
		}
		evs, next, err := feed.List(p, ep.Cursor, 100)
		if err != nil {
			res.Err = err
			out = append(out, res)
			continue
		}
		for _, e := range evs {
			if !feed.Matches(e, ep.Events) {
				ep.Cursor = e.Cursor()
				continue
			}
			status, err := send(ctx, client, *ep, e, now)
			ep.LastStatus = status
			if err != nil {
				res.Err = err
				fail(ep, status, err, now)
				res.Disabled = ep.Disabled
				break
			}
			res.Sent++
			ep.Cursor, ep.Failures, ep.NextAttempt, ep.LastError, ep.LastSuccess = e.Cursor(), 0, time.Time{}, "", now.UTC()
		}
		// Every event listed went (or was filtered out): carry on from the
		// feed's next cursor, not a comparison of cursors as strings.
		if res.Err == nil {
			ep.Cursor = next
		}
		out = append(out, res)
	}
	return out, save(p, eps)
}

func fail(ep *Endpoint, status int, err error, now time.Time) {
	ep.Failures++
	ep.LastError = err.Error()
	switch {
	case status == http.StatusGone:
		ep.Disabled, ep.DisabledWhy = true, "the endpoint answered 410 Gone"
	case ep.Failures >= MaxFailures:
		ep.Disabled, ep.DisabledWhy = true, fmt.Sprintf("%d failures in a row", ep.Failures)
	default:
		ep.NextAttempt = now.Add(backoff[min(ep.Failures, len(backoff))-1]).UTC()
	}
}

// send POSTs one event and returns the status it was answered with.
func send(ctx context.Context, client *http.Client, ep Endpoint, e feed.Event, now time.Time) (int, error) {
	e.Sub = ep.Name
	body, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	ts := now.Unix()
	sig, err := Sign(ep.Secret, e.ID, ts, body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", ContentType)
	req.Header.Set("User-Agent", "authama-webhooks/1")
	req.Header.Set("webhook-id", e.ID)
	req.Header.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("webhook-signature", sig)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("answered %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// Test sends one webhook.tested event to an endpoint now, whatever its state,
// and moves nothing: its cursor and failures stay as they were.
func Test(ctx context.Context, p paths.Paths, client *http.Client, name string, now time.Time) (int, error) {
	eps, err := List(p)
	if err != nil {
		return 0, err
	}
	i := slices.IndexFunc(eps, func(e Endpoint) bool { return e.Name == name })
	if i < 0 {
		return 0, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	raw := make([]byte, 12)
	_, _ = rand.Read(raw)
	e := feed.New(p, "test_"+base64.RawURLEncoding.EncodeToString(raw), feed.TypeWebhookTested, "webhook/"+name, now,
		map[string]string{"message": "a test from authama: this endpoint receives its webhooks"})
	return send(ctx, client, eps[i], e, now)
}
