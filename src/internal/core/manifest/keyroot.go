package manifest

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultKeyRoot is the directory under ~/.ssh that holds the profile
// directories when defaults.key_root is unset: ~/.ssh/profiles/.
const DefaultKeyRoot = "profiles"

// KeyRoot is the directory under ~/.ssh that holds the profile directories:
// defaults.key_root, or DefaultKeyRoot when it is unset or m is nil. It is the
// one place the root is decided; every path into the key tree is built from
// it, and a guard test fails on a "profiles" literal anywhere else.
func KeyRoot(m *Manifest) string {
	if m == nil || m.Defaults.KeyRoot == "" {
		return DefaultKeyRoot
	}
	return m.Defaults.KeyRoot
}

// KeyRootAt is the key root of the manifest at path, for the scans that start
// from ~/.ssh alone (doctor, snapshots, the key audit, permissions). A
// manifest that is missing or does not load gives DefaultKeyRoot: those scans
// look for what a manifest has forgotten, so they never fail for want of one.
func KeyRootAt(path string) string {
	m, err := Load(path)
	if err != nil {
		return DefaultKeyRoot
	}
	return KeyRoot(m)
}

// sshOwnNames are the names in ~/.ssh that belong to ssh itself, or to
// authama's own files there, and so can never be the key root.
var sshOwnNames = map[string]bool{
	"config": true, "authorized_keys": true, "environment": true, "rc": true,
}

// CheckKeyRoot reports whether root can be defaults.key_root: one directory
// name directly under ~/.ssh, not hidden and not one of ssh's own files. ""
// is the default. A nested root ("a/b") and "." (profile directories loose in
// ~/.ssh, beside the user's own) are refused: a scan of every directory in
// ~/.ssh would take the user's folders for profiles.
func CheckKeyRoot(root string) error {
	if root == "" {
		return nil
	}
	if strings.ContainsAny(root, `/\`) {
		return fmt.Errorf("defaults.key_root=%q: name one directory directly under ~/.ssh, such as \"keys\"", root)
	}
	if root == "." {
		return fmt.Errorf("defaults.key_root=%q: profile directories cannot sit loose in ~/.ssh beside your own files; name a directory, such as \"keys\"", root)
	}
	if err := safeSegment("defaults.key_root", root); err != nil {
		return err
	}
	lower := strings.ToLower(root)
	switch {
	case strings.HasPrefix(root, "."):
		return fmt.Errorf("defaults.key_root=%q: a hidden directory is where authama stages files; name another", root)
	case sshOwnNames[lower] || strings.HasPrefix(lower, "known_hosts") ||
		strings.HasPrefix(lower, "authorized_keys") || strings.HasPrefix(lower, "id_") ||
		strings.HasSuffix(lower, ".pub"):
		return fmt.Errorf("defaults.key_root=%q: that name belongs to ssh's own files; name another directory", root)
	}
	return nil
}

func checkKeyRootFor(m *Manifest) error {
	if err := CheckKeyRoot(m.Defaults.KeyRoot); err != nil {
		return err
	}
	root := KeyRoot(m)
	for _, name := range ReservedKeyNames(m) {
		if name == root {
			return fmt.Errorf("reserved key %q has the same name as the key directory ~/.ssh/%s/", name, root)
		}
	}
	return nil
}

// GlobKeyTree matches the pattern tail inside every profile directory under
// ~/.ssh/<root>, in both layouts: <root>/<profile>/<tail> and
// <root>/<category>/<profile>/<tail>. It needs no manifest, because the scans
// that use it (doctor, snapshots, keyaudit) also look for what the manifest
// has forgotten.
//
// A two-level match whose middle segment is "old" or dot-prefixed is dropped:
// that is a flat profile's rotation archive or staging directory, not a
// categorized profile, and a flat-only glob never saw inside them either.
// The result is sorted.
func GlobKeyTree(sshDir, root string, tail ...string) []string {
	base := filepath.Join(sshDir, root)
	flat, _ := filepath.Glob(filepath.Join(append([]string{base, "*"}, tail...)...))
	nested, _ := filepath.Glob(filepath.Join(append([]string{base, "*", "*"}, tail...)...))
	out := append([]string{}, flat...)
	for _, p := range nested {
		rel, err := filepath.Rel(base, p)
		if err != nil {
			continue
		}
		mid := strings.Split(filepath.ToSlash(rel), "/")[1]
		if mid == "old" || strings.HasPrefix(mid, ".") {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// CategoryConfigRelIn is the forward-slash path, relative to ~/.ssh, of the
// file holding a category's Host blocks under the key root.
func CategoryConfigRelIn(root, category string) string {
	return root + "/" + category + "/config"
}

// CategoryConfigFileIn is the ~ form path the root config Includes for a
// category, matching how IdentityFile paths render.
func CategoryConfigFileIn(root, category string) string {
	return sshToken + "/" + CategoryConfigRelIn(root, category)
}
