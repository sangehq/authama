package webhooks

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// capture keeps the body of every request and answers 204.
type capture struct {
	mu     sync.Mutex
	bodies []string
}

func (c *capture) RoundTrip(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.bodies = append(c.bodies, string(b))
	c.mu.Unlock()
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

// failedOp files an operation that failed, with an error naming a host and a
// subject, as a failed deploy records them.
func failedOp(t *testing.T, p paths.Paths, at time.Time) {
	t.Helper()
	s, err := history.Open(p.HistoryDB())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	op := history.Op{OpID: "failed", StartedAt: at.Format(history.TimeFormat), FinishedAt: at.Format(history.TimeFormat),
		Command: "ssh deploy", Source: "cli", Result: history.ResultError,
		Error:    "ssh deploy@db1.internal.example: Permission denied (publickey)",
		Subjects: []history.Subject{{Kind: "host", Name: "db1-internal"}}}
	if err := s.Begin(&op); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(&op); err != nil {
		t.Fatal(err)
	}
}

// A webhook sends an operation's result, never its error text or subjects,
// unless webhooks.include_details is on (audit A-SEC-12): an error can hold
// host names, addresses, user names and remote stderr, and a webhook hands it
// to a third party. The local feed keeps both.
func TestAWebhookCarriesNoErrorTextOrSubjectsByDefault(t *testing.T) {
	for _, tc := range []struct {
		name   string
		detail Detail
		want   bool
	}{
		{"redacted", DetailRedacted, false},
		{"full", DetailFull, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := paths.Paths{ConfigDir: t.TempDir()}
			t0 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
			if _, err := Add(p, "one", "https://hooks.example.com/a", nil, false, t0); err != nil {
				t.Fatal(err)
			}
			failedOp(t, p, t0.Add(time.Second))
			c := &capture{}
			if _, err := Deliver(context.Background(), p, &http.Client{Transport: c}, t0.Add(10*time.Second), tc.detail); err != nil {
				t.Fatal(err)
			}
			if len(c.bodies) != 1 {
				t.Fatalf("%d deliveries, want 1", len(c.bodies))
			}
			body := c.bodies[0]
			if !strings.Contains(body, `"result":"error"`) {
				t.Errorf("the result is missing, so a receiver cannot tell it failed:\n%s", body)
			}
			for _, secret := range []string{"db1.internal.example", "db1-internal"} {
				if got := strings.Contains(body, secret); got != tc.want {
					t.Errorf("%q in the payload: %v, want %v:\n%s", secret, got, tc.want, body)
				}
			}
		})
	}
}
