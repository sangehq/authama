package configsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/util/fs"
)

// RecordFile is where the state home keeps what authama last rendered into
// each managed file. It is how a render tells a line it wrote itself from one
// a person typed into the file since.
const RecordFile = "render-record.json"

// HandEditFix is what to run about hand edits in a managed file.
const HandEditFix = "keep them with `authama ssh config adopt-edits` (it adds them to the manifest), " +
	"or discard them with `authama ssh config render --force` (the file is backed up first)"

// HandEdit is one thing typed into a managed file by hand that the next
// render would remove: a Host block the manifest does not have, or a line a
// managed block did not carry when authama wrote it.
type HandEdit struct {
	File  string
	Alias string
	What  string
}

func (h HandEdit) String() string {
	if h.Alias == "" {
		return h.File + ": " + h.What
	}
	return h.File + ": " + h.Alias + ": " + h.What
}

// HandEditsError is a render refused because it would discard hand edits.
type HandEditsError struct{ Edits []HandEdit }

func (e *HandEditsError) Error() string {
	var b strings.Builder
	b.WriteString("the managed SSH config was edited by hand, and rendering would discard it:\n")
	for _, h := range e.Edits {
		b.WriteString("  " + h.String() + "\n")
	}
	b.WriteString(HandEditFix)
	return b.String()
}

// IsHandEdits reports whether err is a render refused over hand edits.
func IsHandEdits(err error) bool {
	var he *HandEditsError
	return errors.As(err, &he)
}

// WithState keeps the render record in stateDir: every write records what it
// wrote, and a write refuses to replace hand edits (see HandEdits). Without
// it the service neither records nor guards, as before.
func (s *Service) WithState(stateDir string) *Service {
	s.stateDir = stateDir
	return s
}

// Force makes a write replace hand edits after copying each edited file into
// backupDir ("" for no copy, when the caller keeps one of its own).
func (s *Service) Force(backupDir string) *Service {
	s.force, s.backupDir = true, backupDir
	return s
}

type renderRecord struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

func (s *Service) loadRecord() map[string]string {
	if s.stateDir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(s.stateDir, RecordFile))
	if err != nil {
		return nil
	}
	var r renderRecord
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return r.Files
}

func (s *Service) saveRecord(rendered map[string]string) error {
	if s.stateDir == "" {
		return nil
	}
	if err := fs.EnsureDir(s.stateDir, dirMode); err != nil {
		return err
	}
	b, err := json.MarshalIndent(renderRecord{Version: 1, Files: rendered}, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteTextAtomic(filepath.Join(s.stateDir, RecordFile), string(b)+"\n", configMode)
}

// managedPart is the part of a managed file on disk that a render replaces.
func managedPart(rel, content string) string {
	if rel == renderer.RootConfig {
		return renderer.ManagedRegion(content)
	}
	if !renderer.IsManagedFile(content) {
		return ""
	}
	return content
}

// HandEdits lists what a person typed into the managed files that the next
// render would discard.
//
// Each file is compared with what authama last wrote into it (the render
// record). Anything there that was not - a Host block, a name on a Host
// line, an option, a changed value, a heading or comment - is a hand edit.
// A file written before the record existed is compared with the render
// instead, and then only what the manifest has no trace of counts: a block
// or a name it does not render, an option keyword its block does not set, a
// heading or comment it does not write. A changed value there reads as an
// older render, which the manifest has since moved on from, and so does a
// block in the exact shape authama writes (its UserKnownHostsFile line is one
// no hand-written block needs): a host deleted before the record existed.
func (s *Service) HandEdits() ([]HandEdit, error) {
	rendered, err := s.Rendered()
	if err != nil {
		return nil, err
	}
	record := s.loadRecord()
	var out []HandEdit
	for _, rel := range sortedKeys(rendered) {
		current, ok := readFile(filepath.Join(s.sshDir, filepath.FromSlash(rel)))
		if !ok {
			continue
		}
		disk := managedPart(rel, current)
		if disk == "" || disk == rendered[rel] {
			continue
		}
		baseline, recorded := record[rel]
		if recorded && disk == baseline {
			continue
		}
		if !recorded {
			baseline = rendered[rel]
		}
		out = append(out, diffBlocks(rel, renderer.ParseManaged(disk), renderer.ParseManaged(baseline), recorded)...)
	}
	return out, nil
}

func diffBlocks(rel string, disk, base renderer.Parsed, exact bool) []HandEdit {
	var out []HandEdit
	byAlias := map[string]renderer.Block{}
	names := map[string]bool{}
	sections := map[string]bool{}
	for _, b := range base.Blocks {
		byAlias[strings.ToLower(b.Alias())] = b
		for _, p := range b.Patterns {
			names[strings.ToLower(p)] = true
		}
		sections[b.Section] = true
	}
	loose := map[string]bool{}
	for _, l := range base.Loose {
		loose[l] = true
	}
	for _, d := range disk.Blocks {
		alias := d.Alias()
		if d.Section != "" && !sections[d.Section] {
			out = append(out, HandEdit{rel, alias, "adds the heading # " + d.Section})
		}
		b, ok := byAlias[strings.ToLower(alias)]
		if !ok {
			if !exact && looksRendered(d) {
				continue
			}
			out = append(out, HandEdit{rel, alias, "a Host block the manifest does not have"})
			continue
		}
		for _, p := range d.Patterns[1:] {
			if !names[strings.ToLower(p)] {
				out = append(out, HandEdit{rel, alias, "adds the name " + p + " to its Host line"})
			}
		}
		if d.Comment != "" && d.Comment != b.Comment {
			out = append(out, HandEdit{rel, alias, "comment # " + d.Comment})
		}
		for _, o := range d.Options {
			v, has := b.Get(o.Key)
			switch {
			case !has:
				out = append(out, HandEdit{rel, alias, "adds " + o.Key + " " + o.Value})
			case exact && !hasValue(b, o):
				out = append(out, HandEdit{rel, alias, fmt.Sprintf("sets %s %s (authama wrote %s)", o.Key, o.Value, v)})
			}
		}
	}
	for _, l := range disk.Loose {
		if !loose[l] {
			out = append(out, HandEdit{rel, "", "comment # " + strings.TrimPrefix(l, "# ")})
		}
	}
	return out
}

// looksRendered reports whether a block has the lines authama writes into
// every host it renders, the way it writes them.
func looksRendered(b renderer.Block) bool {
	kh, _ := b.Get("UserKnownHostsFile")
	_, file := b.Get("IdentityFile")
	_, only := b.Get("IdentitiesOnly")
	return kh == "~/.ssh/known_hosts" && file && only
}

func hasValue(b renderer.Block, o renderer.OptionLine) bool {
	for _, x := range b.Options {
		if strings.EqualFold(x.Key, o.Key) && x.Value == o.Value {
			return true
		}
	}
	return false
}

// backupEdited copies each file with hand edits into s.backupDir.
func (s *Service) backupEdited(edits []HandEdit) error {
	if s.backupDir == "" {
		return nil
	}
	seen := map[string]bool{}
	var files []string
	for _, e := range edits {
		if !seen[e.File] {
			seen[e.File] = true
			files = append(files, e.File)
		}
	}
	sort.Strings(files)
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(s.sshDir, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		dst := filepath.Join(s.backupDir, filepath.FromSlash(rel))
		if err := fs.EnsureDir(filepath.Dir(dst), dirMode); err != nil {
			return err
		}
		if err := fs.WriteTextAtomic(dst, string(b), configMode); err != nil {
			return fmt.Errorf("could not back %s up, so it was not replaced: %w", rel, err)
		}
	}
	return nil
}
