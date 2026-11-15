package web

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/tokens"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

// The editor shows a tokens.json that names variables, and previews an edit
// as a diff; a file that holds a value is not shown, and an edit that adds
// one is refused before anything is written.
func TestTheTokenConfigEditorShowsNamesAndRefusesValues(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if err := os.WriteFile(p.Tokens(), []byte(tokens.AllTargetsTemplate()), 0o644); err != nil {
		t.Fatal(err)
	}
	if body := c.get("/tokens").Body.String(); !strings.Contains(body, `id="edit-token-config"`) {
		t.Fatal("a names-only tokens.json is not offered for editing")
	}

	edited := strings.Replace(tokens.AllTargetsTemplate(), `"GH_TOKEN"`, `"GH_TOKEN_RENAMED"`, 1)
	rec := c.change("/changes/preview", url.Values{"action": {"token-config-edit"}, "body": {edited}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "GH_TOKEN_RENAMED") {
		t.Fatalf("preview of a rename: %d\n%.400s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(p.Tokens()); strings.Contains(string(b), "GH_TOKEN_RENAMED") {
		t.Fatal("the preview wrote the file")
	}

	// Joined with a hyphen, so it is never a variable name however the
	// generator draws it.
	secret := fmt.Sprintf("%s-%s", testcred.Secret("tokensjson"), testcred.Secret("tokensjson"))
	withValue := strings.Replace(tokens.AllTargetsTemplate(), `"GH_TOKEN"`, `"`+secret+`"`, 1)
	rec = c.change("/changes/preview", url.Values{"action": {"token-config-edit"}, "body": {withValue}})
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `name="hash"`) {
		t.Fatal("an edit holding a value was offered for confirmation")
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Error("the refusal echoed the value back into the page")
	}

	// A file that already holds a value is not put into a page at all.
	if err := os.WriteFile(p.Tokens(), []byte(withValue), 0o644); err != nil {
		t.Fatal(err)
	}
	body := c.get("/tokens").Body.String()
	if strings.Contains(body, `id="edit-token-config"`) || strings.Contains(body, secret) {
		t.Fatal("a tokens.json holding a value reached the page")
	}
	if !strings.Contains(body, "Not editable here") {
		t.Error("the page does not say why the file is not editable")
	}
}

// A public key line is shown only when it is one: a known type whose blob
// names that type. A private key, or anything else, in a .pub is not.
func TestOnlyAPublicKeyLineIsShown(t *testing.T) {
	blob := func(typ string) string {
		b := binary.BigEndian.AppendUint32(nil, uint32(len(typ)))
		b = append(b, typ...)
		b = append(b, make([]byte, 32)...)
		return base64.StdEncoding.EncodeToString(b)
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	good := "ssh-ed25519 " + blob("ssh-ed25519") + " me@laptop"
	if got := publicKeyLine(write("good.pub", good+"\n")); got != good {
		t.Errorf("a valid key: got %q", got)
	}
	for name, text := range map[string]string{
		"private.pub":  "-----BEGIN OPENSSH PRIVATE KEY-----\n" + blob("ssh-ed25519") + "\n-----END OPENSSH PRIVATE KEY-----\n",
		"mislabel.pub": "ssh-rsa " + blob("ssh-ed25519") + " wrong type\n",
		"unknown.pub":  "ssh-made-up " + blob("ssh-made-up") + "\n",
		"notb64.pub":   "ssh-ed25519 not*base64\n",
		"two.pub":      good + "\n" + good + "\n",
	} {
		if got := publicKeyLine(write(name, text)); got != "" {
			t.Errorf("%s was shown: %q", name, got)
		}
	}
	if publicKeyLine(filepath.Join(dir, "absent.pub")) != "" || publicKeyLine("") != "" {
		t.Error("a missing file produced a line")
	}
}
