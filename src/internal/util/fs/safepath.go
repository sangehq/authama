package fs

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// The path guards every name and relative path from a manifest, a setup
// document or a web form goes through before it becomes part of a path. The
// manifest validator already refuses a bad name when a file is loaded; these
// keep a path inside its root even for one that skipped it (a manifest built
// in memory, a document being previewed). Each is a check static analysis
// recognises (filepath.IsLocal, strings.HasPrefix, a regexp match).

// InvalidSegment stands in for a name that is not one plain path element. It
// is not a name any validated manifest holds, so it matches no file the caller
// meant, and it keeps the path it is joined into inside its directory.
const InvalidSegment = "_invalid-name_"

// Segment is name when it is one plain path element: not empty, ".", "..",
// absolute, or holding a separator or a NUL. Anything else is InvalidSegment.
func Segment(name string) string {
	if name == "." || strings.ContainsAny(name, "/\\\x00") {
		return InvalidSegment
	}
	if filepath.IsLocal(name) {
		return name
	}
	return InvalidSegment
}

// LocalRel is rel, a forward-slash path relative to a root, cleaned, when it
// stays inside that root: not absolute, no "..", no NUL. Anything else is
// InvalidSegment.
func LocalRel(rel string) string {
	if strings.Contains(rel, "\x00") {
		return InvalidSegment
	}
	native := filepath.FromSlash(rel)
	if filepath.IsLocal(native) {
		return filepath.ToSlash(filepath.Clean(native))
	}
	return InvalidSegment
}

// ErrOutside is Within refusing a path outside its root.
var ErrOutside = errors.New("outside its root")

// Within is p, cleaned, when it is root or inside it; an error otherwise. root
// is cleaned too, and both should be absolute.
func Within(root, p string) (string, error) {
	root, p = filepath.Clean(root), filepath.Clean(p)
	if p == root {
		return root, nil
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if strings.HasPrefix(p, prefix) {
		return p, nil
	}
	return "", fmt.Errorf("%s: %w %s", p, ErrOutside, root)
}

// plainDir is an absolute path with no control character: a drive or a
// slash first, so it can never read as an option.
var plainDir = regexp.MustCompile(`^(/|[A-Za-z]:[\\/]|\\\\)[^\x00-\x1f\x7f]*$`)

// AbsDir is dir made absolute and cleaned, when it holds no control
// character; an error otherwise. It is for a directory a person names on
// purpose (a git checkout), which may be anywhere they can read.
func AbsDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if plainDir.MatchString(abs) {
		return abs, nil
	}
	return "", fmt.Errorf("%q is not a plain absolute directory", dir)
}
