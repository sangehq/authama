package tokens

import (
	"path/filepath"
	"strings"
)

// The shell aliases auth-token-manager installed (atm, atm-ui, atm-doctor,
// atm-init) keep their names and now call the token group. They live in a
// marked block in the shell's startup file; installing replaces the block,
// and also an old auth-token-manager block, so a user moving over ends with
// one block, not two.

// The markers around the managed block.
const (
	AliasBegin       = "# >>> authama token aliases >>>"
	AliasEnd         = "# <<< authama token aliases <<<"
	legacyAliasBegin = "# >>> auth-token-manager >>>"
	legacyAliasEnd   = "# <<< auth-token-manager <<<"
)

// Alias is one managed alias.
type Alias struct{ Name, Value string }

// DefaultAliases are the managed aliases for command (the binary name).
func DefaultAliases(command string) []Alias {
	return []Alias{
		{"atm", command + " token"},
		{"atm-ui", command + " tui"},
		{"atm-doctor", command + " token audit"},
		{"atm-init", command + " token init"},
	}
}

func shQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

// AliasLines renders `alias name='value'` lines, for eval or a block.
func AliasLines(as []Alias) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = "alias " + a.Name + "=" + shQuote(a.Value)
	}
	return out
}

// AliasBlock is the marked block for a startup file.
func AliasBlock(as []Alias, command string) string {
	note := "# Managed by `" + command + " token shell-aliases` — run it with --uninstall to delete this block."
	return strings.Join(append(append([]string{AliasBegin, note}, AliasLines(as)...), AliasEnd), "\n")
}

// StripAliasBlock removes the managed block (and the legacy one), with the
// blank lines just before it. Line-based, so hand edits inside survive.
func StripAliasBlock(text string) string {
	var out []string
	end := ""
	for _, line := range pySplitLines(text) {
		st := pyStrip(line)
		if end == "" && (st == AliasBegin || st == legacyAliasBegin) {
			end = AliasEnd
			if st == legacyAliasBegin {
				end = legacyAliasEnd
			}
			for len(out) > 0 && pyStrip(out[len(out)-1]) == "" {
				out = out[:len(out)-1]
			}
			continue
		}
		if end != "" {
			if st == end {
				end = ""
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// RenderAliasInstall is text with the block appended, replacing any old one.
func RenderAliasInstall(text, block string) string {
	base := strings.TrimRight(StripAliasBlock(text), "\n")
	if pyStrip(base) != "" {
		return base + "\n\n" + block + "\n"
	}
	return block + "\n"
}

// RenderAliasRemove is text without the block.
func RenderAliasRemove(text string) string {
	base := strings.TrimRight(StripAliasBlock(text), "\n")
	if pyStrip(base) != "" {
		return base + "\n"
	}
	return ""
}

// HasAliasBlock reports whether text holds a managed block.
func HasAliasBlock(text string) bool { return strings.Contains(text, AliasBegin) }

// HasLegacyAliasBlock reports whether text holds an auth-token-manager block.
func HasLegacyAliasBlock(text string) bool { return strings.Contains(text, legacyAliasBegin) }

// BlockAliasLines are the alias lines inside the managed block.
func BlockAliasLines(text string) []string {
	var out []string
	inside := false
	for _, line := range pySplitLines(text) {
		st := pyStrip(line)
		switch {
		case st == AliasBegin:
			inside = true
		case st == AliasEnd:
			return out
		case inside && strings.HasPrefix(line, "alias "):
			out = append(out, line)
		}
	}
	return out
}

// DefaultProfile is the startup file for the shell named by $SHELL: the rc
// file, or with login the login profile.
func DefaultProfile(home, shell string, login bool, exists func(string) bool) string {
	name := shell[strings.LastIndex(shell, "/")+1:]
	switch {
	case strings.Contains(name, "zsh"):
		if login {
			return filepath.Join(home, ".zprofile")
		}
		return filepath.Join(home, ".zshrc")
	case strings.Contains(name, "bash"):
		if !login {
			return filepath.Join(home, ".bashrc")
		}
		for _, c := range []string{".bash_profile", ".bash_login", ".profile"} {
			if exists != nil && exists(filepath.Join(home, c)) {
				return filepath.Join(home, c)
			}
		}
		return filepath.Join(home, ".bash_profile")
	}
	return filepath.Join(home, ".profile")
}
