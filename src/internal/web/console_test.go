package web

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The console reads each line's status word, subject and text, and how the
// line reads, so the page can colour and align a job's plain output.
func TestTheConsoleReadsEachLine(t *testing.T) {
	for _, tc := range []struct {
		raw                   string
		indent                int
		tag, kind, name, text string
	}{
		{"ok    simtabi/web-ed25519", 0, "ok", "ok", "simtabi/web-ed25519", ""},
		{"  FAIL  web-00: answers, but the current key does not log in", 1, "FAIL", "bad", "web-00", "answers, but the current key does not log in"},
		{"  paste oribi-web: ploi takes the new key by hand", 1, "paste", "warn", "oribi-web", "ploi takes the new key by hand"},
		{"rotation FAILED: work/gh", 0, "", "bad", "", "rotation FAILED: work/gh"},
		{"WARNING: proceeding without a key backup", 0, "WARNING", "warn", "", "proceeding without a key backup"},
		{"key backup: /tmp/b.age", 0, "", "", "", "key backup: /tmp/b.age"},
		{"", 0, "", "", "", ""},
	} {
		got := readConsoleLine(tc.raw)
		if got.Indent != tc.indent || got.Tag != tc.tag || got.Kind != tc.kind || got.Name != tc.name || got.Text != tc.text {
			t.Errorf("%q read as %+v", tc.raw, got)
		}
	}
}

// The page's script reads lines that arrive while a job runs with the same
// status words as the server: a word known to only one would colour a line
// differently before and after a reload.
func TestTheConsoleScriptKnowsTheSameStatusWords(t *testing.T) {
	b, err := os.ReadFile("assets/js/components/console.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)const TAGS = \{(.*?)\};`).FindSubmatch(b)
	if block == nil {
		t.Fatal("console.js has no TAGS table")
	}
	var js []string
	for _, m := range regexp.MustCompile(`"?([A-Za-z:]+)"?:\s*"(\w+)"`).FindAllSubmatch(block[1], -1) {
		js = append(js, string(m[1])+"="+string(m[2]))
	}
	var gosrc []string
	for k, v := range consoleTags {
		gosrc = append(gosrc, k+"="+v)
	}
	sort.Strings(js)
	sort.Strings(gosrc)
	if len(gosrc) < 10 || strings.Join(js, " ") != strings.Join(gosrc, " ") {
		t.Errorf("status words differ:\n script %v\n server %v", js, gosrc)
	}
}

// Nested lines are drawn as a tree: each is ├─ but the last of its group,
// which is └─.
func TestConsoleLinesKnowTheLastOfEachGroup(t *testing.T) {
	ls := consoleLines([]string{
		"ok    work/a",
		"  ok    a-1: logs in",
		"  paste a-2: by hand",
		"FAIL  work/b",
		"  FAIL  b-1: unreachable",
	})
	var got []bool
	for _, l := range ls {
		got = append(got, l.Last)
	}
	if want := []bool{false, false, true, false, true}; strings.Join(strings.Fields(strings.Trim(fmt.Sprint(got), "[]")), ",") != strings.Join(strings.Fields(strings.Trim(fmt.Sprint(want), "[]")), ",") {
		t.Errorf("last flags = %v, want %v", got, want)
	}
}
