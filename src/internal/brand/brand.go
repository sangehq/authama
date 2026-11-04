// Package brand is the one place the tool's names are decided.
//
// The product grew from an SSH manager into two features - SSH keys and config,
// and the auth tokens package managers and git read - and took a new name and a
// new command with it. Every user-facing mention of either goes through these
// constants, so the next rename is an edit here and not a search across the tree.
//
// The repository (simtabi/authama) and the Go module path
// (github.com/simtabi/authama/src) followed the name at 0.1.0; the config home
// (~/.config/authama) and the environment variables (AUTHAMA_*) at 0.1.1; the
// release archives, the Linux packages, the Homebrew cask and the Scoop
// manifest at 0.1.2. What is kept for migration only: the sshmgr command
// (LegacyCommand, deprecated: it prints DeprecationNotice), the old variable
// names (SSH_MANAGER_*, SSHMGR_*; honored with a deprecation notice, see
// envnames), the old home (copied over once, see migratesvc), and the old
// managed-block markers, known_hosts tag, bundle names and scheduler label,
// which are still recognized.
package brand

import "strings"

const (
	// Name is the display name: titles, banners, the web UI, the docs.
	Name = "Authentication Access Manager"

	// Command is the binary and the command people type.
	Command = "authama"

	// LegacyCommand is the name the tool shipped under before. It is still
	// installed beside Command and behaves identically, so no script breaks,
	// but it is deprecated: it prints DeprecationNotice on stderr.
	LegacyCommand = "sshmgr"

	// DeprecationNotice is the one line the LegacyCommand binary prints on
	// stderr before it runs.
	DeprecationNotice = LegacyCommand + " is deprecated and will be removed in a future release; run " +
		Command + " instead (same commands)"

	// Summary is the one-line description under the name.
	Summary = "SSH keys & config, and package-manager / git auth tokens, in one tool"
)

// TopLevel lists the commands that sit directly under Command. Every other
// verb lives in the ssh group; a test in the cli package keeps this list equal
// to the command tree.
var TopLevel = []string{"version", "doctor", "init", "migrate", "settings", "tui", "history", "serve", "ssh", "token"}

// TopLevelAliases are the other names of top-level commands: `auth` is the
// token group. The same test keeps it equal to the tree's aliases.
var TopLevelAliases = []string{"auth"}

// IsTopLevel reports whether word names a command directly under Command,
// by its name or an alias.
func IsTopLevel(word string) bool {
	for _, t := range TopLevel {
		if word == t {
			return true
		}
	}
	for _, t := range TopLevelAliases {
		if word == t {
			return true
		}
	}
	return false
}

// Line spells a command the way a user types it now: a top-level verb as
// `authama <verb>`, an SSH verb (as the history and the web record it) as
// `authama ssh <verb>`.
func Line(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return Command
	}
	if IsTopLevel(strings.Fields(cmd)[0]) {
		return Command + " " + cmd
	}
	return Command + " ssh " + cmd
}

// Commands lists every name the binary answers to, the current one first.
func Commands() []string { return []string{Command, LegacyCommand} }
