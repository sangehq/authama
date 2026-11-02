package tokensvc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/util/fs"
)

// AliasBackupSuffix names the copy of a startup file taken before an edit.
const AliasBackupSuffix = ".authama.bak"

// Profile is a shell startup file the aliases go in.
type Profile struct {
	env  Env
	File string
}

// Profile picks the startup file: file when given, else the shell's rc (or
// login profile with login).
func (e Env) Profile(file string, login bool) Profile {
	if file != "" {
		return Profile{e, e.abs(file)}
	}
	return Profile{e, tokens.DefaultProfile(e.Home, e.getenv("SHELL"), login, exists)}
}

// AliasStatus is what a startup file holds.
type AliasStatus struct {
	File    string
	Exists  bool
	Present bool
	Legacy  bool // an auth-token-manager block is still there
	Lines   []string
}

// Status reports the alias block in the file.
func (pr Profile) Status() AliasStatus {
	file := pr.File
	text := readOrEmpty(file)
	return AliasStatus{
		File: file, Exists: exists(file), Present: tokens.HasAliasBlock(text),
		Legacy: tokens.HasLegacyAliasBlock(text), Lines: tokens.BlockAliasLines(text),
	}
}

// Install writes the block into the file (replacing an old one, and an
// auth-token-manager block). updated is set when a block was already there.
func (pr Profile) Install(command string, backup bool) (updated bool, warns []string, err error) {
	text := readOrEmpty(pr.File)
	updated = tokens.HasAliasBlock(text) || tokens.HasLegacyAliasBlock(text)
	block := tokens.AliasBlock(tokens.DefaultAliases(command), command)
	warns, err = pr.write(tokens.RenderAliasInstall(text, block), backup)
	return updated, warns, err
}

// Remove deletes the block from the file; removed is false when there was
// none.
func (pr Profile) Remove(backup bool) (removed bool, err error) {
	text := readOrEmpty(pr.File)
	if !tokens.HasAliasBlock(text) && !tokens.HasLegacyAliasBlock(text) {
		return false, nil
	}
	_, err = pr.write(tokens.RenderAliasRemove(text), backup)
	return err == nil, err
}

func readOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// write replaces the startup file atomically, keeping its mode (0644 for a
// new one) and backing it up first.
func (pr Profile) write(text string, backup bool) ([]string, error) {
	path := pr.File
	if err := pr.env.Guard(path); err != nil {
		return nil, err
	}
	var warns []string
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
		if backup {
			if err := copyFile(path, path+AliasBackupSuffix, mode); err != nil {
				warns = append(warns, fmt.Sprintf("could not back up %s: %v", path, err))
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return warns, fs.WriteTextAtomic(path, text, mode)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
