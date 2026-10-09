package bots

// Acceptance gates for milestone L1 (docs/DESIGN-LIGHT.md, Milestones).
// With -short they run a reduced set; CI's sim job runs them in full.

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/radimlif/nlSnipes/core"
)

func hunters(n int) []Bot {
	b := make([]Bot, n)
	for i := range b {
		b[i] = NewHunter()
	}
	return b
}

func randoms(n int, seed uint32) []Bot {
	b := make([]Bot, n)
	for i := range b {
		b[i] = NewRandom(seed*31 + uint32(i))
	}
	return b
}

// Gate: 10 000-tick bot games at A1/M5/Z9 with 1 and 4 players replay to
// the same hash. A game that ends early is followed by the next seed, as
// Light starts a new maze, until the configuration has played its ticks;
// every game's replay must reproduce its final hash.
func TestGateReplaysMatch(t *testing.T) {
	ticks := 10000
	if testing.Short() {
		ticks = 1500
	}
	for _, skill := range []string{"A1", "M5", "Z9"} {
		for _, players := range []int{1, 4} {
			for _, kind := range []string{"hunter", "random"} {
				t.Run(fmt.Sprintf("%s/p%d/%s", skill, players, kind), func(t *testing.T) {
					t.Parallel()
					cfg, _ := core.NewConfig(skill, players, true)
					played, games := 0, 0
					for seed := uint32(1); played < ticks; seed++ {
						bots := hunters(players)
						if kind == "random" {
							bots = randoms(players, seed)
						}
						r := Play(cfg, seed, bots, Options{MaxTicks: ticks - played})
						b, _ := r.Replay.MarshalBinary()
						var rp core.Replay
						if err := rp.UnmarshalBinary(b); err != nil {
							t.Fatalf("seed %d: %v", seed, err)
						}
						if _, err := rp.Play(); err != nil {
							t.Fatalf("seed %d: %v", seed, err)
						}
						played += int(r.Ticks)
						games++
					}
					t.Logf("%d ticks over %d games, all replays match", played, games)
				})
			}
		}
	}
}

// Gate: HunterBot wins A1 in ≤ 5 000 ticks on ≥ 90 % of 100 seeds.
func TestGateHunterWinsA1(t *testing.T) {
	seeds := 100
	if testing.Short() {
		seeds = 20
	}
	cfg, _ := core.NewConfig("A1", 1, true)
	results := make([]Result, seeds)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = Play(cfg, uint32(i), []Bot{NewHunter()}, Options{MaxTicks: 5000})
		}(i)
	}
	wg.Wait()
	wins, total := 0, 0
	for _, r := range results {
		if r.Phase == core.PhaseWon {
			wins++
			total += int(r.Ticks)
		} else {
			t.Logf("seed %d: not won (phase %d after %d ticks)", r.Seed, r.Phase, r.Ticks)
		}
	}
	t.Logf("won %d/%d, mean %d ticks", wins, seeds, total/max(wins, 1))
	if wins*10 < seeds*9 {
		t.Fatalf("HunterBot won %d of %d A1 games, want ≥ 90 %%", wins, seeds)
	}
}

// loadMeter wraps a bot and records the most snipes it ever saw alive.
type loadMeter struct {
	Bot
	peak *int32
}

func (m loadMeter) Input(s *core.State, slot int) core.Input {
	*m.peak = max(*m.peak, s.SnipesAlive)
	return m.Bot.Input(s, slot)
}

// Gate: Step p99 < 1 ms at Z9 with 4 bots. Random bots with unlimited lives
// rarely kill snipes, so the maze fills to its 150-snipe limit and stays
// there: the worst case for Step.
func TestGateStepP99(t *testing.T) {
	if raceEnabled {
		t.Skip("timing is meaningless under the race detector")
	}
	cfg, _ := core.NewConfig("Z9", 4, true)
	start := time.Now()
	opt := Options{
		MaxTicks: 10000,
		Clock:    func() int64 { return int64(time.Since(start)) },
		Setup: func(s *core.State) {
			for i := range s.Players {
				s.Players[i].Lives = 1 << 20
			}
		},
	}
	var ns []int64
	var peak int32
	for seed := uint32(0); seed < 4; seed++ {
		bots := randoms(4, seed)
		bots[0] = loadMeter{bots[0], &peak}
		ns = append(ns, Play(cfg, seed, bots, opt).StepNs...)
	}
	slices.Sort(ns)
	p99 := time.Duration(ns[len(ns)*99/100])
	t.Logf("%d steps, peak %d snipes: p50 %v, p99 %v, max %v",
		len(ns), peak, time.Duration(ns[len(ns)/2]), p99, time.Duration(ns[len(ns)-1]))
	if peak < int32(cfg.MaxSnipes) {
		t.Fatalf("load too light: peak %d snipes, want %d", peak, cfg.MaxSnipes)
	}
	if p99 >= time.Millisecond {
		t.Fatalf("Step p99 %v ≥ 1 ms", p99)
	}
}
