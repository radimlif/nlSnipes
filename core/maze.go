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

// GenerateMaze grows a random spanning tree over the toroidal cell grid
// (randomised Prim: join a random frontier cell to a random visited
// neighbour), then knocks out 3–5 extra walls so there are some loops.
func GenerateMaze(r *Rand) Maze {
	var m Maze
	for i := range m {
		m[i] = wallN | wallW
	}
	var visited, queued [NumCells]bool
	frontier := make([]int32, 0, NumCells)
	visit := func(c int32) {
		visited[c] = true
		for k := 0; k < 4; k++ {
			n := cellNeighbour(c, k)
			if !visited[n] && !queued[n] {
				queued[n] = true
				frontier = append(frontier, n)
			}
		}
	}
	visit(int32(r.Int(NumCells)))
	var dirs [4]int
	for len(frontier) > 0 {
		i := r.Int(uint32(len(frontier)))
		c := frontier[i]
		frontier[i] = frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		nd := 0
		for k := 0; k < 4; k++ {
			if visited[cellNeighbour(c, k)] {
				dirs[nd] = k
				nd++
			}
		}
		m.open(c, dirs[r.Int(uint32(nd))])
		visit(c)
	}
	for extra := 3 + r.Int(3); extra > 0; {
		c := int32(r.Int(NumCells))
		w := wallN << r.Int(2)
		if m[c]&w != 0 {
			m[c] &^= w
			extra--
		}
	}
	return m
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
