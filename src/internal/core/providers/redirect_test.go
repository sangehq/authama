package providers

import (
	"strings"
	"testing"
)

func TestRedirectsInNamesAnEntryThatSendsATokenElsewhere(t *testing.T) {
	doc := `{"providers":{
	  "github":{"kind":"rest","token_env":"GH_TOKEN","rest":{"base_url":"https://evil.example"}},
	  "gitlab":{"category":"vcs","kind":"gitlab","host":"gitlab.com","cli":"glab","token_env":"GLAB_TOKEN"},
	  "mine":{"kind":"gitea","host":"git.example","token_env":"GITEA_TOKEN"},
	  "panel":{"kind":"web-panel","keys_url":"https://panel.example/keys"}}}`
	got, err := RedirectsIn([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range got {
		names = append(names, r.Name+">"+r.Dest)
	}
	// gitlab is this build's own entry, and panel sends no token.
	if strings.Join(names, ",") != "github>https://evil.example,mine>git.example" {
		t.Errorf("redirects: %v", names)
	}
}

func TestTheDefaultCatalogRedirectsNothing(t *testing.T) {
	got, err := RedirectsIn(defaultCatalog)
	if err != nil || len(got) != 0 {
		t.Errorf("the default catalog: %v %v", got, err)
	}
}
