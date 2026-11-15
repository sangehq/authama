package webhooks

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// holdFirst answers 204 to every request, holding the first until released,
// so a test can change the endpoints while a delivery is in flight.
type holdFirst struct {
	entered, release chan struct{}
	mu               sync.Mutex
	n                int
}

func newHoldFirst() *holdFirst {
	return &holdFirst{entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *holdFirst) RoundTrip(r *http.Request) (*http.Response, error) {
	h.mu.Lock()
	h.n++
	first := h.n == 1
	h.mu.Unlock()
	if first {
		close(h.entered)
		<-h.release
	}
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

func (h *holdFirst) sent() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

func oneEndpointWithAnEvent(t *testing.T) (paths.Paths, time.Time) {
	t.Helper()
	p := paths.Paths{ConfigDir: t.TempDir()}
	t0 := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	if _, err := Add(p, "one", "https://hooks.example.com/a", nil, false, t0); err != nil {
		t.Fatal(err)
	}
	finishOp(t, p, "op_one", t0.Add(time.Second))
	return p, t0
}

// Delivery wrote back the endpoints it read before sending, so a webhook
// removed while an event was in flight came back, secret and all (review
// R-BUG-04).
func TestAWebhookRemovedDuringDeliveryStaysRemoved(t *testing.T) {
	p, t0 := oneEndpointWithAnEvent(t)
	h := newHoldFirst()
	done := make(chan struct{})
	go func() {
		_, _ = Deliver(context.Background(), p, &http.Client{Transport: h}, t0.Add(10*time.Second))
		close(done)
	}()
	<-h.entered
	if err := Remove(p, "one"); err != nil {
		t.Fatal(err)
	}
	close(h.release)
	<-done
	if eps, _ := List(p); len(eps) != 0 {
		t.Errorf("the removed webhook came back: %+v", eps)
	}
}

// The same for a webhook disabled by hand while an event was in flight.
func TestAWebhookDisabledDuringDeliveryStaysDisabled(t *testing.T) {
	p, t0 := oneEndpointWithAnEvent(t)
	h := newHoldFirst()
	done := make(chan struct{})
	go func() {
		_, _ = Deliver(context.Background(), p, &http.Client{Transport: h}, t0.Add(10*time.Second))
		close(done)
	}()
	<-h.entered
	if err := SetDisabled(p, "one", true); err != nil {
		t.Fatal(err)
	}
	close(h.release)
	<-done
	eps, _ := List(p)
	if len(eps) != 1 || !eps[0].Disabled {
		t.Fatalf("the disable was lost: %+v", eps)
	}
	if eps[0].LastSuccess.IsZero() || eps[0].Cursor == "" {
		t.Errorf("the delivery's own progress was not kept: %+v", eps[0])
	}
}

// Two deliveries at once (the server's loop and `authama webhook deliver`)
// sent the same event twice; they now take turns, and the second finds
// nothing left to send.
func TestTwoDeliveriesAtOnceSendAnEventOnce(t *testing.T) {
	p, t0 := oneEndpointWithAnEvent(t)
	h := newHoldFirst()
	cl := &http.Client{Transport: h}
	first := make(chan struct{})
	go func() { _, _ = Deliver(context.Background(), p, cl, t0.Add(10*time.Second)); close(first) }()
	<-h.entered
	second := make(chan struct{})
	go func() { _, _ = Deliver(context.Background(), p, cl, t0.Add(10*time.Second)); close(second) }()
	time.Sleep(100 * time.Millisecond) // the second is waiting its turn
	close(h.release)
	<-first
	<-second
	if n := h.sent(); n != 1 {
		t.Errorf("one event was sent %d times", n)
	}
}
