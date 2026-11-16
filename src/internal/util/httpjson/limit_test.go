package httpjson

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubTransport answers every request with status and body, binding no port.
func stubTransport(t *testing.T, status int, body func() io.Reader) {
	t.Helper()
	prev := client.Transport
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(body()), Header: http.Header{}, Request: r}, nil
	})
	t.Cleanup(func() { client.Transport = prev })
}

// endless stands for a body that never ends, what a hostile endpoint streams.
// It stops at eight times the limit only so a regression fails instead of
// hanging the suite.
type endless struct{ read int64 }

func (e *endless) Read(p []byte) (int, error) {
	if e.read >= 8*MaxBody {
		return 0, io.EOF
	}
	for i := range p {
		p[i] = ' '
	}
	e.read += int64(len(p))
	return len(p), nil
}

// A provider API that streams without end cannot exhaust memory: the body is
// read up to MaxBody and the call fails past it (audit S-04).
func TestAnOversizedBodyIsRefusedNotReadToTheEnd(t *testing.T) {
	noSleep(t)
	src := &endless{}
	stubTransport(t, 200, func() io.Reader { return src })
	_, err := RequestJSON("GET", "https://api.example.test/keys", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("an endless body was not refused: %v", err)
	}
	if src.read > MaxBody+64<<10 {
		t.Errorf("read %d bytes, want at most about %d", src.read, MaxBody)
	}
}

// A body just inside the limit still parses.
func TestABodyAtTheLimitIsRead(t *testing.T) {
	noSleep(t)
	body := `{"k":"` + strings.Repeat("a", MaxBody-10) + `"}`
	if len(body) > MaxBody {
		t.Fatalf("fixture is %d bytes, over the limit", len(body))
	}
	stubTransport(t, 200, func() io.Reader { return strings.NewReader(body) })
	if _, err := RequestJSON("GET", "https://api.example.test/keys", nil, nil); err != nil {
		t.Fatalf("a body within the limit failed: %v", err)
	}
}

// Server-chosen text quoted in an error is short and carries no control
// characters, so it cannot forge lines in the terminal, the history or the
// events feed.
func TestAnErrorQuotesAShortCleanExcerpt(t *testing.T) {
	noSleep(t)
	evil := "denied\x1b[2J\r\nFORGED LINE\x00" + strings.Repeat("y", 5000)
	stubTransport(t, 403, func() io.Reader { return strings.NewReader(evil) })
	_, err := RequestJSON("GET", "https://api.example.test/keys", nil, nil)
	if err == nil {
		t.Fatal("403 should be an error")
	}
	msg := err.Error()
	if strings.ContainsAny(msg, "\x1b\r\n\x00") {
		t.Errorf("the error carries control characters: %q", msg)
	}
	if !strings.Contains(msg, "denied") {
		t.Errorf("the error lost the body's start: %q", msg)
	}
	if n := strings.Count(msg, "y"); n > ExcerptLen {
		t.Errorf("the error quotes %d bytes of body, want at most %d", n, ExcerptLen)
	}

	stubTransport(t, 200, func() io.Reader { return strings.NewReader("<html>\x1b]0;x\x07" + strings.Repeat("z", 5000)) })
	_, err = RequestJSON("GET", "https://api.example.test/keys", nil, nil)
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\x07") || strings.Count(err.Error(), "z") > ExcerptLen {
		t.Errorf("a non-JSON body's excerpt is not short and clean: %q", err)
	}
}
