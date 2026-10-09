package core

import "testing"

// Expected tables restated from DESIGN.md §2, independently of config.go.
var (
	wantAccuracy  = "23434434344534343434454455"
	wantSmall     = "00011100111100110011111111"
	wantBounce    = "00000011111100001111111111"
	wantExplosion = []uint8{0x7F, 0x7F, 0x7F, 0x3F, 0x3F, 0x1F, 0x7F, 0x7F, 0x3F, 0x3F, 0x1F, 0x1F, 0x7F, 0x7F, 0x3F, 0x3F, 0x7F, 0x7F, 0x3F, 0x3F, 0x1F, 0x1F, 0x3F, 0x1F, 0x1F, 0x0F}
	wantMax       = []uint16{10, 20, 30, 40, 60, 80, 100, 120, 150}
	wantHives     = []uint8{3, 3, 4, 4, 5, 5, 6, 8, 10}
	wantLives     = []uint8{5, 5, 5, 5, 5, 4, 4, 3, 2}
)

func TestAllSkillCodes(t *testing.T) {
	n := 0
	for l := byte('A'); l <= 'Z'; l++ {
		for d := byte('1'); d <= '9'; d++ {
			code := string([]byte{l, d})
			c, err := NewConfig(code, 1, true)
			if err != nil {
				t.Fatalf("%s: %v", code, err)
			}
			li, di := l-'A', d-'1'
			switch {
			case c.Skill() != code:
				t.Errorf("%s: Skill() = %s", code, c.Skill())
			case c.Accuracy != wantAccuracy[li]-'0':
				t.Errorf("%s: accuracy %d", code, c.Accuracy)
			case c.SmallSnipes != (wantSmall[li] == '1'):
				t.Errorf("%s: smallSnipes %v", code, c.SmallSnipes)
			case c.Bounce != (wantBounce[li] == '1'):
				t.Errorf("%s: bounce %v", code, c.Bounce)
			case c.ExplosionMask != wantExplosion[li]:
				t.Errorf("%s: explosionMask %#x", code, c.ExplosionMask)
			case c.ElectricWalls != (l >= 'M'):
				t.Errorf("%s: electricWalls %v", code, c.ElectricWalls)
			case c.HivesResistSpears != (l >= 'W'):
				t.Errorf("%s: hivesResistSpears %v", code, c.HivesResistSpears)
			case c.MaxSnipes != wantMax[di] || c.Hives != wantHives[di] || c.Lives != wantLives[di]:
				t.Errorf("%s: digit tables %d/%d/%d", code, c.MaxSnipes, c.Hives, c.Lives)
			}
			n++
		}
	}
	if n != 234 {
		t.Fatalf("checked %d codes, want 234", n)
	}
}

func TestBadSkillCodes(t *testing.T) {
	for _, code := range []string{"", "A", "a1", "A0", "1A", "AA", "A10", "[1"} {
		if _, err := NewConfig(code, 1, true); err == nil {
			t.Errorf("%q accepted", code)
		}
	}
	for _, p := range []int{0, 5} {
		if _, err := NewConfig("A1", p, true); err == nil {
			t.Errorf("players=%d accepted", p)
		}
	}
}
