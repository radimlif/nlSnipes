package core

// Rand is the sfc32 generator. Its whole state lives in the game state, so a
// game is reproducible from its seed and serialises with it.
type Rand struct{ A, B, C, D uint32 }

// NewRand seeds a generator from a 32-bit seed.
func NewRand(seed uint32) Rand {
	r := Rand{0x9E3779B9, 0x243F6A88, 0xB7E15162, seed}
	for i := 0; i < 15; i++ {
		r.Next()
	}
	return r
}

// Next returns the next 32 random bits.
func (r *Rand) Next() uint32 {
	t := r.A + r.B + r.D
	r.D++
	r.A = r.B ^ (r.B >> 9)
	r.B = r.C + (r.C << 3)
	r.C = (r.C<<21 | r.C>>11) + t
	return t
}

// Int returns a uniform value in [0, n); 0 when n is 0.
func (r *Rand) Int(n uint32) uint32 {
	if n == 0 {
		return 0
	}
	threshold := -n % n
	for {
		if v := r.Next(); v >= threshold {
			return v % n
		}
	}
}

// Mask returns Next() & m, the original's way of drawing odds of 1 in m+1.
func (r *Rand) Mask(m uint32) uint32 { return r.Next() & m }
