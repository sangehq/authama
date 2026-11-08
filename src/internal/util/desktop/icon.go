package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
)

// The notification icon is the web UI's logo mark, drawn from the same
// geometry as assets/favicon.svg (a 32-unit square, corner radius 7, in
// #066fd1, with a white key rotated -45 degrees about the centre), so the
// banner and the browser tab show one product. TestIconMatchesTheFavicon reads
// the favicon and fails when the two drift apart.
// It is drawn rather than committed as a PNG, so changing the mark is an edit
// to these numbers and the favicon, not a regenerated binary.
const (
	iconSize   = 1024
	markUnits  = 32.0
	markRadius = 7.0
	glyphWidth = 2.5
	// The key's bow: a ring around (bowX, centre).
	bowX      = 10.0
	bowRadius = 5.25
	// The glyph is drawn upright and turned by glyphTurn degrees about the
	// centre, as the favicon's transform="rotate(-45 16 16)" does.
	glyphTurn = -45.0
)

var (
	markBlue  = color.NRGBA{R: 0x06, G: 0x6f, B: 0xd1, A: 0xff}
	glyphPath = [][2][2]float64{ // the shaft and its two teeth, in favicon units
		{{15.25, 16}, {27.25, 16}},
		{{21.75, 16}, {21.75, 21.25}},
		{{25.25, 16}, {25.25, 19.25}},
	}
)

var (
	iconOnce sync.Once
	iconPNG  []byte
)

// IconPNG is the logo mark as a 1024px PNG, drawn once per process.
func IconPNG() []byte {
	iconOnce.Do(func() {
		var b bytes.Buffer
		_ = png.Encode(&b, drawMark(iconSize))
		iconPNG = b.Bytes()
	})
	return iconPNG
}

// drawMark renders the mark at size x size, 4x4 supersampled.
func drawMark(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := markUnits / float64(size)
	const ss = 4
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var inMark, inGlyph int
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) * scale
					y := (float64(py) + (float64(sy)+0.5)/ss) * scale
					if !inRoundedSquare(x, y) {
						continue
					}
					inMark++
					if onGlyph(x, y) {
						inGlyph++
					}
				}
			}
			if inMark == 0 {
				continue
			}
			// Mix white over blue by glyph coverage; alpha is the mark's coverage.
			t := float64(inGlyph) / float64(inMark)
			mix := func(c uint8) uint8 { return uint8(math.Round(float64(c)*(1-t) + 255*t)) }
			img.SetNRGBA(px, py, color.NRGBA{
				R: mix(markBlue.R), G: mix(markBlue.G), B: mix(markBlue.B),
				A: uint8(math.Round(255 * float64(inMark) / ss / ss)),
			})
		}
	}
	return img
}

func inRoundedSquare(x, y float64) bool {
	cx := math.Min(math.Max(x, markRadius), markUnits-markRadius)
	cy := math.Min(math.Max(y, markRadius), markUnits-markRadius)
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= markRadius*markRadius
}

// onGlyph is within half the stroke width of the bow's ring or of a segment
// (round caps and joins), once (x, y) is turned back into the upright glyph.
func onGlyph(x, y float64) bool {
	const c = markUnits / 2
	a := -glyphTurn * math.Pi / 180
	x, y = c+(x-c)*math.Cos(a)-(y-c)*math.Sin(a), c+(x-c)*math.Sin(a)+(y-c)*math.Cos(a)
	if math.Abs(math.Hypot(x-bowX, y-c)-bowRadius) <= glyphWidth/2 {
		return true
	}
	for _, s := range glyphPath {
		ax, ay, bx, by := s[0][0], s[0][1], s[1][0], s[1][1]
		dx, dy := bx-ax, by-ay
		t := ((x-ax)*dx + (y-ay)*dy) / (dx*dx + dy*dy)
		t = math.Max(0, math.Min(1, t))
		qx, qy := ax+t*dx-x, ay+t*dy-y
		if qx*qx+qy*qy <= glyphWidth*glyphWidth/4 {
			return true
		}
	}
	return false
}
