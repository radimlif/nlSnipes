package core

import "strings"

var dumpGlyph = [8]byte{' ', '1', 'H', 'S', 's', '*', '!', '.'}

// Dump renders the whole map as ASCII for tests and debugging: '#' wall,
// players as their slot number 1–4, 'H' hive, 'S' large snipe, 's' small
// snipe, '*' bullet, '!' spear, '.' debris.
func (s *State) Dump() string {
	grid := make([]byte, GridWidth*GridHeight)
	for i, t := range s.Tiles {
		grid[i] = ' '
		if t == TileWall {
			grid[i] = '#'
		}
	}
	for _, e := range s.Ents {
		if e.Dead {
			continue
		}
		g := dumpGlyph[e.Kind]
		if e.Kind == KindPlayer {
			g = '1' + byte(e.Owner)
		}
		w, h := e.Kind.Size()
		for dy := int32(0); dy < h; dy++ {
			for dx := int32(0); dx < w; dx++ {
				i := wrapY(e.Y+dy)*GridWidth + wrapX(e.X+dx)
				if e.Kind != KindDebris || grid[i] == ' ' {
					grid[i] = g
				}
			}
		}
	}
	var sb strings.Builder
	sb.Grow(len(grid) + GridHeight)
	for y := 0; y < GridHeight; y++ {
		sb.Write(grid[y*GridWidth : (y+1)*GridWidth])
		sb.WriteByte('\n')
	}
	return sb.String()
}
