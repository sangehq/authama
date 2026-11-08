package desktop

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The notification icon and the favicon are one mark. Every number icon.go
// draws with appears in the favicon's SVG, so changing the favicon without the
// icon (or the reverse) fails here.
func TestIconMatchesTheFavicon(t *testing.T) {
	svg, err := os.ReadFile("../../web/assets/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	s := string(svg)
	f := func(v float64) string {
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", v), "0"), ".0")
	}
	want := []string{
		`viewBox="0 0 32 32"`, `rx="7"`, `fill="#066fd1"`,
		fmt.Sprintf(`stroke-width="%s"`, f(glyphWidth)),
		fmt.Sprintf(`rotate(%s 16 16)`, f(glyphTurn)),
		fmt.Sprintf(`cx="%s" cy="16" r="%s"`, f(bowX), f(bowRadius)),
	}
	for _, seg := range glyphPath {
		a, b := seg[0], seg[1]
		if a[1] == b[1] {
			want = append(want, fmt.Sprintf(`M%s %sH%s`, f(a[0]), f(a[1]), f(b[0])))
		} else {
			want = append(want, fmt.Sprintf(`M%s %sV%s`, f(a[0]), f(a[1]), f(b[1])))
		}
	}
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("favicon.svg has no %q; icon.go and the favicon disagree", w)
		}
	}
	// The rendered icon has both colours: blue mark, white key.
	img := drawMark(64)
	blue, white := 0, 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := img.NRGBAAt(x, y)
			switch {
			case c.A == 255 && c.R == markBlue.R && c.B == markBlue.B:
				blue++
			case c.A == 255 && c.R == 255 && c.G == 255 && c.B == 255:
				white++
			}
		}
	}
	if blue < 1500 || white < 150 {
		t.Fatalf("drawn mark has %d blue and %d white pixels; the key is not drawn", blue, white)
	}
}
