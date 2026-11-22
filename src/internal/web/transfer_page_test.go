package web

import (
	"archive/zip"
	"bytes"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

// upload posts a file to /transfer/import as the page's form does.
func (c *client) upload(name string, data []byte, fields map[string]string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	sess := c.s.sessions.get(c.cookie.Value, c.s.now())
	_ = mw.WriteField(csrfField, sess.csrf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/transfer/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "http://"+testHost)
	r.AddCookie(c.cookie)
	return do(c.s, r)
}

// The header's Export / Import button opens /transfer, whose export picker
// previews the chosen files and downloads them as one zip with no secret.
func TestTheTransferPageExportsAPackWithNoSecret(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	home := c.get("/").Body.String()
	if !strings.Contains(home, `id="navbar-transfer"`) || !strings.Contains(home, `/transfer#import`) {
		t.Fatal("the header has no Export / Import button")
	}
	page := c.get("/transfer?preview=1&part=termius&part=setup&part=git-signing&termius_groups=profile")
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "termius-hosts.csv") || !strings.Contains(body, "setup.json") ||
		!strings.Contains(body, "allowed_signers") || !strings.Contains(body, `id="export-preview"`) {
		t.Fatalf("/transfer preview: %d\n%.2000s", page.Code, body)
	}
	if strings.Contains(body, privateMarker) {
		t.Fatal("the preview shows a private key")
	}
	zr := c.get("/transfer/pack.zip?part=all")
	if zr.Code != http.StatusOK || zr.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("pack.zip: %d %s\n%.400s", zr.Code, zr.Header().Get("Content-Type"), zr.Body.String())
	}
	data := zr.Body.Bytes()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		if bytes.Contains(b, []byte(privateMarker)) {
			t.Errorf("%s in the pack holds a private key", f.Name)
		}
	}
	for _, want := range []string{"README.txt", "termius-hosts.csv", "ssh_config", "setup.json", "structure.json", "git-signing.json", "tokens.csv", "hosts.csv", "keys.csv", "snapshots.csv"} {
		if !names[want] {
			t.Errorf("the pack has no %s: %v", want, names)
		}
	}
	if bad := c.get("/transfer/pack.zip?part=nope"); bad.Code != http.StatusBadRequest {
		t.Errorf("an unknown part: %d", bad.Code)
	}
}

// An uploaded pack is told apart, its setup.json summarised, and handed to
// the change preview as a setup import; an ssh config is pointed elsewhere.
func TestTheTransferPageImportsAPackThroughThePreview(t *testing.T) {
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	pack := c.get("/transfer/pack.zip?part=setup&part=termius").Body.Bytes()
	rev := c.upload("authama-export.zip", pack, map[string]string{"on_conflict": "keep"})
	body := html.UnescapeString(rev.Body.String())
	if rev.Code != http.StatusOK || !strings.Contains(body, "Export pack") || !strings.Contains(body, `value="setup-import"`) {
		t.Fatalf("upload: %d\n%.3000s", rev.Code, body)
	}
	m := regexp.MustCompile(`name="body" value="([^"]*)"`).FindStringSubmatch(rev.Body.String())
	if m == nil {
		t.Fatal("the review carries no document")
	}
	prev := c.change("/changes/preview", url.Values{"action": {"setup-import"}, "body": {html.UnescapeString(m[1])}, "on_conflict": {"keep"}})
	if prev.Code != http.StatusOK || hashField.FindStringSubmatch(prev.Body.String()) == nil {
		t.Fatalf("preview: %d\n%.1500s", prev.Code, prev.Body.String())
	}
	cfg := c.upload("config", []byte("Host x\n  HostName x.example\n"), nil)
	if !strings.Contains(cfg.Body.String(), "SSH import") {
		t.Errorf("an ssh config is not pointed to SSH import:\n%.1500s", cfg.Body.String())
	}
}

