package main

import "testing"

func TestAnnounce(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"doctor"}, true},
		{[]string{"ssh", "deploy", "work"}, true},
		{[]string{"vm", "proxy", "devbox"}, false},
		{[]string{"ssh", "vm", "proxy", "devbox"}, false},
		{[]string{"--dev-root", "/tmp/x", "vm", "proxy", "devbox"}, false},
		{[]string{"completion", "zsh"}, false},
		{[]string{"__complete", "ssh", ""}, false},
		{[]string{"proxy"}, true},
	} {
		if got := announce(tc.args); got != tc.want {
			t.Errorf("announce(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
