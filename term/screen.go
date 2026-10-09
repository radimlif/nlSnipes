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

// Classic screen size in characters: 3 HUD rows above a 40 × 22 viewport,
// the original's 40 × 25 text mode. Frames can be larger (the full view).
const (
	Width    = 40
	Height   = 25
	HUDRows  = 3
	ViewRows = Height - HUDRows
)

// Largest useful frame: wider or taller would show the toroidal maze twice.
const (
	MaxWidth  = 120
	MaxHeight = HUDRows + 110
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

// Frame is a screen of W × H cells.
type Frame struct {
	W, H  int
	Cells []Cell
}

// NewFrame returns a cleared frame.
func NewFrame(w, h int) *Frame {
	f := &Frame{}
	f.Resize(w, h)
	return f
}

// Resize changes the frame size and clears it.
func (f *Frame) Resize(w, h int) {
	f.W, f.H = w, h
	if cap(f.Cells) < w*h {
		f.Cells = make([]Cell, w*h)
	}
	f.Cells = f.Cells[:w*h]
	f.Clear()
}

// Clear fills the frame with blank black cells.
func (f *Frame) Clear() {
	for i := range f.Cells {
		f.Cells[i] = Cell{Ch: ' ', Fg: LightGray}
	}
}

// Set writes one cell; positions outside the frame are ignored.
func (f *Frame) Set(x, y int, c Cell) {
	if x >= 0 && x < f.W && y >= 0 && y < f.H {
		f.Cells[y*f.W+x] = c
	}
}

// At returns the cell at (x, y).
func (f *Frame) At(x, y int) Cell { return f.Cells[y*f.W+x] }

// Text writes s at (x, y) in the given colours, clipped to the frame.
func (f *Frame) Text(x, y int, s string, fg, bg uint8) {
	for _, r := range s {
		f.Set(x, y, Cell{Ch: r, Fg: fg, Bg: bg})
		x++
	}
}

// Line returns row y as plain text (for tests).
func (f *Frame) Line(y int) string {
	var b []rune
	for _, c := range f.Cells[y*f.W : (y+1)*f.W] {
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
	fw, fh   int // size of the last frame rendered
	buf      bytes.Buffer
	fg, bg   int
	curX     int
	curY     int
	tooSmall bool
}

// Invalidate forces the next Render to redraw everything.
func (r *Renderer) Invalidate() { r.valid = false }

// Render returns the bytes that bring a terminal of size w × h from the
// previous frame to f. The frame is centred and never scaled.
func (r *Renderer) Render(f *Frame, w, h int) []byte {
	r.buf.Reset()
	if w != r.termW || h != r.termH || f.W != r.fw || f.H != r.fh {
		r.termW, r.termH, r.fw, r.fh, r.valid = w, h, f.W, f.H, false
	}
	if w < f.W || h < f.H {
		if !r.tooSmall || !r.valid {
			r.buf.WriteString("\x1b[0m\x1b[2J\x1b[H")
			r.buf.WriteString("Please enlarge the terminal to at least " + strconv.Itoa(f.W) + " x " + strconv.Itoa(f.H) + " (now " +
				strconv.Itoa(w) + " x " + strconv.Itoa(h) + ").")
		}
		r.tooSmall, r.valid = true, true
		return r.buf.Bytes()
	}
	if r.tooSmall {
		r.tooSmall, r.valid = false, false
	}
	ox, oy := (w-f.W)/2, (h-f.H)/2
	if !r.valid {
		r.buf.WriteString("\x1b[0m\x1b[2J")
		r.fg, r.bg = -1, -1
	}
	r.curX, r.curY = -1, -1
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			c := f.Cells[y*f.W+x]
			if r.valid && r.prev.Cells[y*f.W+x] == c {
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
	r.prev.W, r.prev.H = f.W, f.H
	r.prev.Cells = append(r.prev.Cells[:0], f.Cells...)
	r.valid = true
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
