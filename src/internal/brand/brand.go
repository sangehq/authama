// Package brand is the one place the tool's names are decided.
//
// The product grew from an SSH manager into two features - SSH keys and config,
// and the auth tokens package managers and git read - and took a new name and a
// new command with it. Every user-facing mention of either goes through these
// constants, so the next rename is an edit here and not a search across the tree.
//
// What does NOT change with them, on purpose: the Go module path, the repository
// and its folder, the config home (~/.config/ssh-manager) and the environment
// variables (SSH_MANAGER_*, SSHMGR_DEV_ROOT). Renaming those would strand every
// existing install's data and every script that sets them.
package brand

import "strings"

const (
	// Name is the display name: titles, banners, the web UI, the docs.
	Name = "Authentication Access Manager"

	// Command is the binary and the command people type.
	Command = "authama"

	// LegacyCommand is the name the tool shipped under before. It is still
	// installed beside Command and behaves identically, so no script breaks.
	LegacyCommand = "sshmgr"

	// Summary is the one-line description under the name.
	Summary = "SSH keys & config, and package-manager / git auth tokens, in one tool"
)

// TopLevel lists the commands that sit directly under Command. Every other
// verb lives in the ssh group; a test in the cli package keeps this list equal
// to the command tree.
var TopLevel = []string{"version", "doctor", "init", "migrate", "tui", "history", "serve", "ssh", "token"}

// Line spells a command the way a user types it now: a top-level verb as
// `authama <verb>`, an SSH verb (as the history and the web record it) as
// `authama ssh <verb>`.
func Line(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return Command
	}
	first := strings.Fields(cmd)[0]
	for _, t := range TopLevel {
		if first == t {
			return Command + " " + cmd
		}
	}
	return Command + " ssh " + cmd
}

// Commands lists every name the binary answers to, the current one first.
func Commands() []string { return []string{Command, LegacyCommand} }
