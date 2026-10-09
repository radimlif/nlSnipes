// Package bots plays the core headlessly: QA players for tests, CI and
// balance checks.
package bots

import "github.com/radimlif/nlSnipes/core"

// Bot chooses one player's input for the coming tick.
type Bot interface {
	Input(s *core.State, slot int) core.Input
}

// Random changes to a uniformly random mask every 6 ticks and holds fast a
// quarter of the time. It walks into walls, which is the point.
type Random struct {
	rng  core.Rand
	cur  core.Input
	tick int
}

// NewRandom returns a Random bot with its own seed.
func NewRandom(seed uint32) *Random { return &Random{rng: core.NewRand(seed)} }

// Input implements Bot.
func (r *Random) Input(*core.State, int) core.Input {
	if r.tick%6 == 0 {
		r.cur = core.Input{Mask: uint8(r.rng.Next()), Fast: r.rng.Mask(3) == 0}
	}
	r.tick++
	return r.cur
}

// Result summarises one bot game.
type Result struct {
	Seed   uint32
	Ticks  uint32
	Phase  core.Phase
	Score  int32
	Hash   uint32
	Replay core.Replay
	StepNs []int64 // per-tick Step duration, when timed
}

// Options control a bot game.
type Options struct {
	MaxTicks int
	// Clock, if set, returns nanoseconds and is used to time each Step.
	Clock func() int64
	// Setup, if set, adjusts the new game before the first tick (for
	// stress tests). The replay is then not playable and is left empty.
	Setup func(*core.State)
}

// Play runs a game until it ends or MaxTicks pass; bots[i] drives slot i.
func Play(cfg core.Config, seed uint32, bots []Bot, opt Options) Result {
	rec := core.NewRecorder(cfg, seed)
	if opt.Setup != nil {
		opt.Setup(rec.State)
	}
	s := rec.State
	res := Result{Seed: seed}
	for t := 0; t < opt.MaxTicks && s.Phase == core.PhaseRunning; t++ {
		var in [core.MaxPlayers]core.Input
		for slot, b := range bots {
			in[slot] = b.Input(s, slot)
		}
		var t0 int64
		if opt.Clock != nil {
			t0 = opt.Clock()
		}
		if opt.Setup != nil {
			s.Step(in)
		} else {
			rec.Step(in)
		}
		if opt.Clock != nil {
			res.StepNs = append(res.StepNs, opt.Clock()-t0)
		}
	}
	res.Ticks, res.Phase, res.Score, res.Hash = s.Tick, s.Phase, s.Score, s.Hash()
	if opt.Setup == nil {
		res.Replay = rec.Replay
	}
	return res
}
