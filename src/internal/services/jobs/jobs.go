// Package jobs runs a change that takes longer than a request, or reaches
// another machine, as a job: one at a time, under the advisory lock for as
// long as it runs, with its output kept line by line for a page to stream.
//
// A job is a person's confirmed command, run exactly as the CLI runs it. It
// can pause to ask the person something (rotate's "pasted the key?"), and it
// can be cancelled, which stops it between steps - never inside one, so a
// cancelled job never leaves a step half done. It holds the process's one
// change slot (mutation.Exclusive) from start to end, so no preview, change
// or other job interleaves with it.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// States a job is in.
const (
	Running   = "running"
	Waiting   = "waiting" // paused on a question
	Done      = "done"
	Failed    = "failed"
	Cancelled = "cancelled"
)

// ErrCancelled is what a job's steps return once it has been cancelled. The
// history records the job as aborted.
var ErrCancelled = fmt.Errorf("cancelled: the steps before this one finished, and nothing after it ran: %w",
	mutation.ErrDeclined)

// Question is what a paused job is asking. Text is shown as is, for copying:
// rotate's staged public key.
type Question struct {
	Prompt string
	Text   string
	// Yes and No label the two answers when the question is not "is the
	// step done?" (pinning asks whether to trust keys). Empty keeps the
	// default wording.
	Yes, No string
}

// Progress is how far a job made of counted steps has got: Done of Total
// steps finished, Label naming the one under way, and Since when the first
// step began, from which a page works out the time left.
type Progress struct {
	Done, Total int
	Label       string
	Since       time.Time
	// Items are the things the job works through in order (keys, hosts),
	// each with its state, when the job named them with Plan.
	Items []Item
}

// Item states, as a job page shows them.
const (
	ItemWaiting = "waiting"
	ItemRunning = "running"
	ItemDone    = "done"
	ItemFailed  = "failed"
)

// Item is one thing a job works through, and how far it has got.
type Item struct{ Name, State string }

// Percent is how much of the job is done, 0 to 100.
func (p Progress) Percent() int {
	if p.Total <= 0 {
		return 0
	}
	return min(100, p.Done*100/p.Total)
}

// Left is the time the remaining steps should take at the pace of the
// finished ones, and false while no step has finished to judge by.
func (p Progress) Left(now time.Time) (time.Duration, bool) {
	if p.Done <= 0 || p.Total <= p.Done || p.Since.IsZero() {
		return 0, false
	}
	per := now.Sub(p.Since) / time.Duration(p.Done)
	return per * time.Duration(p.Total-p.Done), true
}

// View is a job as a page shows it.
type View struct {
	ID, Command, State, Err, OpID string
	// Title names the job for people ("Rotate 10 keys"); "" for a job that
	// was given none, whose page then shows its command.
	Title    string
	Args     []string
	Lines    []string
	Question *Question
	Started  time.Time
	// Progress is nil for a job that does not count its steps.
	Progress *Progress
	// Summary is what the job set as its result once it ended, for its page
	// to draw (a rotation's grouped summary); nil for most jobs.
	Summary any
}

// Job is one running or finished job.
type Job struct {
	id      string
	command string
	args    []string
	started time.Time

	mu       sync.Mutex
	summary  any
	lines    []string
	partial  string
	state    string
	err      string
	opID     string
	question *Question
	changed  chan struct{} // closed and replaced on every update
	answer   chan bool
	cancel   bool
	progress *Progress
	items    []string
	failed   map[string]bool
	title    string
}

// SetSummary gives the job a result for its page to draw, above its output.
func (j *Job) SetSummary(v any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.summary = v
	j.notify()
}

// SetTitle names the job for people, as its review did: "Rotate 10 keys".
func (j *Job) SetTitle(title string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.title = title
	j.notify()
}

// Plan names the things the job will work through, in order, so its page can
// list them one per row: waiting, running, done or failed. Progress moves
// through them.
func (j *Job) Plan(items []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.items = append([]string(nil), items...)
	if j.progress == nil {
		j.progress = &Progress{Since: time.Now(), Total: len(items)}
	}
	j.notify()
}

// Failed marks one of the planned items as failed; the rest go on.
func (j *Job) Failed(item string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failed == nil {
		j.failed = map[string]bool{}
	}
	j.failed[item] = true
	j.notify()
}

// Progress records that done of total steps are finished and label is the
// one under way, for the job's page to show as a bar with the time left.
// A job whose work is one step does not call it.
func (j *Job) Progress(done, total int, label string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.progress == nil {
		j.progress = &Progress{Since: time.Now()}
	}
	j.progress.Done, j.progress.Total, j.progress.Label = done, total, label
	j.notify()
}

// Write appends output, one line at a time, so a page can stream it.
func (j *Job) Write(b []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	text := j.partial + string(b)
	parts := strings.Split(text, "\n")
	j.partial = parts[len(parts)-1]
	j.lines = append(j.lines, parts[:len(parts)-1]...)
	j.notify()
	return len(b), nil
}

// Ask pauses the job until the person answers, and reports their answer. A
// cancel answers no.
func (j *Job) Ask(prompt, text string) bool {
	return j.AskWith(Question{Prompt: prompt, Text: text})
}

