package core

import "testing"

func TestGridIsWholeCells(t *testing.T) {
	if GridWidth%8 != 0 || GridHeight%6 != 0 || GridWidth/8 != 16 || GridHeight/6 != 20 {
		t.Fatalf("grid %d×%d is not 16×20 cells of 8×6", GridWidth, GridHeight)
	}
}
