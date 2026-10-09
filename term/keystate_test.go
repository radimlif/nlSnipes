package term

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

var up = KeyID{Key: KeyUp}

func ev(kind EventKind) Event { return Event{Kind: kind, Key: KeyUp} }

func TestRealReleases(t *testing.T) {
	k := NewKeyState(true)
	k.Handle(ev(EvPress), at(0))
	if !k.Held(up, at(5000)) {
		t.Fatal("held key without repeats dropped")
	}
	k.Handle(ev(EvRelease), at(5000))
	if k.Held(up, at(5001)) {
		t.Fatal("released key still held")
	}
}

func TestShortTapStillCountsOnce(t *testing.T) {
	k := NewKeyState(true)
	k.Handle(ev(EvPress), at(0))
	k.Handle(ev(EvRelease), at(10))
	if !k.Active(up, at(30)) {
		t.Fatal("tap between ticks was lost")
	}
	if k.Active(up, at(85)) {
		t.Fatal("tap counted twice")
	}
}

func TestEmulatedHoldDoesNotStutterBeforeRepeats(t *testing.T) {
	k := NewKeyState(false)
	k.Handle(ev(EvPress), at(0))
	for ms := 0; ms <= 500; ms += 55 { // every tick up to the default repeat delay
		if !k.Held(up, at(ms)) {
			t.Fatalf("gap in the hold at %d ms", ms)
		}
	}
	if k.Guessing(up, at(30)) || !k.Guessing(up, at(300)) {
		t.Fatal("a fresh press is certain; later, before repeats, it is a guess")
	}
	k.Handle(ev(EvPress), at(500)) // first repeat confirms the hold
	if k.Guessing(up, at(520)) {
		t.Fatal("still guessing after a repeat")
	}
}

func TestEmulatedTapEndsAfterRepeatDelay(t *testing.T) {
	k := NewKeyState(false)
	k.Handle(ev(EvPress), at(0))
	if k.Held(up, at(700)) {
		t.Fatal("tap still held well after the repeat delay")
	}
}

func TestEmulatedHoldFollowsRepeats(t *testing.T) {
	k := NewKeyState(false)
	k.Handle(ev(EvPress), at(0))
	for ms := 500; ms <= 2000; ms += 33 { // OS repeat: 500 ms delay, 30/s
		k.Handle(ev(EvPress), at(ms))
		if !k.Held(up, at(ms+20)) {
			t.Fatalf("dropped during repeats at %d ms", ms)
		}
	}
	if k.Held(up, at(2000+250)) {
		t.Fatal("still held 250 ms after the last repeat")
	}
}

func TestEmulationLearnsRepeatDelay(t *testing.T) {
	k := NewKeyState(false)
	for round := 0; round < 6; round++ {
		base := round * 5000
		k.Handle(ev(EvPress), at(base))
		for ms := 250; ms < 800; ms += 30 {
			k.Handle(ev(EvPress), at(base+ms))
		}
		k.Held(up, at(base+3000)) // expire
	}
	if k.delay > 300*time.Millisecond {
		t.Fatalf("delay estimate %v, want near 250 ms", k.delay)
	}
	if k.interval > 40*time.Millisecond {
		t.Fatalf("interval estimate %v, want near 30 ms", k.interval)
	}
}

func TestOppositeDirectionReplaces(t *testing.T) {
	k := NewKeyState(false)
	k.Handle(Event{Kind: EvPress, Key: KeyLeft}, at(0))
	k.Handle(Event{Kind: EvPress, Key: KeyRight}, at(50))
	if k.Held(KeyID{Key: KeyLeft}, at(60)) || !k.Held(KeyID{Key: KeyRight}, at(60)) {
		t.Fatal("left should give way to right")
	}
}
