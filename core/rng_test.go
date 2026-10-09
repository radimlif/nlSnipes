package core

import "testing"

func TestRandSequenceIsStable(t *testing.T) {
	r := NewRand(1)
	got := [4]uint32{r.Next(), r.Next(), r.Next(), r.Next()}
	r2 := NewRand(1)
	for i, want := range got {
		if v := r2.Next(); v != want {
			t.Fatalf("draw %d: %08x then %08x", i, want, v)
		}
	}
	r1, r2b := NewRand(1), NewRand(2)
	if a, b := r1.Next(), r2b.Next(); a == b {
		t.Fatalf("seeds 1 and 2 give the same first draw %08x", a)
	}
}

func TestRandIntBoundsAndSpread(t *testing.T) {
	r := NewRand(99)
	for _, n := range []uint32{1, 2, 3, 7, 8, 320, 1 << 31} {
		for i := 0; i < 2000; i++ {
			if v := r.Int(n); v >= n {
				t.Fatalf("Int(%d) = %d", n, v)
			}
		}
	}
	if r.Int(0) != 0 {
		t.Fatal("Int(0) != 0")
	}
	var counts [8]int
	for i := 0; i < 80000; i++ {
		counts[r.Int(8)]++
	}
	for d, c := range counts {
		if c < 9000 || c > 11000 {
			t.Errorf("Int(8) bucket %d has %d of 80000", d, c)
		}
	}
}
