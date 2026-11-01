package tokensvc

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/fs"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/log"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/perms"
)

// SecretMode is the mode every rendered credential file gets.
const SecretMode os.FileMode = 0o600

// readExisting reads a target file for merging. A file that exists but
// cannot be read is an error: writing over it would drop every credential
// in it.
func readExisting(path string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("cannot read existing %s to merge into — refusing to overwrite it: %w", path, err)
	}
	return string(b), true, nil
}

// writeSecret writes text (plus a final newline) atomically and owner-only.
// A failure to harden is a warning, not an error: the file is written.
func (e Env) writeSecret(path, text string) ([]string, error) {
	if err := e.Guard(path); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := fs.WriteTextAtomic(path, text, SecretMode); err != nil {
		return nil, err
	}
	if err := perms.SetPerms(path, SecretMode); err != nil {
		return []string{fmt.Sprintf("could not restrict %s to the owner: %v", path, err)}, nil
	}
	return nil, nil
}

// WriteTarget merges set's entries for target into path. Docker hosts whose
// credentials are delegated to a credential store are not written here; see
// SyncDocker.
func (e Env) WriteTarget(set *tokens.Set, target, path string) ([]string, error) {
	if err := e.Guard(path); err != nil {
		return nil, err
	}
	existing, present, err := readExisting(path)
	if err != nil {
		return nil, err
	}
	text, err := set.Render(target, existing, present)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	warns, err := e.writeSecret(path, text)
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, set.Len(target))
	for _, c := range set.Entries(target) {
		hosts = append(hosts, c.Host)
	}
	e.audit("token.write", log.Field{Key: "target", Value: target},
		log.Field{Key: "hosts", Value: strings.Join(hosts, ",")}, log.Field{Key: "file", Value: path})
	return warns, nil
}

func (e Env) audit(event string, fields ...log.Field) {
	log.Audit(e.Paths.AuditLog(), event, fields...)
}

// Inventory lists every credential installed in the target files. Secrets
// are blanked unless reveal is set.
func (e Env) Inventory(project string, reveal bool) []tokens.Installed {
	var out []tokens.Installed
	for _, t := range tokens.Targets() {
		path := e.TargetPath(t, project)
		fi, err := os.Stat(path)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, it := range tokens.Scan(t, string(b)) {
			it.File = path
			if !reveal {
				it.Secret = ""
			}
			out = append(out, it)
		}
	}
	return out
}

// Find returns the installed credential for (target, host), secret included.
func (e Env) Find(target, host, project string) (tokens.Installed, bool) {
	host = tokens.CanonicalHost(target, host)
	for _, it := range e.Inventory(project, true) {
		if it.Target == target && it.Host == host {
			return it, true
		}
	}
	return tokens.Installed{}, false
}

// Forget deletes the credential for host from target's file, rewritten
// atomically and owner-only. It reports whether host was there.
func (e Env) Forget(target, host, project string) (bool, error) {
	if !tokens.IsTarget(target) {
		return false, fmt.Errorf("unknown target %q (choose from %s)", target, strings.Join(tokens.Targets(), ", "))
	}
	path := e.TargetPath(target, project)
	if err := e.Guard(path); err != nil {
		return false, err
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	host = tokens.CanonicalHost(target, host)
	found := false
	for _, it := range tokens.Scan(target, string(b)) {
		if it.Host == host {
			found = true
		}
	}
	if !found {
		return false, nil
	}
	if _, err := e.writeSecret(path, tokens.Purge(target, host, string(b))); err != nil {
		return false, err
	}
	e.audit("token.delete", log.Field{Key: "target", Value: target},
		log.Field{Key: "host", Value: host}, log.Field{Key: "file", Value: path})
	return true, nil
}
