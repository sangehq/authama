package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// The signature is Standard Webhooks' v1: the base64 HMAC-SHA256, keyed with
// the decoded secret, of "<id>.<timestamp>.<body>". Checked against the
// standard library computing it independently, with a secret, id and body
// generated for each run (no secret is written in the source).
func TestTheSignatureIsStandardWebhooksV1(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	secret := SecretPrefix + base64.StdEncoding.EncodeToString(key)
	idRaw := make([]byte, 12)
	_, _ = rand.Read(idRaw)
	id := "msg_" + base64.RawURLEncoding.EncodeToString(idRaw)
	ts := time.Now().Unix()
	body := []byte(fmt.Sprintf(`{"n":%d}`, ts))

	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%d.%s", id, ts, body)
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	got, err := Sign(secret, id, ts, body)
	if err != nil || got != want {
		t.Fatalf("signature %s, want %s (%v)", got, want, err)
	}
	now := time.Unix(ts, 0)
	tsHeader := strconv.FormatInt(ts, 10)
	if !Verify(secret, id, tsHeader, "v1,other "+got, body, now, 5*time.Minute) {
		t.Error("a matching signature among several is refused")
	}
	if Verify(secret, id, tsHeader, got, append(body, ' '), now, 5*time.Minute) {
		t.Error("a changed body verifies")
	}
	if Verify(secret, id, tsHeader, got, body, now.Add(time.Hour), 5*time.Minute) {
		t.Error("an old timestamp verifies")
	}
	if _, err := Sign("not-a-secret!", id, ts, body); err == nil {
		t.Error("a secret that is not whsec_ base64 was used")
	}
}

// An endpoint must be https to a public address; a private one only when the
// owner allows it, and plain http only to this machine then.
func TestAnEndpointMustBePublicHTTPSUnlessAllowed(t *testing.T) {
	for _, tc := range []struct {
		url     string
		private bool
		ok      bool
	}{
		{"https://hooks.example.com/authama", false, true},
		{"http://hooks.example.com/authama", false, false},
		{"ftp://hooks.example.com", false, false},
		{"https://127.0.0.1/hook", false, false},
		{"https://10.0.0.5/hook", false, false},
		{"https://192.168.1.2/hook", false, false},
		{"https://169.254.169.254/latest/meta-data", false, false},
		{"https://[::1]/hook", false, false},
		{"https://localhost/hook", false, false},
		{"https://user:pw@hooks.example.com/", false, false},
		{"https://10.0.0.5/hook", true, true},
		{"http://127.0.0.1:9000/hook", true, true},
		{"http://10.0.0.5/hook", true, false},
	} {
		if err := CheckURL(tc.url, tc.private); (err == nil) != tc.ok {
			t.Errorf("%s (allow private %v): %v, want ok=%v", tc.url, tc.private, err, tc.ok)
		}
	}
}

// The dialer checks the address it actually connects to, so a public name
// that resolves to a private address is refused too.
func TestTheSenderRefusesAPrivateAddressWhenDialling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	resp, err := Client(false).Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("a loopback address was dialled with private endpoints off")
	}
	if !strings.Contains(err.Error(), "not a public address") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
	resp, err = Client(true).Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("with private endpoints allowed: %v", err)
	}
	_ = resp.Body.Close()
}

// receiver is a test endpoint that verifies each delivery and answers with
// the status it is told to.
type receiver struct {
	mu     sync.Mutex
	secret string
	status int
	name   string
	got    []string // webhook-id of each verified delivery
	bad    int
}

func (rc *receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var ce map[string]any
	_ = json.Unmarshal(body, &ce)
	cloudEvent := r.Header.Get("Content-Type") == ContentType && ce["specversion"] == "1.0" && ce["id"] == r.Header.Get("webhook-id") &&
		ce["authamasub"] == rc.name && ce["source"] != nil && ce["type"] != nil
	if cloudEvent && Verify(rc.secret, r.Header.Get("webhook-id"), r.Header.Get("webhook-timestamp"), r.Header.Get("webhook-signature"), body, time.Now(), time.Hour*24*365*50) {
		rc.got = append(rc.got, r.Header.Get("webhook-id"))
	} else {
		rc.bad++
	}
	w.WriteHeader(rc.status)
}

