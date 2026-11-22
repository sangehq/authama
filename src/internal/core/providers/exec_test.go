package providers

import (
	"slices"
	"testing"
)

func TestProgramRunsOnlyTheFixedFew(t *testing.T) {
	for _, ok := range []string{"ssh", "ssh-copy-id", "gh", "glab"} {
		if got, err := program(ok); err != nil || got != ok {
			t.Errorf("program(%q) = %q, %v", ok, got, err)
		}
	}
	for _, bad := range []string{"", "sh", "bash", "/bin/sh", "ssh ", "./ssh", "-oProxyCommand=x", "curl"} {
		if _, err := program(bad); err == nil {
			t.Errorf("program(%q) accepted", bad)
		}
	}
}

func TestDestArgsEndOptionsAndRefuseAnOption(t *testing.T) {
	for _, ok := range []string{"host", "web.example.test", "deploy@web.example.test", "git@github.com",
		"first.last@10.0.0.1", "root@[fe80::1%en0]", "fe80::1", "u_1+ci@h-1"} {
		if got, err := destArgs(ok); err != nil || !slices.Equal(got, []string{"--", ok}) {
			t.Errorf("destArgs(%q) = %v, %v", ok, got, err)
		}
	}
	for _, bad := range []string{"", "-oProxyCommand=touch /tmp/x", "user@-oProxyCommand=x", "-l", "a b", "h;id",
		"h`id`", "h$(id)", "u@h\n", "u@@h", "'h'", "@h"} {
		if _, err := destArgs(bad); err == nil {
			t.Errorf("destArgs(%q) accepted", bad)
		}
	}
}
