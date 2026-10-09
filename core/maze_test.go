package core

import "testing"

func connectedCells(m *Maze) int {
	var seen [NumCells]bool
	queue := []int32{0}
	seen[0] = true
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for k := 0; k < 4; k++ {
			if n := cellNeighbour(c, k); m.Open(c, k) && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	n := 0
	for _, s := range seen {
		if s {
			n++
		}
	}
	return n
}

func TestMazeConnectedFor1000Seeds(t *testing.T) {
	for seed := uint32(0); seed < 1000; seed++ {
		r := NewRand(seed)
		m := GenerateMaze(&r)
		if n := connectedCells(&m); n != NumCells {
			t.Fatalf("seed %d: only %d of %d cells reachable", seed, n, NumCells)
		}
		walls := 0
		for _, c := range m {
			walls += int(c&wallN) + int(c&wallW)>>1
		}
		// A spanning tree opens NumCells-1 of the 2*NumCells walls, then 3–5 more go.
		if open := 2*NumCells - walls; open < NumCells-1+3 || open > NumCells-1+5 {
			t.Fatalf("seed %d: %d walls open, want %d..%d", seed, open, NumCells+2, NumCells+4)
		}
	}
}

func TestMazeRasterMatchesCells(t *testing.T) {
	r := NewRand(5)
	m := GenerateMaze(&r)
	tiles := make([]uint8, GridWidth*GridHeight)
	m.Rasterize(tiles)
	at := func(x, y int32) bool { return tiles[wrapY(y)*GridWidth+wrapX(x)] == TileWall }
	for c := int32(0); c < NumCells; c++ {
		x0, y0 := (c%CellsX)*CellWidth, (c/CellsX)*CellHeight
		for y := y0 + 1; y < y0+CellHeight; y++ {
			for x := x0 + 1; x < x0+CellWidth; x++ {
				if at(x, y) {
					t.Fatalf("cell %d interior (%d,%d) is wall", c, x, y)
				}
			}
		}
		if at(x0+3, y0) != !m.Open(c, 0) || at(x0, y0+3) != !m.Open(c, 3) {
			t.Fatalf("cell %d: raster walls disagree with maze", c)
		}
	}
}

func TestMazeRegenerationIsIdentical(t *testing.T) {
	for seed := uint32(0); seed < 50; seed++ {
		a, b := NewRand(seed), NewRand(seed)
		if GenerateMaze(&a) != GenerateMaze(&b) {
			t.Fatalf("seed %d: two generations differ", seed)
		}
	}
}
