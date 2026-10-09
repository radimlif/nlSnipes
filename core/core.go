// Package core is the deterministic Snipes simulation: maze, entities, snipe
// AI, scoring and lives. It is pure — no I/O, no clocks, no math/rand, no
// floats — so the same seed and inputs always produce the same state hash.
//
// Implemented in milestone L1; see docs/DESIGN-LIGHT.md and docs/DESIGN.md §6.
package core

// TicksPerSecond is the fixed simulation rate, as in the original (18.2 Hz).
const TicksPerSecond = 18

// Grid size of the toroidal maze in characters (16 × 20 cells of 8 × 6).
const (
	GridWidth  = 128
	GridHeight = 120
)
