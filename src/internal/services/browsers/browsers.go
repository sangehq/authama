// Package browsers keeps the browsers a home remembers: a browser signed in
// with a one-time link may be remembered for a while, so opening the web UI's
// address again signs it in without a new link.
//
// What the browser holds is a cookie with an id and a random secret. The file
// here keeps only the secret's SHA-256, so reading it gives nothing that signs
// a browser in. Every time a remembered browser is signed in by its cookie the
// secret is replaced (rotated), and the one it replaced is kept for a short
// grace, for a second tab that sent the old cookie at the same moment. An old
// secret presented after that means the cookie was copied: the browser is
// forgotten, so neither copy works any more.
package browsers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/lock"
)

// File is where a home keeps its remembered browsers: in the state home,
// owner-only.
func File(stateDir string) string { return filepath.Join(stateDir, "browsers.json") }

// DefaultDays is how long a browser is remembered unless web.remember_days
// says otherwise.
const DefaultDays = 30

// Grace is how long a rotated secret is still taken, without rotating again.
const Grace = time.Minute

// maxBrowsers bounds the file; the oldest is forgotten past it.
const maxBrowsers = 32

// Browser is one remembered browser.
type Browser struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Hash     string    `json:"hash"`
	Prev     string    `json:"prev,omitempty"`
	PrevTill time.Time `json:"prev_till,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
	Expires  time.Time `json:"expires"`
}

type file struct {
	Version  int       `json:"version"`
	Browsers []Browser `json:"browsers"`
}

// ErrReused is returned when a cookie's secret was already rotated away: the
// cookie was copied, and the browser has been forgotten.
var ErrReused = errors.New("a remembered browser's cookie was used after it was replaced; that browser is forgotten")

func secret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hash(s string) string {
	h := sha256.Sum256([]byte("authama-browser\x00" + s))
	return hex.EncodeToString(h[:])
}

func same(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Cookie is the value a browser holds: "<id>.<secret>".
func Cookie(id, sec string) string { return id + "." + sec }

// ParseCookie splits a cookie value into its id and secret.
func ParseCookie(v string) (id, sec string, ok bool) {
	id, sec, ok = strings.Cut(v, ".")
	return id, sec, ok && id != "" && sec != "" && len(v) < 256
}

func load(path string) (file, error) {
	var f file
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return file{Version: 1}, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	return f, nil
}

func save(path string, f file) error {
	f.Version = 1
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := fs.EnsureDir(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(path, string(b)+"\n", 0o600)
}

// update loads path under its lock, lets fn change it, and saves it when fn
// says so. Expired browsers are dropped on the way.
func update(path string, now time.Time, fn func(*file) (bool, error)) error {
	release, err := lock.Acquire(path + ".lock")
	if err != nil {
		return err
	}
	defer release()
	f, err := load(path)
	if err != nil {
		return err
	}
	kept := f.Browsers[:0]
	pruned := false
	for _, b := range f.Browsers {
		if now.Before(b.Expires) {
			kept = append(kept, b)
		} else {
			pruned = true
		}
	}
	f.Browsers = kept
	changed, err := fn(&f)
	if (changed || pruned) && err == nil {
		return save(path, f)
	}
	if changed || pruned {
		_ = save(path, f)
	}
	return err
}

// Remember records a browser for lifetime from now and returns its id and
// the secret for its cookie.
func Remember(path, label string, now time.Time, lifetime time.Duration) (id, sec string, err error) {
	id, sec = secret()[:16], secret()
	err = update(path, now, func(f *file) (bool, error) {
		if len(f.Browsers) >= maxBrowsers {
			sort.Slice(f.Browsers, func(i, j int) bool { return f.Browsers[i].LastUsed.Before(f.Browsers[j].LastUsed) })
			f.Browsers = f.Browsers[1:]
		}
		f.Browsers = append(f.Browsers, Browser{ID: id, Label: label, Hash: hash(sec), Created: now, LastUsed: now, Expires: now.Add(lifetime)})
		return true, nil
	})
	return id, sec, err
}

// SignIn checks a browser's cookie. With its current secret it is signed in,
// and gets a new secret to hold (next); with the one just replaced, within
// Grace, it is signed in and keeps the newer one it was already sent (next is
// ""). Anything else is refused, and an older secret forgets the browser
// (ErrReused). The lifetime starts again from now: a browser in use stays
// remembered.
func SignIn(path, id, sec string, now time.Time, lifetime time.Duration) (next string, ok bool, err error) {
	h := hash(sec)
	err = update(path, now, func(f *file) (bool, error) {
		for i := range f.Browsers {
			b := &f.Browsers[i]
			if !same(b.ID, id) {
				continue
			}
			switch {
			case same(b.Hash, h):
				next = secret()
				b.Prev, b.PrevTill, b.Hash = b.Hash, now.Add(Grace), hash(next)
				b.LastUsed, b.Expires = now, now.Add(lifetime)
				ok = true
				return true, nil
			case b.Prev != "" && same(b.Prev, h) && now.Before(b.PrevTill):
				ok = true
				return false, nil
			case b.Prev != "" && same(b.Prev, h):
				f.Browsers = append(f.Browsers[:i], f.Browsers[i+1:]...)
				return true, ErrReused
			}
			return false, nil
		}
		return false, nil
	})
	return next, ok, err
}

// List is every browser still remembered, the most recently used first.
func List(path string, now time.Time) ([]Browser, error) {
	f, err := load(path)
	if err != nil {
		return nil, err
	}
	out := make([]Browser, 0, len(f.Browsers))
	for _, b := range f.Browsers {
		if now.Before(b.Expires) {
			b.Hash, b.Prev = "", ""
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed.After(out[j].LastUsed) })
	return out, nil
}

// Known reports whether id is still remembered: a session signed in by a
// browser ends when the browser is forgotten.
func Known(path, id string, now time.Time) bool {
	f, err := load(path)
	if err != nil {
		return false
	}
	for _, b := range f.Browsers {
		if b.ID == id && now.Before(b.Expires) {
			return true
		}
	}
	return false
}

// Forget removes the browsers with these ids, or every one when all is set,
// and says how many went.
func Forget(path string, ids []string, all bool, now time.Time) (int, error) {
	n := 0
	err := update(path, now, func(f *file) (bool, error) {
		kept := f.Browsers[:0]
		for _, b := range f.Browsers {
			drop := all
			for _, id := range ids {
				if b.ID == id {
					drop = true
				}
			}
			if drop {
				n++
			} else {
				kept = append(kept, b)
			}
		}
		f.Browsers = kept
		return n > 0, nil
	})
	return n, err
}

// Label names a browser from its User-Agent: "Chrome on macOS". Only the
// browser and the system are kept, not the version or anything else the
// header carries.
func Label(userAgent string) string {
	ua := userAgent
	browser := "A browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	system := ""
	switch {
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		system = "macOS"
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		system = "iOS"
	case strings.Contains(ua, "Linux"):
		system = "Linux"
	}
	if system == "" {
		return browser
	}
	return browser + " on " + system
}
