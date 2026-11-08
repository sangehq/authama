// Package checklist answers, for anything authama can revoke (an SSH key, an
// auth token), where it is used and what is left to do to rotate or revoke it
// completely. Steps authama can see for itself (a key file on disk, a target it
// was removed from, a token still in a tool's file) are worked out each time;
// the ones it cannot see (the token's issuer, a copy in Termius, a CI secret)
// are ticked by hand, and those ticks are kept in the state directory.
//
// An item is a key or a token. A key's checklist follows its life: while in use
// it lists where the key is, and once revoked or replaced by a rotation it
// lists what still has to go. A token has no revoked state of its own, so its
// revocation checklist starts when you say you are revoking it.
package checklist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// State is how far one step is.
type State string

const (
	// Done needs nothing more.
	Done State = "done"
	// Todo is still to do.
	Todo State = "todo"
	// Unknown is a step authama could not check this time (no ssh-agent, a file
	// it could not read).
	Unknown State = "unknown"
)

// Modes an item's checklist is in.
const (
	// InUse lists where an item is, and that each place is sound.
	InUse = "in-use"
	// Revoking is a token whose revocation has been started.
	Revoking = "revoking"
	// Revoked is a key revoked with `authama ssh key revoke`.
	Revoked = "revoked"
	// Retired is a key a rotation replaced, kept in old/ for rollback.
	Retired = "retired"
)

// Kinds of item.
const (
	KindKey   = "key"
	KindToken = "token"
)

// Step is one thing to check or do.
type Step struct {
	ID     string `json:"id"`
	Group  string `json:"group"` // the heading it is shown under
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	State  State  `json:"state"`
	// Manual steps are ones authama cannot see: the person ticks them.
	Manual bool `json:"manual,omitempty"`
	// Ticked is when a manual step was ticked, "" when it was not.
	Ticked string `json:"ticked,omitempty"`
	// Fix is the authama (or other) command that does the step.
	Fix string `json:"fix,omitempty"`
	// Link is where the step is done by hand.
	Link string `json:"link,omitempty"`
}

// Checklist is one item's steps.
type Checklist struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`   // key:<fingerprint or profile/key>, token:<target>/<host>
	Name        string `json:"name"` // profile/key, or "<target> <host>"
	Mode        string `json:"mode"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Since       string `json:"since,omitempty"` // when it was revoked, retired or its revocation started
	Steps       []Step `json:"steps"`
}

// Counts is how many steps are done, and how many there are.
func (c Checklist) Counts() (done, total int) {
	for _, s := range c.Steps {
		total++
		if s.State == Done {
			done++
		}
	}
	return done, total
}

// Open is how many steps still need something.
func (c Checklist) Open() int {
	done, total := c.Counts()
	return total - done
}

// Complete reports whether every step is done.
func (c Checklist) Complete() bool { return c.Open() == 0 }

// Groups returns the steps by group, in the order groups first appear.
func (c Checklist) Groups() []Group {
	var out []Group
	at := map[string]int{}
	for _, s := range c.Steps {
		i, ok := at[s.Group]
		if !ok {
			i = len(out)
			at[s.Group] = i
			out = append(out, Group{Name: s.Group})
		}
		out[i].Steps = append(out[i].Steps, s)
	}
	return out
}

// Group is the steps under one heading.
type Group struct {
	Name  string
	Steps []Step
}

// ---- the hand-kept part ----------------------------------------------------

// itemState is what the person said about one item: the manual steps they
// ticked, and when they started revoking it (tokens).
type itemState struct {
	Revoking string            `json:"revoking,omitempty"`
	Ticks    map[string]string `json:"ticks,omitempty"`
}

// store is checklists.json, by item id.
type store map[string]itemState

// StorePath is checklists.json in the state directory: what the person ticked
// is about this machine's work, not the manifest's, so it does not travel
// with a bundle.
func StorePath(p paths.Paths) string { return filepath.Join(p.StateDir(), "checklists.json") }

func loadStore(path string) (store, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s is not a checklist file: %w", path, err)
	}
	if s == nil {
		s = store{}
	}
	return s, nil
}

