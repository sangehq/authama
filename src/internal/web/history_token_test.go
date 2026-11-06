package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The history's token filter once used ?token=, which the auth check reads as
// a sign-in token: a signed-in browser following "History" from a token's
// menu got a 401. The filter is ?credential= now, and a stray ?token= no
// longer signs a session out.
func TestATokenFilterLinkKeepsTheBrowserSignedIn(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, path := range []string{
		"/history?" + url.Values{tokenFilterParam: {"composer/github.com"}}.Encode(),
		"/history?token=composer/github.com", // a link from before the rename
		"/keys?token=not-the-sign-in-token",
	} {
		if rec := c.get(path); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	body := c.get("/history?" + tokenFilterParam + "=composer/github.com").Body.String()
	if !strings.Contains(body, `name="credential" value="composer/github.com"`) {
		t.Error("the filter form does not carry the token filter back")
	}
	// Without a session, a wrong token is still refused.
	rec := do(s, httptestGet("/?token=wrong"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a wrong token with no session: %d", rec.Code)
	}
}

func httptestGet(path string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "http://"+testHost+path, nil)
	return r
}
