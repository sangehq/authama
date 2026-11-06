package web

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// secretTTL is how long a secret typed into the token form waits for its
// confirmation.
const secretTTL = 15 * time.Minute

// secretStash holds the one secret a page ever receives - a token typed into
// the token form - between its preview and its confirmation, so it is never
// written into a page: the preview's form carries a random reference instead.
// A reference answers only to the session that made it, and only until it is
// used or expires; nothing here is written anywhere.
type secretStash struct {
	mu sync.Mutex
	m  map[string]stashed
}

type stashed struct {
	value   string
	sess    *session
	expires time.Time
}

// put stores value for sess and returns its reference.
func (s *secretStash) put(sess *session, value string, now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]stashed{}
	}
	for ref, e := range s.m {
		if now.After(e.expires) {
			delete(s.m, ref)
		}
	}
	ref := NewToken()
	s.m[ref] = stashed{value: value, sess: sess, expires: now.Add(secretTTL)}
	return ref
}

// get returns the secret ref stands for, if it is this session's and current.
func (s *secretStash) get(sess *session, ref string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[ref]
	if !ok || sess == nil || e.sess != sess || now.After(e.expires) {
		return "", false
	}
	return e.value, true
}

// drop forgets ref, once the change that used it is made.
func (s *secretStash) drop(ref string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, ref)
}

// errSecretGone is a confirmation whose secret has expired or was used.
var errSecretGone = errors.New("the secret typed for this change is no longer held (it expires after 15 minutes, " +
	"and is used once); fill in the token form again")

// secretField checks a typed secret: present, no control character, a sane size.
func secretField(v string) (string, error) {
	if v == "" {
		return "", badForm("the secret is empty")
	}
	if strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", badForm("the secret holds a control character")
	}
	if len(v) > 4096 {
		return "", badForm("the secret is too long")
	}
	return v, nil
}
