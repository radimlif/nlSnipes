package core

import (
	"reflect"
	"testing"
)

func TestStateRoundTripKeepsHash(t *testing.T) {
	for _, skill := range []string{"A1", "M5", "Z9"} {
		cfg, _ := NewConfig(skill, 4, true)
		s := randomGame(cfg, 11, 400, nil)
		want := s.Hash()
		for i := 0; i < 100; i++ {
			b, err := s.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			var n State
			if err := n.UnmarshalBinary(b); err != nil {
				t.Fatalf("%s round %d: %v", skill, i, err)
			}
			if !reflect.DeepEqual(n.Ents, s.Ents) || n.Players != s.Players || n.Cfg != s.Cfg {
				t.Fatalf("%s round %d: state differs after round-trip", skill, i)
			}
			if h := n.Hash(); h != want {
				t.Fatalf("%s round %d: hash %08x, want %08x", skill, i, h, want)
			}
			s = &n
		}
		// A restored state must also play on identically.
		a, b := s.Clone(), s
		for i := 0; i < 200; i++ {
			a.Step([MaxPlayers]Input{{Mask: MoveR | FireU}})
			b.Step([MaxPlayers]Input{{Mask: MoveR | FireU}})
		}
		if a.Hash() != b.Hash() {
			t.Fatalf("%s: clone diverged", skill)
		}
	}
}

func TestUnmarshalRejectsGarbage(t *testing.T) {
	cfg, _ := NewConfig("A1", 1, true)
	good, _ := NewGame(cfg, 1).MarshalBinary()
	for _, b := range [][]byte{nil, {9}, good[:len(good)-1], append(append([]byte{}, good...), 0)} {
		var s State
		if err := s.UnmarshalBinary(b); err == nil {
			t.Errorf("accepted %d bytes", len(b))
		}
	}
}

func FuzzUnmarshalState(f *testing.F) {
	cfg, _ := NewConfig("D3", 2, true)
	good, _ := NewGame(cfg, 1).MarshalBinary()
	f.Add(good)
	f.Fuzz(func(t *testing.T, b []byte) {
		var s State
		if s.UnmarshalBinary(b) == nil {
			s.Step([MaxPlayers]Input{})
		}
	})
}

func TestReplayRoundTrip(t *testing.T) {
	cfg, _ := NewConfig("J4", 2, true)
	rec := NewRecorder(cfg, 3)
	r := NewRand(8)
	for i := 0; i < 500; i++ {
		rec.Step([MaxPlayers]Input{{Mask: uint8(r.Next())}, {Mask: uint8(r.Next()), Fast: true}})
	}
	b, _ := rec.Replay.MarshalBinary()
	var rp Replay
	if err := rp.UnmarshalBinary(b); err != nil {
		t.Fatal(err)
	}
	if _, err := rp.Play(); err != nil {
		t.Fatal(err)
	}
	for i := 10; i < 60; i++ {
		rp.Inputs[i][1] = Input{Mask: MoveL | MoveU}
	}
	if _, err := rp.Play(); err == nil {
		t.Fatal("tampered replay still matched its hash")
	}
}
