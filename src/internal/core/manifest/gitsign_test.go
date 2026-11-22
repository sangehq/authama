package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileGitIsValidatedWhereItWouldBreakGitOrAllowedSigners(t *testing.T) {
	cases := []struct {
		name string
		git  ProfileGit
		want string // "" = valid
	}{
		{"fine", ProfileGit{Email: "ada@acme.example", Name: "Ada Lovelace", Sign: true, Dirs: []string{"~/code/acme", "/srv/x/"}}, ""},
		{"no email", ProfileGit{}, "email is empty"},
		{"two addresses", ProfileGit{Email: "a@x.example,b@x.example"}, "not one email"},
		{"space", ProfileGit{Email: "a b@x.example"}, "not one email"},
		{"quote in name", ProfileGit{Email: "a@x.example", Name: `Ada "AL"`}, "quote"},
		{"newline in name", ProfileGit{Email: "a@x.example", Name: "Ada\n[core]"}, "control"},
		{"relative dir", ProfileGit{Email: "a@x.example", Dirs: []string{"code/acme"}}, "absolute"},
		{"bracket dir", ProfileGit{Email: "a@x.example", Dirs: []string{"/x]y"}}, "]"},
		{"bad key", ProfileGit{Email: "a@x.example", SigningKey: "../k"}, "safe name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Starter(false)
			g := tc.git
			m.SetProfile("work", Profile{KeyScope: "per_service", Git: &g})
			err := m.Validate()
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
}

func TestTwoProfilesCannotClaimOneDirectory(t *testing.T) {
	m := Starter(false)
	m.SetProfile("a", Profile{KeyScope: "per_service", Git: &ProfileGit{Email: "a@x.example", Dirs: []string{"~/code"}}})
	m.SetProfile("b", Profile{KeyScope: "per_service", Git: &ProfileGit{Email: "b@x.example", Dirs: []string{"~/code/"}}})
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "already profile") {
		t.Fatalf("got %v", err)
	}
}

func TestAProfileWithoutGitSavesAsBefore(t *testing.T) {
	b, err := json.Marshal(Profile{KeyScope: "per_service"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"git"`) {
		t.Errorf("an unset git block was written: %s", b)
	}
	var back Profile
	if err := json.Unmarshal([]byte(`{"key_scope":"per_service","git":{"email":"a@x.example","sign":true,"dirs":["~/c"]},"hosts":[]}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.Git == nil || !back.Git.Sign || back.Git.Dirs[0] != "~/c" {
		t.Errorf("read back %+v", back.Git)
	}
}
