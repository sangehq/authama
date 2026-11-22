package providers

import (
	"fmt"
	"regexp"
)

// program is the program a provider runs for name: one of a fixed few, so
// nothing from a manifest, a document or the network can choose what runs.
// The returned string is a constant, never name itself.
func program(name string) (string, error) {
	switch name {
	case "ssh":
		return "ssh", nil
	case "ssh-copy-id":
		return "ssh-copy-id", nil
	case "gh":
		return "gh", nil
	case "glab":
		return "glab", nil
	}
	return "", fmt.Errorf("providers: %q is not a program authama runs", name)
}

// plainDest is an ssh destination that can only be one, by the rules the
// manifest validator and OpenSSH already apply: an optional user (which may
// hold '@', '\' or '$', as in first.last@corp.example or DOMAIN\user) and a
// host name, IPv4 or IPv6 address (bare or bracketed). Neither starts with
// '-' or holds whitespace, a control character, a quote or a character ssh
// refuses (` ; & | < > ( ) { }, and $ or \ in a host). ssh splits at the last
// '@', and so does this.
var plainDest = regexp.MustCompile("^(?:[^-\\s\\x00-\\x1f\\x7f`;&|<>(){}\"'][^\\s\\x00-\\x1f\\x7f`;&|<>(){}\"']*@)?" +
	"[^-@\\s\\x00-\\x1f\\x7f`;&|<>(){}\"'$\\\\][^@\\s\\x00-\\x1f\\x7f`;&|<>(){}\"'$\\\\]*$")

// destArgs is "--" and dest, the end of every ssh command line: after "--"
// ssh reads no option, so a host can never be taken for one, and dest must be
// a plain destination as well.
func destArgs(dest string) ([]string, error) {
	if !plainDest.MatchString(dest) {
		return nil, fmt.Errorf("%q cannot be an ssh destination: the user and hostname must not start with '-' "+
			"or hold whitespace, quotes or any of `;&|<>(){} (and no $ or \\ in the hostname); fix the host with `authama ssh host edit`", dest)
	}
	return []string{"--", dest}, nil
}
