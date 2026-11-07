package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// lockedBuffer is a bytes.Buffer the drawing goroutine and the test can share.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A step that ends before the delay draws nothing at all.
func TestProgressDrawsNothingForAQuickStep(t *testing.T) {
	var w lockedBuffer
	stop := drawProgress(&w, "scanning gh", time.Hour, time.Millisecond)
	stop()
	if got := w.String(); got != "" {
		t.Fatalf("a quick step drew %q", got)
	}
}

// A slow step shows a spinner, its label and the seconds so far, redrawn in
// place, and stop leaves the line blank with the cursor at its start.
func TestProgressShowsASlowStepAndClearsIt(t *testing.T) {
	var w lockedBuffer
	stop := drawProgress(&w, "scanning gh (1 of 3)", time.Millisecond, time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for strings.Count(w.String(), "\r") < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stop()
	stop() // a second stop is harmless
	got := w.String()
	if !strings.Contains(got, "scanning gh (1 of 3) (0s)") {
		t.Errorf("no labelled progress line in %q", got)
	}
	frames := 0
	for _, f := range progressFrames {
		if strings.Contains(got, "\r"+string(f)+" ") {
			frames++
		}
	}
	if frames < 2 {
		t.Errorf("the spinner did not turn: %q", got)
	}
	last := got[strings.LastIndex(got[:len(got)-1], "\r"):]
	if strings.TrimSpace(last) != "" || !strings.HasSuffix(got, "\r") {
		t.Errorf("stop did not clear the line: ends %q", last)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\n") {
		t.Errorf("the progress line wrote an escape code or a newline: %q", got)
	}
}

// Anywhere but the process's own terminal, startProgress draws nothing: a
// command run in a test, a pipe or a log sees the output it always did.
func TestProgressStaysOffTheTerminalless(t *testing.T) {
	prev := stderrIsTerminal
	stderrIsTerminal = func() bool { return true }
	t.Cleanup(func() { stderrIsTerminal = prev })

	var w lockedBuffer
	c := &cobra.Command{}
	c.SetErr(&w)
	stop := startProgress(c, "scanning gh")
	time.Sleep(progressDelay + 200*time.Millisecond)
	stop()
	if got := w.String(); got != "" {
		t.Fatalf("progress drew on a writer that is not stderr: %q", got)
	}
}
