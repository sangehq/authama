package web

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16LE(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func TestFileTextDecodesWhatWindowsToolsWrite(t *testing.T) {
	for name, c := range map[string]struct {
		in   []byte
		want string
		ok   bool
	}{
		"utf-16le, crlf": {utf16LE("Windows Registry Editor Version 5.00\r\n[HKEY]\r\n"), "Windows Registry Editor Version 5.00\n[HKEY]\n", true},
		"utf-16be":       {[]byte{0xFE, 0xFF, 0x00, 'h', 0x00, 'i'}, "hi", true},
		"utf-8 with bom": {[]byte("\xEF\xBB\xBFHost web\n"), "Host web\n", true},
		"plain utf-8":    {[]byte("Host web\n"), "Host web\n", true},
		"binary":         {[]byte{0x1F, 0x8B, 0xFF, 0x00, 0xC3}, "", false},
	} {
		got, ok := fileText(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: (%q, %v), want (%q, %v)", name, got, ok, c.want, c.ok)
		}
	}
}

// The Export / Import preview shows PuTTY's .reg, which is UTF-16, as text:
// it used to show its size alone, as for a binary file.
func TestTheTransferPreviewReadsUTF16(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := signIn(t, s).get("/transfer?preview=1&part=ssh-clients&client=putty").Body.String()
	if !strings.Contains(body, "Windows Registry Editor") {
		t.Errorf("the PuTTY .reg preview is not decoded:\n%.2000s", body[strings.Index(body, "clients/putty"):])
	}
}
