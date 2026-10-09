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
	}
}

func cellDistances(m *Maze, from int32) [NumCells]int32 {
	var d [NumCells]int32
	for i := range d {
		d[i] = -1
	}
	d[from] = 0
	q := []int32{from}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for k := 0; k < 4; k++ {
			if n := cellNeighbour(c, k); m.Open(c, k) && d[n] < 0 {
				d[n] = d[c] + 1
				q = append(q, n)
			}
		}
	}
	return d
}

// TestMazeIsEasyToGetAround pins the playability targets from ADR 0007:
// short detours, few dead ends, nothing far away.
func TestMazeIsEasyToGetAround(t *testing.T) {
	var detour, pairs float64
	deadEnds, maxDist := 0, int32(0)
	const seeds = 200
	for seed := uint32(0); seed < seeds; seed++ {
		r := NewRand(seed)
		m := GenerateMaze(&r)
		for c := int32(0); c < NumCells; c++ {
			open := 0
			for k := 0; k < 4; k++ {
				if m.Open(c, k) {
					open++
				}
			}
			if open == 1 {
				deadEnds++
			}
		}
		for i := 0; i < 8; i++ {
			a := int32(r.Int(NumCells))
			d := cellDistances(&m, a)
			for b := int32(0); b < NumCells; b++ {
				man := abs32(WrapDelta(a%CellsX, b%CellsX, CellsX)) + abs32(WrapDelta(a/CellsX, b/CellsX, CellsY))
				if man >= 3 {
					detour += float64(d[b]) / float64(man)
					pairs++
				}
				maxDist = max(maxDist, d[b])
			}
		}
	}
	avg, deadPct := detour/pairs, 100*float64(deadEnds)/float64(seeds*NumCells)
	t.Logf("detour ×%.2f, dead ends %.1f%%, farthest %d cells", avg, deadPct, maxDist)
	if avg > 1.5 || deadPct > 8 || maxDist > 40 {
		t.Fatalf("maze too closed: detour ×%.2f (≤ 1.5), dead ends %.1f%% (≤ 8), farthest %d (≤ 40)", avg, deadPct, maxDist)
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
