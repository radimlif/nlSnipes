// Package term renders the game in a 40 × 25 terminal window with tcell and
// maps key presses to input masks. Implemented in milestone L2.
package term

// Screen size in characters: 3 HUD rows above a 40 × 22 viewport.
const (
	Width    = 40
	Height   = 25
	HUDRows  = 3
	ViewRows = Height - HUDRows
)
