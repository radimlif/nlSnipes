// Package core is the deterministic Snipes simulation: maze, entities, snipe
// AI, scoring and lives. It is pure — no I/O, no clocks, no math/rand, no
// floats — so the same seed and inputs always produce the same state hash.
//
// Rules follow docs/DESIGN.md §2 and §6; readings of open points are in
// docs/decisions/0004-core-rule-readings.md.
package core

// TicksPerSecond is the fixed simulation rate, as in the original (18.2 Hz).
const TicksPerSecond = 18

// Grid size of the toroidal maze in characters (16 × 20 cells of 8 × 6).
const (
	GridWidth  = 128
	GridHeight = 120
	CellWidth  = 8
	CellHeight = 6
	CellsX     = GridWidth / CellWidth
	CellsY     = GridHeight / CellHeight
	NumCells   = CellsX * CellsY
)

// Directions, clockwise from east. Turning ±45° is ±1 modulo 8.
const (
	DirE uint8 = iota
	DirSE
	DirS
	DirSW
	DirW
	DirNW
	DirN
	DirNE
)

var (
	dirDX = [8]int32{1, 1, 0, -1, -1, -1, 0, 1}
	dirDY = [8]int32{0, 1, 1, 1, 0, -1, -1, -1}
)

// DirDelta returns the unit step of direction d.
func DirDelta(d uint8) (dx, dy int32) { return dirDX[d&7], dirDY[d&7] }

// dirOf maps a step vector with components in {-1,0,1} to a direction.
func dirOf(dx, dy int32) (uint8, bool) {
	for d := uint8(0); d < 8; d++ {
		if dirDX[d] == dx && dirDY[d] == dy {
			return d, true
		}
	}
	return 0, false
}

func wrapX(x int32) int32 { return ((x % GridWidth) + GridWidth) % GridWidth }
func wrapY(y int32) int32 { return ((y % GridHeight) + GridHeight) % GridHeight }

// WrapDelta returns the shortest signed toroidal difference b-a on an axis of size n.
func WrapDelta(a, b, n int32) int32 {
	d := ((b-a)%n + n) % n
	if d > n/2 {
		d -= n
	}
	return d
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func sign32(v int32) int32 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
