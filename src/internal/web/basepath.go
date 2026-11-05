package web

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// basePathSegment is one segment of a base path: plain URL characters only,
// so the path can be written into HTML and a Location header unescaped.
var basePathSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]*$`)

// CleanBasePath normalises the path the web UI is served under (web.base_path):
// "" or "/" mean the root and give "", anything else gives "/a" or "/a/b" with
// no trailing slash. A segment that is empty, "." or "..", or holds anything
// but letters, digits and . _ ~ -, is refused.
func CleanBasePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("web.base_path %q: start it with a slash, such as /authama", p)
	}
	trimmed := strings.TrimSuffix(p, "/")
	for _, seg := range strings.Split(strings.TrimPrefix(trimmed, "/"), "/") {
		if seg == "." || seg == ".." || !basePathSegment.MatchString(seg) {
			return "", fmt.Errorf("web.base_path %q: use letters, digits and . _ ~ - between slashes, such as /authama", p)
		}
	}
	return trimmed, nil
}

// withBasePath serves next under base: a request for base or anything below
// it reaches next with base taken off, the bare root is sent to base/, and
// anything else is not found. A Location the handlers set (a redirect to
// /hosts, say) gets base put back, so no handler needs to know it.
func withBasePath(base string, next http.Handler) http.Handler {
	if base == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == base:
			http.Redirect(w, r, base+"/"+rawQuery(r), http.StatusFound)
			return
		case r.URL.Path == "/":
			http.Redirect(w, r, base+"/"+rawQuery(r), http.StatusFound)
			return
		case !strings.HasPrefix(r.URL.Path, base+"/"):
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, base)
		if r.URL.RawPath != "" {
			r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, base)
		}
		next.ServeHTTP(&basePathWriter{ResponseWriter: w, base: base}, r2)
	})
}

func rawQuery(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

// basePathWriter puts the base path in front of a Location that names a path
// on this server ("/hosts"), leaving an absolute URL or a scheme-relative one
// ("//host/x") alone.
type basePathWriter struct {
	http.ResponseWriter
	base        string
	wroteHeader bool
}

func (b *basePathWriter) WriteHeader(code int) {
	if !b.wroteHeader {
		b.wroteHeader = true
		h := b.Header()
		if loc := h.Get("Location"); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "//") {
			h.Set("Location", b.base+loc)
		}
	}
	b.ResponseWriter.WriteHeader(code)
}

func (b *basePathWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
	return b.ResponseWriter.Write(p)
}

// Flush keeps a job's event stream streaming through the wrapper.
func (b *basePathWriter) Flush() {
	if f, ok := b.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (b *basePathWriter) Unwrap() http.ResponseWriter { return b.ResponseWriter }

// prefixed puts the base path in front of a path on this server, for links
// built in Go and written into a page: "/hosts" under /authama is
// "/authama/hosts". Anything else (a #fragment, an absolute URL, "//host")
// is returned as it is.
func prefixed(base, p string) string {
	if base == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return p
	}
	return base + p
}