// AskWith is Ask with the question's own answer labels.
func (j *Job) AskWith(q Question) bool {
	j.mu.Lock()
	if j.cancel {
		j.mu.Unlock()
		return false
	}
	j.question = &q
	j.state = Waiting
	j.notify()
	j.mu.Unlock()
	yes := <-j.answer
	j.mu.Lock()
	j.question, j.state = nil, Running
	j.notify()
	j.mu.Unlock()
	return yes
}

// Step reports ErrCancelled once the job has been cancelled; a job calls it
// between its steps.
func (j *Job) Step() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancel {
		return ErrCancelled
	}
	return nil
}

// View is the job as it is now, and a channel closed at its next change.
func (j *Job) View() (View, <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	lines := append([]string(nil), j.lines...)
	if j.partial != "" && j.finished() {
		lines = append(lines, j.partial)
	}
	v := View{ID: j.id, Command: j.command, Title: j.title, Args: j.args, State: j.state, Err: j.err, OpID: j.opID,
		Lines: lines, Started: j.started, Summary: j.summary}
	if j.question != nil {
		q := *j.question
		v.Question = &q
	}
	if j.progress != nil {
		pr := *j.progress
		pr.Items = j.itemStates(pr.Done)
		v.Progress = &pr
	}
	return v, j.changed
}

// itemStates is each planned item's state: those before done are done (or
// failed), the next is running while the job runs, the rest wait. Called with
// mu held.
func (j *Job) itemStates(done int) []Item {
	if len(j.items) == 0 {
		return nil
	}
	out := make([]Item, len(j.items))
	for i, name := range j.items {
		st := ItemWaiting
		switch {
		case j.failed[name]:
			st = ItemFailed
		case i < done:
			st = ItemDone
		case i == done && !j.finished():
			st = ItemRunning
		}
		out[i] = Item{Name: name, State: st}
	}
	return out
}

func (j *Job) finished() bool { return j.state == Done || j.state == Failed || j.state == Cancelled }

// notify wakes everyone waiting on the job. Called with mu held.
func (j *Job) notify() {
	close(j.changed)
	j.changed = make(chan struct{})
}

// Registry holds the jobs a server has started, and allows one to run at a
// time.
type Registry struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	order   []string
	running *Job
}

// NewRegistry is an empty registry.
func NewRegistry() *Registry { return &Registry{jobs: map[string]*Job{}} }

// keep is how many finished jobs a registry remembers.
const keep = 50

// ErrBusy names the job already running.
type ErrBusy struct{ ID, Command string }

func (e ErrBusy) Error() string {
	return brand.Line(e.Command) + " is still running; one change runs at a time"
}

// Start runs run as a job on p, under the lock, as one operation in the
// history. It refuses while another job runs, and waits briefly for a preview
// or change in flight. The job runs on after Start returns.
func (r *Registry) Start(p paths.Paths, scope mutation.Scope, run func(j *Job) error) (*Job, error) {
	r.mu.Lock()
	if r.running != nil {
		busy := ErrBusy{ID: r.running.id, Command: r.running.command}
		r.mu.Unlock()
		return nil, busy
	}
	r.mu.Unlock()
	release, err := mutation.Exclusive(10 * time.Second)
	if err != nil {
		return nil, err
	}
	if scope.OpID == "" {
		scope.OpID = history.NewOpID()
	}
	j := &Job{id: newID(), command: scope.Command, args: scope.Args, started: time.Now(),
		state: Running, opID: scope.OpID, changed: make(chan struct{}), answer: make(chan bool, 1)}
	r.mu.Lock()
	r.running = j
	r.jobs[j.id] = j
	r.order = append(r.order, j.id)
	for len(r.order) > keep {
		delete(r.jobs, r.order[0])
		r.order = r.order[1:]
	}
	r.mu.Unlock()
	go func() {
		defer release()
		err := mutation.Scoped(p, scope, func() error { return run(j) })
		j.mu.Lock()
		switch {
		case errors.Is(err, ErrCancelled):
			j.state, j.err = Cancelled, err.Error()
		case err != nil:
			j.state, j.err = Failed, err.Error()
		default:
			j.state = Done
		}
		j.notify()
		j.mu.Unlock()
		r.mu.Lock()
		r.running = nil
		r.mu.Unlock()
	}()
	return j, nil
}

// Get is the job with id, or nil.
func (r *Registry) Get(id string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.jobs[id]
}

// Running is the job running now, or nil.
func (r *Registry) Running() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Answer answers the question a job is waiting on. It reports false when the
// job is not waiting.
func (r *Registry) Answer(id string, yes bool) bool {
	j := r.Get(id)
	if j == nil {
		return false
	}
	j.mu.Lock()
	waiting := j.state == Waiting
	j.mu.Unlock()
	if !waiting {
		return false
	}
	select {
	case j.answer <- yes:
		return true
	default:
		return false // answered already
	}
}

// Cancel stops a job at its next step, answering no to a question it waits on.
func (r *Registry) Cancel(id string) bool {
	j := r.Get(id)
	if j == nil {
		return false
	}
	j.mu.Lock()
	if j.finished() {
		j.mu.Unlock()
		return false
	}
	j.cancel = true
	waiting := j.state == Waiting
	j.notify()
	j.mu.Unlock()
	if waiting {
		select {
		case j.answer <- false:
		default:
		}
	}
	return true
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