func finishOp(t *testing.T, p paths.Paths, id string, at time.Time) {
	t.Helper()
	s, err := history.Open(p.HistoryDB())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	op := history.Op{OpID: id, StartedAt: at.Format(history.TimeFormat), FinishedAt: at.Format(history.TimeFormat), Command: "host add", Source: "cli", Result: "ok"}
	if err := s.Begin(&op); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(&op); err != nil {
		t.Fatal(err)
	}
}

// Delivery sends what happened after the endpoint was added, in order and
// signed; a failure is retried after its backoff from the same event, and a
// 410 disables the endpoint.
func TestDeliveryIsInOrderSignedAndRetried(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	t0 := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	finishOp(t, p, "before", t0.Add(-time.Minute)) // history: never sent
	rc := &receiver{status: 204}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	ep, err := Add(p, "local", srv.URL+"/hook", nil, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ep.Secret, SecretPrefix) {
		t.Fatalf("secret %q", ep.Secret)
	}
	if fi, _ := os.Stat(File(p)); fi.Mode().Perm() != 0o600 {
		t.Errorf("webhooks.json is %v", fi.Mode().Perm())
	}
	rc.secret, rc.name = ep.Secret, ep.Name
	finishOp(t, p, "one", t0.Add(10*time.Second))
	finishOp(t, p, "two", t0.Add(20*time.Second))
	cl := Client(true)
	ctx := context.Background()

	if _, err := Deliver(ctx, p, cl, t0.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rc.got, ",") != "op_one,op_two" || rc.bad != 0 {
		t.Fatalf("delivered %v (%d bad), want op_one,op_two", rc.got, rc.bad)
	}
	if _, err := Deliver(ctx, p, cl, t0.Add(40*time.Second)); err != nil || len(rc.got) != 2 {
		t.Fatalf("a second delivery sent again: %v %v", rc.got, err)
	}

	// The receiver fails: the event waits for its backoff, then goes again.
	rc.status = 500
	finishOp(t, p, "three", t0.Add(50*time.Second))
	res, _ := Deliver(ctx, p, cl, t0.Add(60*time.Second))
	if len(res) != 1 || res[0].Err == nil || res[0].Sent != 0 {
		t.Fatalf("a failing receiver: %+v", res)
	}
	if res, _ := Deliver(ctx, p, cl, t0.Add(61*time.Second)); !res[0].Postponed {
		t.Errorf("the backoff was not kept: %+v", res)
	}
	rc.status = 204
	if res, _ := Deliver(ctx, p, cl, t0.Add(70*time.Second)); res[0].Sent != 1 || rc.got[len(rc.got)-1] != "op_three" {
		t.Fatalf("after the backoff: %+v, got %v", res, rc.got)
	}

	// 410 Gone disables it, and says why.
	rc.status = 410
	finishOp(t, p, "four", t0.Add(80*time.Second))
	if res, _ := Deliver(ctx, p, cl, t0.Add(90*time.Second)); !res[0].Disabled {
		t.Fatalf("a 410 did not disable: %+v", res)
	}
	eps, _ := List(p)
	if !eps[0].Disabled || !strings.Contains(eps[0].DisabledWhy, "410") {
		t.Errorf("endpoint after a 410: %+v", eps[0])
	}
	if err := SetDisabled(p, "local", false); err != nil {
		t.Fatal(err)
	}
	if eps, _ := List(p); eps[0].Disabled || eps[0].Failures != 0 {
		t.Errorf("enabled again: %+v", eps[0])
	}
	// A test message goes whatever the state, and moves nothing.
	rc.status = 204
	before, _ := List(p)
	if status, err := Test(ctx, p, cl, "local", t0.Add(100*time.Second)); err != nil || status != 204 {
		t.Fatalf("test: %d %v", status, err)
	}
	if after, _ := List(p); after[0].Cursor != before[0].Cursor {
		t.Error("a test moved the cursor")
	}
	if err := Remove(p, "local"); err != nil {
		t.Fatal(err)
	}
	if eps, _ := List(p); len(eps) != 0 {
		t.Errorf("removed, still %v", eps)
	}
}