func (s store) save(path string) error {
	for id, st := range s {
		if st.Revoking == "" && len(st.Ticks) == 0 {
			delete(s, id)
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(path, string(b)+"\n", 0o600)
}

// applyTicks marks the manual steps the person ticked as done.
func applyTicks(c *Checklist, st itemState) {
	for i := range c.Steps {
		if !c.Steps[i].Manual {
			continue
		}
		if at, ok := st.Ticks[c.Steps[i].ID]; ok {
			c.Steps[i].State, c.Steps[i].Ticked = Done, at
		} else {
			c.Steps[i].State = Todo
		}
	}
}

// ---- the service -----------------------------------------------------------

// ErrNotFound is an item id that names nothing authama knows.
var ErrNotFound = errors.New("no key or token by that name")

// Service builds checklists for one authama home.
type Service struct {
	P paths.Paths
	// Now is the clock, for expiry and for the date a step is ticked.
	Now func() time.Time
	// AgentFingerprints lists the fingerprints ssh-agent holds; ok is false
	// when there is no agent to ask. Replaced in tests.
	AgentFingerprints func() (fps map[string]bool, ok bool)
	// Getenv reads the environment, for whether a token's variable is set.
	Getenv func(string) string
}

// New returns the service for the home at p.
func New(p paths.Paths) *Service {
	return &Service{P: p, Now: time.Now, AgentFingerprints: agentFingerprints, Getenv: os.Getenv}
}

// All is every key's and token's checklist: keys in manifest order, then
// retired and revoked keys, then tokens by target and host.
func (s *Service) All() ([]Checklist, error) {
	st, err := loadStore(StorePath(s.P))
	if err != nil {
		return nil, err
	}
	keys, err := s.keyChecklists()
	if err != nil {
		return nil, err
	}
	toks := s.tokenChecklists(st)
	all := append(keys, toks...)
	for i := range all {
		applyTicks(&all[i], st[all[i].ID])
	}
	return all, nil
}

// Get is one item's checklist by id, or by a key's profile/key reference.
func (s *Service) Get(id string) (Checklist, error) {
	all, err := s.All()
	if err != nil {
		return Checklist{}, err
	}
	for _, c := range all {
		if c.ID == id {
			return c, nil
		}
	}
	// A key may be named by its profile/key; prefer the one in use.
	var hit *Checklist
	for i := range all {
		c := &all[i]
		if c.Kind == KindKey && (c.Name == id || "key:"+c.Name == id) {
			if hit == nil || c.Mode == InUse {
				hit = c
			}
		}
	}
	if hit != nil {
		return *hit, nil
	}
	return Checklist{}, fmt.Errorf("%w: %q", ErrNotFound, id)
}

// Tick marks a manual step done (on) or not done.
func (s *Service) Tick(id, step string, on bool) (Checklist, error) {
	c, err := s.Get(id)
	if err != nil {
		return Checklist{}, err
	}
	var found *Step
	for i := range c.Steps {
		if c.Steps[i].ID == step {
			found = &c.Steps[i]
		}
	}
	switch {
	case found == nil:
		return Checklist{}, fmt.Errorf("%s has no step %q", c.Name, step)
	case !found.Manual:
		return Checklist{}, fmt.Errorf("authama checks %q for itself, so it cannot be ticked by hand", found.Title)
	}
	path := StorePath(s.P)
	st, err := loadStore(path)
	if err != nil {
		return Checklist{}, err
	}
	is := st[c.ID]
	if on {
		if is.Ticks == nil {
			is.Ticks = map[string]string{}
		}
		is.Ticks[step] = s.Now().UTC().Format(time.RFC3339)
	} else {
		delete(is.Ticks, step)
	}
	st[c.ID] = is
	if err := st.save(path); err != nil {
		return Checklist{}, err
	}
	return s.Get(c.ID)
}

// SetRevoking starts (on) or stops a token's revocation checklist. A key's
// follows its own state, so this is for tokens only.
func (s *Service) SetRevoking(id string, on bool) (Checklist, error) {
	c, err := s.Get(id)
	if err != nil {
		return Checklist{}, err
	}
	if c.Kind != KindToken {
		return Checklist{}, errors.New("a key's checklist follows the key: revoke it with `authama ssh key revoke`, or rotate it")
	}
	path := StorePath(s.P)
	st, err := loadStore(path)
	if err != nil {
		return Checklist{}, err
	}
	is := st[c.ID]
	if on {
		if is.Revoking == "" {
			is.Revoking = s.Now().UTC().Format("2006-01-02")
		}
	} else {
		is.Revoking = ""
	}
	st[c.ID] = is
	if err := st.save(path); err != nil {
		return Checklist{}, err
	}
	return s.Get(c.ID)
}

// Summary is one line per item: what it is, its mode and its progress.
func Summary(all []Checklist) string {
	var b strings.Builder
	for _, c := range all {
		done, total := c.Counts()
		fmt.Fprintf(&b, "%-6s %-9s %3d/%-3d %s\n", c.Kind, c.Mode, done, total, c.Name)
	}
	return b.String()
}

// Format renders one checklist for the terminal.
func Format(c Checklist) string {
	var b strings.Builder
	done, total := c.Counts()
	fmt.Fprintf(&b, "%s %s: %s, %d of %d done\n", c.Kind, c.Name, c.Mode, done, total)
	if c.Fingerprint != "" {
		fmt.Fprintf(&b, "  %s\n", c.Fingerprint)
	}
	for _, g := range c.Groups() {
		fmt.Fprintf(&b, "\n%s\n", g.Name)
		for _, st := range g.Steps {
			mark := map[State]string{Done: "[x]", Todo: "[ ]", Unknown: "[?]"}[st.State]
			who := ""
			if st.Manual {
				who = " (by hand: " + st.ID + ")"
			}
			fmt.Fprintf(&b, "  %s %s%s\n", mark, st.Title, who)
			if st.Detail != "" {
				fmt.Fprintf(&b, "      %s\n", st.Detail)
			}
			if st.State != Done && st.Fix != "" {
				fmt.Fprintf(&b, "      run: %s\n", st.Fix)
			}
			if st.State != Done && st.Link != "" {
				fmt.Fprintf(&b, "      at: %s\n", st.Link)
			}
		}
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
