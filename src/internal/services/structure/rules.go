package structure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// Rule is a learned or user-written purpose classification.
// AutoApply is off unless the owner turns it on: then a later identical
// suggestion (confidence 1.0, complete rewrite set) is applied with a backup
// and a history entry, never silently.
type Rule struct {
	ID          string `json:"id"`
	Profile     string `json:"profile,omitempty"`
	NameGlob    string `json:"name,omitempty"`
	CommentRe   string `json:"comment,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Tool        string `json:"tool"`
	ToolKind    string `json:"tool_kind,omitempty"`
	AutoApply   bool   `json:"auto_apply,omitempty"`
}

func rulesFile(p paths.Paths) string {
	return filepath.Join(p.StateDir(), "purpose-rules.json")
}

// LoadRules reads the owner's purpose rules; a missing file is empty.
func LoadRules(p paths.Paths) ([]Rule, error) {
	b, err := os.ReadFile(rulesFile(p))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rs []Rule
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

func saveRules(p paths.Paths, rs []Rule) error {
	if err := os.MkdirAll(p.StateDir(), perms.DirMode); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteTextAtomic(rulesFile(p), string(b)+"\n", perms.ConfigMode)
}

func ruleID(r Rule) string {
	sum := sha256.Sum256([]byte(r.Profile + "\x00" + r.NameGlob + "\x00" + r.CommentRe + "\x00" + r.Fingerprint + "\x00" + r.Tool + "\x00" + r.ToolKind))
	return hex.EncodeToString(sum[:])[:8]
}

// AddRule stores a rule and returns its id.
func AddRule(p paths.Paths, r Rule) (string, error) {
	if err := manifest.CheckToolSlug(r.Tool); err != nil {
		return "", err
	}
	if r.CommentRe != "" {
		if _, err := regexp.Compile(r.CommentRe); err != nil {
			return "", fmt.Errorf("comment: %w", err)
		}
	}
	r.ID = ruleID(r)
	rs, err := LoadRules(p)
	if err != nil {
		return "", err
	}
	for _, e := range rs {
		if e.ID == r.ID {
			return r.ID, nil
		}
	}
	rs = append(rs, r)
	return r.ID, saveRules(p, rs)
}

// SetRuleAuto turns auto-apply on or off for a rule.
func SetRuleAuto(p paths.Paths, id string, auto bool) error {
	rs, err := LoadRules(p)
	if err != nil {
		return err
	}
	for i := range rs {
		if rs[i].ID == id || (len(id) >= 4 && strings.HasPrefix(rs[i].ID, id)) {
			rs[i].AutoApply = auto
			return saveRules(p, rs)
		}
	}
	return fmt.Errorf("no rule %s", id)
}

// RemoveRule deletes a rule by id or prefix.
func RemoveRule(p paths.Paths, id string) error {
	rs, err := LoadRules(p)
	if err != nil {
		return err
	}
	var out []Rule
	found := false
	for _, r := range rs {
		if r.ID == id || (len(id) >= 4 && strings.HasPrefix(r.ID, id)) {
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return fmt.Errorf("no rule %s", id)
	}
	return saveRules(p, out)
}

func matchRule(r Rule, profile, name, comment, fingerprint string) bool {
	if r.Profile != "" && r.Profile != profile {
		return false
	}
	if r.NameGlob != "" {
		ok, err := filepath.Match(r.NameGlob, name)
		if err != nil || !ok {
			return false
		}
	}
	if r.CommentRe != "" {
		re, err := regexp.Compile(r.CommentRe)
		if err != nil || !re.MatchString(comment) {
			return false
		}
	}
	if r.Fingerprint != "" && r.Fingerprint != fingerprint {
		return false
	}
	// A rule must have at least one matcher besides the destination.
	return r.NameGlob != "" || r.CommentRe != "" || r.Fingerprint != "" || r.Profile != ""
}

func applyRules(p paths.Paths, profile, name, comment, fingerprint string, c *classified) {
	rs, err := LoadRules(p)
	if err != nil {
		return
	}
	for _, r := range rs {
		if !matchRule(r, profile, name, comment, fingerprint) {
			continue
		}
		c.tool, c.kind, c.confidence = r.Tool, r.ToolKind, 1.0
		c.evidence = append([]string{"rule " + r.ID}, c.evidence...)
		return
	}
}
