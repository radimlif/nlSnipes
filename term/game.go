package term

import "github.com/radimlif/nlSnipes/core"

// Look of the game, after the 1982 original: CP437 glyphs drawn with their
// Unicode equivalents and the original's CGA colours.
var (
	// PlayerColours are the slot colours: white, yellow, cyan, magenta.
	PlayerColours = [core.MaxPlayers]uint8{White, Yellow, LightCyan, LightMagenta}

	wallColour  = LightBlue
	snipeColour = Green
	spearColour = LightGreen

	// hiveColours cycle to make the portals shimmer; every other frame is blank.
	hiveColours = [8]uint8{White, Yellow, LightMagenta, LightRed, LightCyan, LightGreen, LightBlue, Red}

	// Explosion frames (6 × 3 ticks = core.DebrisLife).
	boomColours = [6]uint8{White, LightCyan, LightRed, Red, Brown, DarkGray}
	boomA       = [6]rune{'░', '▓', '░', '▓', '*', '·'}
	boomB       = [6]rune{'▓', '░', '▓', '░', '☼', ' '}

	// spearGlyph by core direction E, SE, S, SW, W, NW, N, NE.
	spearGlyph = [8]rune{'→', '\\', '↓', '/', '←', '\\', '↑', '/'}
)

// wallGlyph picks the double-line box character from which neighbours are
// walls: bit 0 north, 1 east, 2 south, 3 west.
var wallGlyph = [16]rune{
	'╬', '║', '═', '╚', '║', '║', '╔', '╠',
	'═', '╝', '═', '╩', '╗', '╣', '╦', '╬',
}

// Camera is the world tile shown at the centre of the viewport.
type Camera struct{ X, Y int32 }

// CameraOn centres the camera on entity e.
func CameraOn(e *core.Entity) Camera { return Camera{e.X + 1, e.Y} }

// DrawWorld draws the 40 × 22 viewport (frame rows HUDRows..Height-1).
func DrawWorld(f *Frame, s *core.State, cam Camera) {
	left := cam.X - Width/2
	top := cam.Y - ViewRows/2
	wall := func(x, y int32) bool { return s.Wall(x, y) }
	for sy := 0; sy < ViewRows; sy++ {
		for sx := 0; sx < Width; sx++ {
			x, y := left+int32(sx), top+int32(sy)
			c := Cell{Ch: ' ', Fg: LightGray}
			if wall(x, y) {
				m := 0
				if wall(x, y-1) {
					m |= 1
				}
				if wall(x+1, y) {
					m |= 2
				}
				if wall(x, y+1) {
					m |= 4
				}
				if wall(x-1, y) {
					m |= 8
				}
				c = Cell{Ch: wallGlyph[m], Fg: wallColour}
			}
			f[HUDRows+sy][sx] = c
		}
	}
	put := func(x, y int32, ch rune, fg uint8) {
		sx := core.WrapDelta(left, x, core.GridWidth)
		sy := core.WrapDelta(top, y, core.GridHeight)
		if sx < 0 {
			sx += core.GridWidth
		}
		if sy < 0 {
			sy += core.GridHeight
		}
		if sx < Width && sy < ViewRows {
			f[HUDRows+int(sy)][sx] = Cell{Ch: ch, Fg: fg}
		}
	}
	tick := s.Tick
	for _, order := range [...]core.Kind{core.KindDebris, core.KindHive, core.KindSnipe, core.KindGhost, core.KindSpear, core.KindBullet, core.KindPlayer} {
		for i := range s.Ents {
			e := &s.Ents[i]
			if e.Kind != order || e.Dead {
				continue
			}
			switch e.Kind {
			case core.KindDebris:
				age := min(max((core.DebrisLife-e.Timer)/3, 0), 5)
				w, h := core.Kind(e.Dir).Size()
				for dy := int32(0); dy < h; dy++ {
					for dx := int32(0); dx < w; dx++ {
						ch := boomA[age]
						if (dx+dy)%2 == 1 {
							ch = boomB[age]
						}
						put(e.X+dx, e.Y+dy, ch, boomColours[age])
					}
				}
			case core.KindHive:
				phase := (uint32(i) + tick/2) % 16
				col := hiveColours[phase/2]
				if phase%2 == 1 {
					put(e.X, e.Y, ' ', col)
					put(e.X+1, e.Y, ' ', col)
					put(e.X, e.Y+1, ' ', col)
					put(e.X+1, e.Y+1, ' ', col)
					continue
				}
				put(e.X, e.Y, '┌', col)
				put(e.X+1, e.Y, '┐', col)
				put(e.X, e.Y+1, '└', col)
				put(e.X+1, e.Y+1, '┘', col)
			case core.KindSnipe:
				switch e.Dir {
				case core.DirN:
					put(e.X, e.Y, '☺', snipeColour)
					put(e.X+1, e.Y, '↑', snipeColour)
				case core.DirS:
					put(e.X, e.Y, '☺', snipeColour)
					put(e.X+1, e.Y, '↓', snipeColour)
				case core.DirSW, core.DirW, core.DirNW:
					put(e.X, e.Y, '←', snipeColour)
					put(e.X+1, e.Y, '☺', snipeColour)
				default:
					put(e.X, e.Y, '☺', snipeColour)
					put(e.X+1, e.Y, '→', snipeColour)
				}
			case core.KindGhost:
				put(e.X, e.Y, '☻', snipeColour)
			case core.KindSpear:
				put(e.X, e.Y, spearGlyph[e.Dir&7], spearColour)
			case core.KindBullet:
				if tick%2 == 0 {
					put(e.X, e.Y, '○', Yellow)
				} else {
					put(e.X, e.Y, '☼', LightCyan)
				}
			case core.KindPlayer:
				col := PlayerColours[e.Owner]
				eye := 'ô'
				if (e.X+e.Y)%2 == 1 { // eyes change as you walk
					eye = 'O'
				}
				put(e.X, e.Y, eye, col)
				put(e.X+1, e.Y, eye, col)
				put(e.X, e.Y+1, '◄', col)
				put(e.X+1, e.Y+1, '►', col)
			}
		}
	}
}
