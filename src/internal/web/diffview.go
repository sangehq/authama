package web

import (
	"html/template"
	"strings"

	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/highlight"
)

// diffView is one file's diff as the "diff" and "file_diff" partials show
// it: numbered, highlighted lines in the file's own syntax, with the counts
// and the validity check in the header.
type diffView struct {
	Path, State    string
	Lang           highlight.Lang
	Rows           []history.DiffLine
	Added, Removed int
	Checked        bool
	Problem        string
}

func newDiffView(path, state string, lines []history.DiffLine, checked bool, problem string) diffView {
	v := diffView{Path: path, State: state, Lang: highlight.For(path), Rows: lines, Checked: checked, Problem: problem}
	for _, l := range lines {
		switch l.Op {
		case '+':
			v.Added++
		case '-':
			v.Removed++
		}
	}
	return v
}

// LangLabel names the syntax for the header, or "" for plain text.
func (v diffView) LangLabel() string {
	return langLabels[v.Lang]
}

var langLabels = map[highlight.Lang]string{
	highlight.JSON: "JSON", highlight.INI: "INI", highlight.TOML: "TOML", highlight.YAML: "YAML",
	highlight.Properties: "properties", highlight.SSHConfig: "ssh_config", highlight.Netrc: "netrc",
	highlight.Shell: "shell",
}

// highlightHTML renders a line's tokens as escaped spans. The classes are the
// only markup, so the page's CSP (no inline style) holds.
func highlightHTML(lang highlight.Lang, line string) template.HTML {
	var b strings.Builder
	for _, t := range highlight.Line(lang, line) {
		text := template.HTMLEscapeString(t.Text)
		if t.Kind == highlight.Text {
			b.WriteString(text)
			continue
		}
		b.WriteString(`<span class="tok-` + string(t.Kind) + `">` + text + `</span>`)
	}
	return template.HTML(b.String()) //nolint:gosec // every piece of text is escaped above; the class is a fixed Kind
}
