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

// plainDest is an ssh destination that can only be one: an optional user and
// a host name or address, neither starting with '-', with no space, quote,
// shell or control character. ssh_config aliases are not destinations here:
// a target is always its hostname.
var plainDest = regexp.MustCompile(`^([A-Za-z0-9_.][A-Za-z0-9_.+-]*@)?[A-Za-z0-9_.:%\[\]][A-Za-z0-9_.:%\[\]-]*$`)

// destArgs is "--" and dest, the end of every ssh command line: after "--"
// ssh reads no option, so a host can never be taken for one, and dest must be
// a plain destination as well.
func destArgs(dest string) ([]string, error) {
	if !plainDest.MatchString(dest) {
		return nil, fmt.Errorf("%q is not a plain ssh destination (user@host)", dest)
	}
	return []string{"--", dest}, nil
}
