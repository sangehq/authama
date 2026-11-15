package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/platform"
)

// stderrIsTerminal is a variable so tests can say whether a progress line may
// be drawn.
var stderrIsTerminal = platform.StderrIsTerminal

// progressDelay is how long a step runs before its progress line appears, so
// a quick step draws nothing at all.
const progressDelay = time.Second

// progressFrames are the spinner's frames: plain ASCII, so it reads on any
// terminal and font.
var progressFrames = []byte{'|', '/', '-', '\\'}

// startProgress shows, on a terminal, that a slow step is still running: after
// a second, a spinner, the label and the seconds so far on one line of stderr,
// redrawn in place with a carriage return. The returned stop clears the line
// and must be called before anything else is printed or asked, so a progress
// line never sits over output or a prompt.
//
// It draws nothing when stderr is not the process's own terminal (a pipe, a log,
// a test's buffer) or TERM is dumb, so scripts and logs see exactly what they
// saw before. It writes no escape codes (TestNoSourceEmitsTerminalEscapes).
func startProgress(c *cobra.Command, label string) (stop func()) {
	w := c.ErrOrStderr()
	if w != io.Writer(os.Stderr) || !stderrIsTerminal() || os.Getenv("TERM") == "dumb" {
		return func() {}
	}
	return drawProgress(w, label, progressDelay, 100*time.Millisecond)
}

// drawProgress runs the progress line on w until stop is called: nothing for
// delay, then a frame every tick. stop waits for the drawing to end and clears
// what it drew, so w is quiet when it returns.
func drawProgress(w io.Writer, label string, delay, tick time.Duration) (stop func()) {
	quit := make(chan struct{})
	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		select {
		case <-quit:
			return
		case <-time.After(delay):
		}
		t := time.NewTicker(tick)
		defer t.Stop()
		width := 0
		for frame := 0; ; frame++ {
			line := fmt.Sprintf("%c %s (%ds)", progressFrames[frame%len(progressFrames)], label, int(time.Since(start).Seconds()))
			pad := ""
			if n := width - len(line); n > 0 {
				pad = strings.Repeat(" ", n)
			}
			_, _ = fmt.Fprint(w, "\r"+line+pad)
			width = max(width, len(line))
			select {
			case <-quit:
				_, _ = fmt.Fprint(w, "\r"+strings.Repeat(" ", width)+"\r")
				return
			case <-t.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(quit) })
		<-done
	}
}
