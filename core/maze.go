package core

// Each cell owns its north and west wall; its south and east walls are the
// north and west walls of the neighbouring cells (the grid wraps).
const (
	wallN uint8 = 1
	wallW uint8 = 2
)

// Tile kinds.
const (
	TileEmpty uint8 = 0
	TileWall  uint8 = 1
)

// Maze is the cell-level wall layout.
type Maze [NumCells]uint8

func cellIndex(cx, cy int32) int32 {
	return ((cy%CellsY+CellsY)%CellsY)*CellsX + (cx%CellsX+CellsX)%CellsX
}

// cellNeighbour returns the neighbour of cell c in direction k (0=N,1=E,2=S,3=W).
func cellNeighbour(c int32, k int) int32 {
	cx, cy := c%CellsX, c/CellsX
	switch k {
	case 0:
		return cellIndex(cx, cy-1)
	case 1:
		return cellIndex(cx+1, cy)
	case 2:
		return cellIndex(cx, cy+1)
	}
	return cellIndex(cx-1, cy)
}

// Open reports whether there is no wall between cell c and its neighbour k.
func (m *Maze) Open(c int32, k int) bool {
	switch k {
	case 0:
		return m[c]&wallN == 0
	case 1:
		return m[cellNeighbour(c, 1)]&wallW == 0
	case 2:
		return m[cellNeighbour(c, 2)]&wallN == 0
	}
	return m[c]&wallW == 0
}

func (m *Maze) open(c int32, k int) {
	switch k {
	case 0:
		m[c] &^= wallN
	case 1:
		m[cellNeighbour(c, 1)] &^= wallW
	case 2:
		m[cellNeighbour(c, 2)] &^= wallN
	default:
		m[c] &^= wallW
	}
}

// Maze tuning (docs/decisions/0007): the original's 64 extra wall
// knock-outs, then most remaining dead ends get a second exit.
const (
	mazeKnockouts = 64
	mazeBraid     = 12 // out of 16: chance a dead end is opened up
)

// GenerateMaze builds a connected toroidal maze with long corridors, plenty
// of loops and few dead ends, so every part of it is reachable without long
// detours.
func GenerateMaze(r *Rand) Maze {
	m := generateWalker(r, mazeKnockouts)
	braid(r, &m, mazeBraid)
	return m
}

// generateWalker grows the tree the way the original did: from a random
// cell next to the tree, join it on, then keep walking straight runs of
// 1–4 cells in random directions through untouched cells, carving as it
// goes, until it runs into the tree; then start again elsewhere. That gives
// long corridors rather than many short spurs. attempts random walls are
// then knocked out (some already open), adding loops.
func generateWalker(r *Rand, attempts int) Maze {
	var m Maze
	for i := range m {
		m[i] = wallN | wallW
	}
	var in [NumCells]bool
	first := int32(r.Int(NumCells))
	in[first] = true
	left := NumCells - 1
	for left > 0 {
		c := int32(r.Int(NumCells))
		if in[c] {
			continue
		}
		// join c to the tree through the first tree neighbour, starting at a random side
		k0 := int(r.Int(4))
		joined := false
		for i := 0; i < 4; i++ {
			k := (k0 + i) & 3
			if in[cellNeighbour(c, k)] {
				m.open(c, k)
				joined = true
				break
			}
		}
		if !joined {
			continue
		}
		in[c] = true
		left--
		for {
			k := int(r.Int(4))
			run := 1 + int(r.Int(4))
			for ; run > 0; run-- {
				n := cellNeighbour(c, k)
				if in[n] {
					break
				}
				m.open(c, k)
				in[n] = true
				left--
				c = n
			}
			if run > 0 {
				break
			}
		}
	}
	for i := 0; i < attempts; i++ {
		c := int32(r.Int(NumCells))
		m.open(c, int(r.Int(4)))
	}
	return m
}

// braid gives each dead end a second exit with probability num/16, opening
// the wall towards a random neighbour.
func braid(r *Rand, m *Maze, num uint32) {
	for c := int32(0); c < NumCells; c++ {
		open, closed := 0, [4]int{}
		nc := 0
		for k := 0; k < 4; k++ {
			if m.Open(c, k) {
				open++
			} else {
				closed[nc] = k
				nc++
			}
		}
		if open == 1 && r.Int(16) < num {
			m.open(c, closed[r.Int(uint32(nc))])
		}
	}
}

// Rasterize draws the maze as GridWidth × GridHeight tiles. A cell's north
// wall is tile row cy*6, its west wall tile column cx*8. A corner post is
// solid when any of the four wall segments meeting at it is present.
func (m *Maze) Rasterize(tiles []uint8) {
	for i := range tiles {
		tiles[i] = TileEmpty
	}
	for c := int32(0); c < NumCells; c++ {
		x0, y0 := (c%CellsX)*CellWidth, (c/CellsX)*CellHeight
		if m[c]&wallN != 0 {
			for x := x0; x <= x0+CellWidth; x++ {
				tiles[y0*GridWidth+wrapX(x)] = TileWall
			}
		}
		if m[c]&wallW != 0 {
			for y := y0; y <= y0+CellHeight; y++ {
				tiles[wrapY(y)*GridWidth+x0] = TileWall
			}
		}
	}
}
