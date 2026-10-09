package lan

import (
	"reflect"
	"testing"

	"github.com/radimlif/nlSnipes/core"
)

func sampleMessages() []any {
	rec := Record{Tick: 77, Hash: 0xDEADBEEF}
	rec.Inputs[0] = core.Input{Mask: core.MoveR | core.FireU, Fast: true}
	rec.Inputs[2] = core.Input{Join: true, ToggleMirror: true}
	rec.Inputs[3] = core.Input{Leave: true}
	return []any{
		Probe{GameID: 1, ClientID: 2},
		Beacon{GameID: 1, Skill: "M5", Tick: 99, Players: 3, Spectators: 2},
		Beacon{GameID: 1},
		Join{GameID: 1, ClientID: 2, Nick: "radim"},
		Welcome{ClientID: 2, Slot: 3},
		Welcome{ClientID: 2, Slot: Spectator},
		Input{ClientID: 2, Frames: []Frame{{Seq: 5, Mask: 3, Fast: true}, {Seq: 6, Mask: 0x81, Mirror: true}}},
		Input{ClientID: 2},
		Snapshot{Epoch: 1, Tick: 36, Hash: 7, Part: 1, Parts: 2, Data: []byte{1, 2, 3}},
		Tick{Epoch: 1, Records: []Record{rec, rec}},
		Bye{ClientID: 2},
		Resync{ClientID: 2},
		Roster{Seats: []Seat{{Slot: 0, Nick: "host"}, {Slot: Spectator, ClientID: 9, Nick: "anna"}}, StartIn: 12},
	}
}

func TestMessagesRoundTrip(t *testing.T) {
	for _, m := range sampleMessages() {
		b := Encode(m)
		if len(b) > MaxDatagram {
			t.Errorf("%T: %d bytes", m, len(b))
		}
		got, err := Decode(b)
		if err != nil {
			t.Fatalf("%T: %v", m, err)
		}
		if !reflect.DeepEqual(got, m) {
			t.Errorf("%T: got %+v, want %+v", m, got, m)
		}
	}
}

func TestDecodeRejectsJunk(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("NLS"), []byte("XXXX\x01"), []byte("NLS1\x63"),
		append(Encode(Bye{ClientID: 1}), 0), Encode(Probe{})[:8]} {
		if _, err := Decode(b); err == nil {
			t.Errorf("accepted %q", b)
		}
	}
}

func TestLongestMessagesFitADatagram(t *testing.T) {
	var recs []Record
	for i := 0; i < MaxTickRecords+5; i++ {
		recs = append(recs, Record{Tick: uint32(i)})
	}
	var seats []Seat
	for i := 0; i < MaxRoster+5; i++ {
		seats = append(seats, Seat{Slot: Spectator, Nick: "abcdefghijklmnop"})
	}
	for _, m := range []any{Tick{Records: recs}, Roster{Seats: seats}, Join{Nick: "a very long nickname indeed"}} {
		b := Encode(m)
		if len(b) > MaxDatagram {
			t.Errorf("%T: %d bytes", m, len(b))
		}
		if _, err := Decode(b); err != nil {
			t.Errorf("%T: capped message does not decode: %v", m, err)
		}
	}
}

func TestSnapshotPartsReassemble(t *testing.T) {
	cfg, _ := core.NewConfig("Z9", 4, true)
	s := core.NewGame(cfg, 3)
	for i := 0; i < 600; i++ {
		s.Step([core.MaxPlayers]core.Input{{Mask: uint8(i)}, {Mask: uint8(i * 3)}})
	}
	parts := snapshotParts(4, s)
	var a assembler
	for i := len(parts) - 1; i >= 0; i-- { // any order
		got, ok := a.add(parts[i])
		if ok != (i == 0) {
			t.Fatalf("part %d: complete=%v", i, ok)
		}
		if ok && got.Hash() != s.Hash() {
			t.Fatal("reassembled state differs")
		}
	}
	t.Logf("Z9 state after 600 ticks: %d part(s)", len(parts))
}

func FuzzDecode(f *testing.F) {
	for _, m := range sampleMessages() {
		f.Add(Encode(m))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := Decode(b)
		if err != nil {
			return
		}
		again, err := Decode(Encode(m))
		if err != nil || !reflect.DeepEqual(again, m) {
			t.Fatalf("decoded message does not survive re-encoding: %+v", m)
		}
	})
}

func FuzzSnapshotData(f *testing.F) {
	cfg, _ := core.NewConfig("A1", 1, true)
	f.Add(snapshotParts(1, core.NewGame(cfg, 1))[0].Data)
	f.Fuzz(func(t *testing.T, data []byte) {
		var a assembler
		if s, ok := a.add(Snapshot{Epoch: 1, Parts: 1, Data: data}); ok {
			s.Step([core.MaxPlayers]core.Input{})
		}
	})
}

func TestMultiPartSnapshot(t *testing.T) {
	cfg, _ := core.NewConfig("Z9", 4, true)
	s := core.NewGame(cfg, 3)
	r := core.NewRand(5)
	for i := 0; i < 1500; i++ { // incompressible: random positions and timers
		s.Ents = append(s.Ents, core.Entity{ID: 5000 + uint32(i), Kind: core.KindDebris,
			X: int32(r.Int(core.GridWidth)), Y: int32(r.Int(core.GridHeight)), Owner: -1, Timer: int32(r.Next())})
	}
	parts := snapshotParts(1, s)
	if len(parts) < 3 {
		t.Fatalf("only %d parts; the test needs a multi-part snapshot", len(parts))
	}
	var a assembler
	order := []int{2, 0}
	for i := range parts {
		if i != 0 && i != 2 {
			order = append(order, i)
		}
	}
	for k, i := range order {
		got, ok := a.add(parts[i])
		if ok != (k == len(order)-1) {
			t.Fatalf("after %d parts complete=%v", k+1, ok)
		}
		if ok && got.Hash() != s.Hash() {
			t.Fatal("reassembled state differs")
		}
	}
	// A duplicate part must not complete a snapshot twice or corrupt it.
	if _, ok := a.add(parts[0]); ok {
		t.Fatal("a lone duplicate completed a snapshot")
	}
}
