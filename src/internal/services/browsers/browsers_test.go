package browsers

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestABrowserSignsInWithItsCookieAndGetsANewOne(t *testing.T) {
	path := File(t.TempDir())
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	life := 30 * 24 * time.Hour
	id, sec, err := Remember(path, "Chrome on macOS", now, life)
	if err != nil {
		t.Fatal(err)
	}
	next, ok, err := SignIn(path, id, sec, now.Add(time.Hour), life)
	if err != nil || !ok || next == "" || next == sec {
		t.Fatalf("first sign-in: next=%q ok=%v err=%v", next, ok, err)
	}
	// The same cookie again within the grace (two tabs racing) still signs in.
	if again, ok, err := SignIn(path, id, sec, now.Add(time.Hour+10*time.Second), life); err != nil || !ok || again != "" {
		t.Fatalf("within grace: %q %v %v", again, ok, err)
	}
	// The new one works.
	if _, ok, err := SignIn(path, id, next, now.Add(2*time.Hour), life); err != nil || !ok {
		t.Fatalf("rotated cookie: %v %v", ok, err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), sec) || strings.Contains(string(b), next) {
		t.Fatal("browsers.json holds a cookie secret; only hashes belong there")
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("browsers.json is %v, want 0600", fi.Mode().Perm())
	}
}

func TestAReplayedOldCookieForgetsTheBrowser(t *testing.T) {
	path := File(t.TempDir())
	now := time.Now()
	id, sec, _ := Remember(path, "x", now, time.Hour)
	if _, ok, _ := SignIn(path, id, sec, now, time.Hour); !ok {
		t.Fatal("no sign-in")
	}
	_, ok, err := SignIn(path, id, sec, now.Add(2*Grace), time.Hour)
	if ok || !errors.Is(err, ErrReused) {
		t.Fatalf("a stolen, replayed cookie: ok=%v err=%v", ok, err)
	}
	if Known(path, id, now.Add(2*Grace)) {
		t.Fatal("the browser is still remembered after its old cookie was replayed")
	}
}

func TestAWrongOrUnknownCookieIsRefused(t *testing.T) {
	path := File(t.TempDir())
	now := time.Now()
	id, _, _ := Remember(path, "x", now, time.Hour)
	for _, c := range [][2]string{{id, "guess"}, {"nobody", "guess"}, {"", ""}} {
		if _, ok, _ := SignIn(path, c[0], c[1], now, time.Hour); ok {
			t.Fatalf("%q signed in", c)
		}
	}
	if !Known(path, id, now) {
		t.Fatal("a wrong guess forgot the browser")
	}
}

func TestABrowserExpiresAndCanBeForgotten(t *testing.T) {
	path := File(t.TempDir())
	now := time.Now()
	a, sa, _ := Remember(path, "a", now, time.Hour)
	b, _, _ := Remember(path, "b", now, 48*time.Hour)
	c, _, _ := Remember(path, "c", now, 48*time.Hour)
	if _, ok, _ := SignIn(path, a, sa, now.Add(2*time.Hour), time.Hour); ok {
		t.Fatal("an expired browser signed in")
	}
	list, _ := List(path, now.Add(2*time.Hour))
	if len(list) != 2 {
		t.Fatalf("list after expiry: %+v", list)
	}
	for _, x := range list {
		if x.Hash != "" || x.Prev != "" {
			t.Fatal("List hands out hashes")
		}
	}
	if n, _ := Forget(path, []string{b}, false, now); n != 1 || Known(path, b, now) || !Known(path, c, now) {
		t.Fatalf("forget one: n=%d", n)
	}
	if n, _ := Forget(path, nil, true, now); n != 1 || Known(path, c, now) {
		t.Fatalf("forget all: n=%d", n)
	}
}

func TestTheNumberOfBrowsersIsCapped(t *testing.T) {
	path := File(t.TempDir())
	now := time.Now()
	first, _, _ := Remember(path, "first", now, time.Hour)
	for i := 1; i <= maxBrowsers; i++ {
		_, _, _ = Remember(path, "x", now.Add(time.Duration(i)*time.Second), time.Hour)
	}
	list, _ := List(path, now)
	if len(list) != maxBrowsers || Known(path, first, now) {
		t.Fatalf("%d remembered, first still known=%v", len(list), Known(path, first, now))
	}
}

func TestCookiesAndLabels(t *testing.T) {
	if id, sec, ok := ParseCookie(Cookie("abc", "def")); !ok || id != "abc" || sec != "def" {
		t.Fatal("cookie round trip")
	}
	for _, bad := range []string{"", "nodot", ".x", "x."} {
		if _, _, ok := ParseCookie(bad); ok {
			t.Fatalf("%q parsed", bad)
		}
	}
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
	if got := Label(ua); got != "Chrome on macOS" {
		t.Fatalf("label %q", got)
	}
	_ = filepath.Join
}

// A cookie copied several rotations ago still trips the alarm: every secret
// a browser has been given is remembered by hash (B-03).
func TestACookieSeveralRotationsOldForgetsTheBrowser(t *testing.T) {
	path := File(t.TempDir())
	now := time.Now()
	id, first, _ := Remember(path, "x", now, time.Hour)
	sec := first
	for i := 0; i < 3; i++ {
		next, ok, err := SignIn(path, id, sec, now.Add(time.Duration(i)*time.Minute), time.Hour)
		if !ok || err != nil {
			t.Fatalf("rotation %d: %v %v", i, ok, err)
		}
		sec = next
	}
	_, ok, err := SignIn(path, id, first, now.Add(10*time.Minute), time.Hour)
	if ok || !errors.Is(err, ErrReused) {
		t.Fatalf("a cookie three rotations old: ok=%v err=%v", ok, err)
	}
	if Known(path, id, now.Add(10*time.Minute)) {
		t.Fatal("the browser is still remembered")
	}
}
