package core

import (
	"errors"
	"fmt"
)

// Replay is a seed, a config and every tick's inputs: enough to rebuild a
// game exactly. The core records them for debug dumps and tests.
type Replay struct {
	Seed      uint32
	Cfg       Config
	Inputs    [][MaxPlayers]Input
	FinalTick uint32
	FinalHash uint32
}

// Recorder plays a game while recording a replay of it.
type Recorder struct {
	State  *State
	Replay Replay
}

// NewRecorder starts a new recorded game.
func NewRecorder(cfg Config, seed uint32) *Recorder {
	return &Recorder{State: NewGame(cfg, seed), Replay: Replay{Seed: seed, Cfg: cfg}}
}

// Step records the inputs and advances the game.
func (r *Recorder) Step(in [MaxPlayers]Input) {
	r.Replay.Inputs = append(r.Replay.Inputs, in)
	r.State.Step(in)
	r.Replay.FinalTick = r.State.Tick
	r.Replay.FinalHash = r.State.Hash()
}

// Play re-runs a replay and fails if the final hash differs, which means
// the simulation is not deterministic.
func (rp *Replay) Play() (*State, error) {
	s := NewGame(rp.Cfg, rp.Seed)
	for _, in := range rp.Inputs {
		s.Step(in)
	}
	if s.Tick != rp.FinalTick || s.Hash() != rp.FinalHash {
		return s, fmt.Errorf("replay diverged: tick %d hash %08x, recorded tick %d hash %08x",
			s.Tick, s.Hash(), rp.FinalTick, rp.FinalHash)
	}
	return s, nil
}

// MarshalBinary encodes the replay for a debug dump file.
func (rp *Replay) MarshalBinary() ([]byte, error) {
	b := enc{codecVersion}
	b.u32(rp.Seed)
	b.u8(rp.Cfg.Letter)
	b.u8(rp.Cfg.Digit)
	b.u8(rp.Cfg.Players)
	b.bool(rp.Cfg.FriendlyFire)
	b.u32(rp.FinalTick)
	b.u32(rp.FinalHash)
	b.u32(uint32(len(rp.Inputs)))
	for _, in := range rp.Inputs {
		for _, p := range in {
			b.u8(p.Mask)
			b.bool(p.Fast)
		}
	}
	return b, nil
}

// UnmarshalBinary decodes a replay written by MarshalBinary.
func (rp *Replay) UnmarshalBinary(data []byte) error {
	d := &dec{b: data}
	if d.u8() != codecVersion {
		return errors.New("core: unknown replay version")
	}
	var r Replay
	r.Seed = d.u32()
	letter, digit, players, ff := d.u8(), d.u8(), d.u8(), d.bool()
	if d.err != nil {
		return d.err
	}
	if letter > 25 || digit < 1 || digit > 9 || players < 1 || players > MaxPlayers {
		return errors.New("core: invalid config in replay")
	}
	r.Cfg = configFor(letter, digit, players, ff)
	r.FinalTick, r.FinalHash = d.u32(), d.u32()
	n := d.u32()
	if d.err != nil {
		return d.err
	}
	if uint64(n)*2*MaxPlayers != uint64(len(d.b)) {
		return errors.New("core: replay input length mismatch")
	}
	r.Inputs = make([][MaxPlayers]Input, n)
	for i := range r.Inputs {
		for p := range r.Inputs[i] {
			r.Inputs[i][p] = Input{Mask: d.b[0], Fast: d.b[1] != 0}
			d.b = d.b[2:]
		}
	}
	*rp = r
	return nil
}
