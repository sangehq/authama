package setupxfer

import (
	"encoding/json"
	"strings"
	"testing"
)

// A document's custom hypervisor commands and token-redirecting providers
// are risky: authama runs the first and sends tokens to the second (S-01, S-02).
func TestRiskyNamesHypervisorCommandsAndProviderRedirects(t *testing.T) {
	d := Doc{
		Settings:  json.RawMessage(`{"hypervisors":{"x":{"list":["/bin/sh","-c","planted"],"address":["true"]}}}`),
		Providers: json.RawMessage(`{"providers":{"github":{"kind":"rest","token_env":"GH_TOKEN","rest":{"base_url":"https://evil.example"}}}}`),
	}
	var got []string
	for _, r := range d.Risky() {
		got = append(got, r.String())
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{"hypervisors.x: list /bin/sh -c planted", "hypervisors.x: address true", "github https://evil.example"} {
		if !strings.Contains(all, want) {
			t.Errorf("Risky misses %q:\n%s", want, all)
		}
	}
	if n := len(Doc{Settings: json.RawMessage(`{"display":{"timezone":"UTC"}}`)}.Risky()); n != 0 {
		t.Errorf("plain settings are risky: %d", n)
	}
	if err := RiskyError(d.Risky(), "authama"); err == nil || !strings.Contains(err.Error(), "setup import FILE --allow-risky-options") {
		t.Errorf("refusal: %v", err)
	}
}

// A full export's .env is imported only for the names its setup reads tokens
// from, and never for a name that changes how programs run (S-03).
func TestOnlyTokenNamesKeepsTheSetupsTokens(t *testing.T) {
	d := Doc{Tokens: json.RawMessage(`{"composer":{"github.com":{"type":"github-oauth","env":"GH_TOKEN_TEST"},"x.example":{"type":"bearer","env":"GIT_SSH_COMMAND"}}}`)}
	env := "GH_TOKEN_TEST=one\nGIT_SSH_COMMAND=planted\nLD_PRELOAD=/x.so\nOTHER=two\nAUTHAMA_WEBHOOKS_ALLOW_PRIVATE=1\n"
	kept, skipped, err := onlyTokenNames(env, d)
	if err != nil {
		t.Fatal(err)
	}
	if kept != "GH_TOKEN_TEST="+Quote("one")+"\n" {
		t.Errorf("kept %q", kept)
	}
	if strings.Join(skipped, ",") != "AUTHAMA_WEBHOOKS_ALLOW_PRIVATE,GIT_SSH_COMMAND,LD_PRELOAD,OTHER" {
		t.Errorf("skipped %v", skipped)
	}
}

// Settings that send backups to another key, hide what happened, re-trust a
// changed host key, or open authama to other programs are risky in a document
// that does not prove who made it (audit F-13, 2026-11-25). Values matching the
// defaults are not.
func TestRiskyNamesSettingsThatWeakenOrRedirect(t *testing.T) {
	d := Doc{Settings: json.RawMessage(`{
	  "backup":{"age_recipient":"age1planted","snapshot_retain":5},
	  "history":{"enabled":false},
	  "vm":{"repin_changed":true},
	  "api":{"enabled":true},
	  "mcp":{"enabled":"true"},
	  "webhooks":{"enabled":true,"allow_private":true,"include_details":true},
	  "ssh":{"auto_pin":true}}`)}
	var got []string
	for _, r := range d.Risky() {
		got = append(got, r.Where+" "+r.Key+" "+r.Value)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		"settings backup age_recipient age1planted", "settings history enabled false", "settings vm repin_changed true",
		"settings api enabled true", "settings mcp enabled true", "settings webhooks enabled true",
		"settings webhooks allow_private true", "settings webhooks include_details true",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("Risky misses %q:\n%s", want, all)
		}
	}
	for _, not := range []string{"snapshot_retain", "auto_pin"} {
		if strings.Contains(all, not) {
			t.Errorf("Risky flags %s, which weakens nothing:\n%s", not, all)
		}
	}
	safe := Doc{Settings: json.RawMessage(`{"history":{"enabled":true},"api":{"enabled":false},"backup":{"age_recipient":""}}`)}
	if r := safe.Risky(); len(r) != 0 {
		t.Errorf("default values are risky: %v", r)
	}
}
