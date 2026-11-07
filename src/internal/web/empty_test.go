package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// An empty state can name the command that fills it: a backticked span is
// set as code, and nothing else in the text becomes markup.
func TestEmptySubtitleSetsCommandsAsCode(t *testing.T) {
	for text, want := range map[string]string{
		"Run `authama ssh reconcile`, then `authama ssh deploy`.": "Run <code>authama ssh reconcile</code>, then <code>authama ssh deploy</code>.",
		"Nothing to do.": "Nothing to do.",
		"<b>x</b> & `<i>y</i>`": "&lt;b&gt;x&lt;/b&gt; &amp; <code>&lt;i&gt;y&lt;/i&gt;</code>",
		"an unclosed ` backtick": "an unclosed ` backtick",
	} {
		if got := string(emptyState{Text: text}.Subtitle()); got != want {
			t.Errorf("%q:\n got %s\nwant %s", text, got, want)
		}
	}
}

// A list with nothing in it uses the shared empty state, which names its
// command as code. Plain card text that names one is the drift this stops.
func TestNoPlainEmptyStateNamesACommand(t *testing.T) {
	plain := regexp.MustCompile(`\{\{else\}\}<div class="card-body text-secondary">[^\n]*<code>`)
	files := 0
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		files++
		b, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		for _, m := range plain.FindAllString(string(b), -1) {
			t.Errorf("%s: an empty state in plain card text; use the empty partial:\n\t%s", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 30 {
		t.Fatalf("read %d templates; the scan is not reading them", files)
	}
}