// The encrypted export downloads an age file under the passphrase, without
// showing anything of it; uploaded again with its passphrase it is listed by
// name only, and imports through the preview with the PIN.
func TestTheTransferPageImportsAnEncryptedExport(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	pass := testcred.Secret("transfer-pass")
	if len(pass) < 12 {
		pass += "-long-enough-pass"
	}
	short := c.change("/transfer/encrypted", url.Values{"passphrase": {"short"}, "passphrase_confirm": {"short"}})
	if short.Code != http.StatusBadRequest {
		t.Fatalf("a short passphrase: %d", short.Code)
	}
	enc := c.change("/transfer/encrypted", url.Values{"passphrase": {pass}, "passphrase_confirm": {pass}})
	if enc.Code != http.StatusOK || !bytes.HasPrefix(enc.Body.Bytes(), []byte("age-encryption.org/v1\n-> scrypt ")) {
		t.Fatalf("encrypted export: %d %.300q", enc.Code, enc.Body.String())
	}
	if bytes.Contains(enc.Body.Bytes(), []byte(privateMarker)) || bytes.Contains(enc.Body.Bytes(), []byte(pass)) {
		t.Fatal("the encrypted export holds plaintext")
	}
	entries, _ := os.ReadDir(p.StateDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "export-") || strings.HasPrefix(e.Name(), "import-") {
			t.Errorf("a temporary folder was left in the state home: %s", e.Name())
		}
	}
	data := enc.Body.Bytes()
	if wrong := c.upload("setup.age", data, map[string]string{"passphrase": pass + "x", "on_conflict": "keep"}); wrong.Code != http.StatusBadRequest {
		t.Errorf("a wrong passphrase: %d", wrong.Code)
	}
	rev := c.upload("setup.age", data, map[string]string{"passphrase": pass, "on_conflict": "keep"})
	body := rev.Body.String()
	if rev.Code != http.StatusOK || !strings.Contains(body, "Encrypted full export") || !strings.Contains(body, `value="transfer-import-full"`) {
		t.Fatalf("upload: %d\n%.3000s", rev.Code, body)
	}
	if strings.Contains(body, privateMarker) || strings.Contains(body, pass) {
		t.Fatal("the review shows a private key or the passphrase")
	}
	ref := func(name string) string {
		m := regexp.MustCompile(`name="` + name + `" value="([^"]+)"`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("no %s in the review", name)
		}
		return m[1]
	}
	f := url.Values{"action": {"transfer-import-full"}, "file_ref": {ref("file_ref")}, "pass_ref": {ref("pass_ref")}, "on_conflict": {"keep"}}
	prev := c.change("/changes/preview", f)
	pb := prev.Body.String()
	hash := hashField.FindStringSubmatch(pb)
	if prev.Code != http.StatusOK || hash == nil || !strings.Contains(pb, "key file") {
		t.Fatalf("preview: %d\n%.2500s", prev.Code, pb)
	}
	if strings.Contains(pb, privateMarker) || strings.Contains(pb, pass) {
		t.Fatal("the preview shows a private key or the passphrase")
	}
	f.Set("hash", hash[1])
	if done := c.change("/changes/apply", f); done.Code != http.StatusOK || !strings.Contains(done.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.1500s", done.Code, done.Body.String())
	}
	if again := c.change("/changes/preview", f); again.Code == http.StatusOK && hashField.FindStringSubmatch(again.Body.String()) != nil {
		t.Error("the uploaded export was used twice")
	}
}

// A read-only server exports, but offers neither the encrypted export nor
// an import.
func TestAReadOnlyTransferPageOnlyExports(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	body := c.get("/transfer").Body.String()
	if !strings.Contains(body, `id="export"`) || strings.Contains(body, `action="/transfer/import"`) || strings.Contains(body, `action="/transfer/encrypted"`) {
		t.Fatalf("read-only /transfer:\n%.2000s", body)
	}
	if r := c.upload("x.json", []byte("{}"), nil); r.Code == http.StatusOK {
		t.Errorf("a read-only server took an upload: %d", r.Code)
	}
}

// A Termius group scheme not on the list is refused, never echoed into a zip.
func TestTheTransferPackRefusesAnUnknownGroupScheme(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	rec := c.get("/transfer/pack.zip?part=termius&termius_groups=%3Cscript%3Ex")
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "<script>x") {
		t.Fatalf("pack: %d\n%.400s", rec.Code, rec.Body.String())
	}
	if rec := c.get("/transfer/pack.zip?part=termius&termius_groups=category/profile"); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("a known scheme: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}
