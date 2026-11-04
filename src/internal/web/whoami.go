package web

import (
	"os"
	"os/user"
	"strings"
)

// whoami is the account the server runs as, and the machine: what the header
// shows in place of a generic name, since every change is made as this user.
type whoami struct {
	user, host string
}

// currentUser reads the OS account and host name once, at start-up.
func currentUser() whoami {
	w := whoami{}
	if u, err := user.Current(); err == nil {
		w.user = u.Username
		// Windows answers DOMAIN\name.
		if i := strings.LastIndexAny(w.user, `\`); i >= 0 {
			w.user = w.user[i+1:]
		}
	}
	if w.user == "" {
		w.user = os.Getenv("USER")
	}
	if h, err := os.Hostname(); err == nil {
		w.host = strings.TrimSuffix(h, ".local")
	}
	return w
}

// initials are what the avatar shows: the first two letters of the user name.
func (w whoami) initials() string {
	r := []rune(strings.ToUpper(w.user))
	if len(r) > 2 {
		r = r[:2]
	}
	return string(r)
}
