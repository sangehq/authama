package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// stubTransport answers every request itself and records the type of each
// CloudEvent it was sent, so delivery is tested without a socket.
type stubTransport struct{ types []string }

func (s *stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	var ce struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(body, &ce)
	s.types = append(s.types, ce.Type)
	return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
}

// A webhook saved with a com.simtabi.authama. filter, before the event types
// moved under sh.sange.authama., keeps being sent the events it asked for.
func TestAWebhookSavedWithTheOldPrefixKeepsReceiving(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	t0 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	if _, err := Add(p, "old", "https://hooks.example.com/hook", []string{"com.simtabi.authama.operation."}, true, t0); err != nil {
		t.Fatal(err)
	}
	finishOp(t, p, "one", t0.Add(10*time.Second))
	tr := &stubTransport{}
	res, err := Deliver(context.Background(), p, &http.Client{Transport: tr}, t0.Add(20*time.Second), DetailRedacted)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Sent != 1 || len(tr.types) != 1 || tr.types[0] != "sh.sange.authama.operation.finished" {
		t.Fatalf("an old-prefix webhook was sent %v (%+v)", tr.types, res)
	}
}
