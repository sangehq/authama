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

// A webhook saved with a filter on either earlier prefix keeps being sent the
// events it asked for: com.simtabi.authama. (the original) and
// sh.sange.authama. (released briefly on 2026-11-16), both read as the same
// filter on com.simtabi.sange.authama..
func TestAWebhookSavedWithTheOldPrefixKeepsReceiving(t *testing.T) {
	for _, old := range []string{"com.simtabi.authama.", "sh.sange.authama."} {
		t.Run(old, func(t *testing.T) {
			p := paths.Paths{ConfigDir: t.TempDir()}
			t0 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
			if _, err := Add(p, "old", "https://hooks.example.com/hook", []string{old + "operation."}, true, t0); err != nil {
				t.Fatal(err)
			}
			finishOp(t, p, "one", t0.Add(10*time.Second))
			tr := &stubTransport{}
			res, err := Deliver(context.Background(), p, &http.Client{Transport: tr}, t0.Add(20*time.Second), DetailRedacted)
			if err != nil {
				t.Fatal(err)
			}
			if len(res) != 1 || res[0].Sent != 1 || len(tr.types) != 1 || tr.types[0] != "com.simtabi.sange.authama.operation.finished" {
				t.Fatalf("a webhook saved with %s was sent %v (%+v)", old, tr.types, res)
			}
		})
	}
}
