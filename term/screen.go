// Package term is NLSNIPES's own small terminal layer: a 40 × 25 frame of
// coloured cells written with plain ANSI sequences, and keyboard input with
// real key releases where the platform reports them (Windows console API,
// the kitty keyboard protocol) and a tuned emulation elsewhere. See
// docs/decisions/0005-own-terminal-layer.md for why this is not tcell.
package term

import (
	"bytes"
	"strconv"
	"unicode/utf8"
)

// Screen size in characters: 3 HUD rows above a 40 × 22 viewport.
const (
	Width    = 40
	Height   = 25
	HUDRows  = 3
	ViewRows = Height - HUDRows
)

// CGA colour indices, as the original used them.
const (
	Black uint8 = iota
	Blue
	Green
	Cyan
	Red
	Magenta
	Brown
	LightGray
	DarkGray
	LightBlue
	LightGreen
	LightCyan
	LightRed
	LightMagenta
	Yellow
	White
)

// Cell is one character position.
type Cell struct {
	Ch     rune
	Fg, Bg uint8
}

// Frame is a full screen.
type Frame [Height][Width]Cell

// Clear fills the frame with blank black cells.
func (f *Frame) Clear() {
	for y := range f {
		for x := range f[y] {
			f[y][x] = Cell{Ch: ' ', Fg: LightGray}
		}
	}
}

// Text writes s at (x, y) in the given colours, clipped to the frame.
func (f *Frame) Text(x, y int, s string, fg, bg uint8) {
	if y < 0 || y >= Height {
		return
	}
	for _, r := range s {
		if x >= 0 && x < Width {
			f[y][x] = Cell{Ch: r, Fg: fg, Bg: bg}
		}
		x++
	}
}

// Line returns row y as plain text (for tests).
func (f *Frame) Line(y int) string {
	var b []rune
	for _, c := range f[y] {
		b = append(b, c.Ch)
	}
	return string(b)
}

// cgaToANSI maps a CGA index to the ANSI 16-colour index.
var cgaToANSI = [16]int{0, 4, 2, 6, 1, 5, 3, 7, 8, 12, 10, 14, 9, 13, 11, 15}

// Renderer turns frames into ANSI output, sending only cells that changed.
type Renderer struct {
	prev     Frame
	valid    bool // prev reflects what is on the terminal
	termW    int
	termH    int
	buf      bytes.Buffer
	fg, bg   int
	curX     int
	curY     int
	tooSmall bool
}

// Invalidate forces the next Render to redraw everything.
func (r *Renderer) Invalidate() { r.valid = false }

// Render returns the bytes that bring a terminal of size w × h from the
// previous frame to f. The 40 × 25 frame is centred and never scaled.
func (r *Renderer) Render(f *Frame, w, h int) []byte {
	r.buf.Reset()
	if w != r.termW || h != r.termH {
		r.termW, r.termH, r.valid = w, h, false
	}
	if w < Width || h < Height {
		if !r.tooSmall || !r.valid {
			r.buf.WriteString("\x1b[0m\x1b[2J\x1b[H")
			r.buf.WriteString("Please enlarge the terminal to at least 40 x 25 (now " +
				strconv.Itoa(w) + " x " + strconv.Itoa(h) + ").")
		}
		r.tooSmall, r.valid = true, true
		return r.buf.Bytes()
	}
	if r.tooSmall {
		r.tooSmall, r.valid = false, false
	}
	ox, oy := (w-Width)/2, (h-Height)/2
	if !r.valid {
		r.buf.WriteString("\x1b[0m\x1b[2J")
		r.fg, r.bg = -1, -1
	}
	r.curX, r.curY = -1, -1
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			c := f[y][x]
			if r.valid && r.prev[y][x] == c {
				continue
			}
			if r.curX != x || r.curY != y {
				r.buf.WriteString("\x1b[")
				r.buf.WriteString(strconv.Itoa(oy + y + 1))
				r.buf.WriteByte(';')
				r.buf.WriteString(strconv.Itoa(ox + x + 1))
				r.buf.WriteByte('H')
			}
			r.colour(int(c.Fg), int(c.Bg))
			ch := c.Ch
			if ch < ' ' || ch == utf8.RuneError {
				ch = ' '
			}
			r.buf.WriteRune(ch)
			// Some terminals draw a few of these symbols wider than one
			// cell; re-position after anything non-ASCII so one wide glyph
			// can never shift the rest of the row.
			if ch < 0x80 {
				r.curX, r.curY = x+1, y
			} else {
				r.curX = -1
			}
		}
	}
	r.prev, r.valid = *f, true
	return r.buf.Bytes()
}

func (r *Renderer) colour(fg, bg int) {
	if fg == r.fg && bg == r.bg {
		return
	}
	r.buf.WriteString("\x1b[0;")
	a := cgaToANSI[fg&15]
	if a < 8 {
		r.buf.WriteString(strconv.Itoa(30 + a))
	} else {
		r.buf.WriteString(strconv.Itoa(90 + a - 8))
	}
	r.buf.WriteByte(';')
	b := cgaToANSI[bg&15]
	if b < 8 {
		r.buf.WriteString(strconv.Itoa(40 + b))
	} else {
		r.buf.WriteString(strconv.Itoa(100 + b - 8))
	}
	r.buf.WriteByte('m')
	r.fg, r.bg = fg, bg
}
