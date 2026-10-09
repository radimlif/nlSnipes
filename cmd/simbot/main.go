// Command simbot plays headless bot games against the core for CI and
// balance checks, verifying every game's replay.
//
//	simbot -skill A1 -bot hunter -seeds 100 -ticks 5000 -min-win 90
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/radimlif/nlSnipes/bots"
	"github.com/radimlif/nlSnipes/core"
)

func main() {
	skill := flag.String("skill", "A1", "skill code")
	players := flag.Int("players", 1, "players (1-4), all driven by the same bot type")
	seeds := flag.Int("seeds", 20, "number of games, seeds 0..n-1")
	ticks := flag.Int("ticks", 5000, "tick limit per game")
	kind := flag.String("bot", "hunter", "hunter or random")
	minWin := flag.Int("min-win", -1, "fail unless at least this percentage of games is won")
	dump := flag.String("dump", "", "write the first lost game's replay to this file")
	verbose := flag.Bool("v", false, "print every game")
	flag.Parse()

	cfg, err := core.NewConfig(*skill, *players, true)
	if err != nil {
		fail(err)
	}
	if *kind != "hunter" && *kind != "random" {
		fail(fmt.Errorf("unknown bot %q", *kind))
	}
	wins, winTicks, score := 0, 0, 0
	dumped := false
	for seed := uint32(0); seed < uint32(*seeds); seed++ {
		b := make([]bots.Bot, *players)
		for i := range b {
			if *kind == "hunter" {
				b[i] = bots.NewHunter()
			} else {
				b[i] = bots.NewRandom(seed*31 + uint32(i))
			}
		}
		r := bots.Play(cfg, seed, b, bots.Options{MaxTicks: *ticks})
		if _, err := r.Replay.Play(); err != nil {
			fail(fmt.Errorf("seed %d: %v", seed, err))
		}
		result := map[core.Phase]string{core.PhaseRunning: "timeout", core.PhaseWon: "won", core.PhaseLost: "lost"}[r.Phase]
		if *verbose {
			fmt.Printf("seed %3d  %-7s  %5d ticks  score %5d  hash %08x\n", seed, result, r.Ticks, r.Score, r.Hash)
		}
		if r.Phase == core.PhaseWon {
			wins++
			winTicks += int(r.Ticks)
		} else if *dump != "" && !dumped {
			b, _ := r.Replay.MarshalBinary()
			if err := os.WriteFile(*dump, b, 0o644); err != nil {
				fail(err)
			}
			dumped = true
		}
		score += int(r.Score)
	}
	pct := wins * 100 / max(*seeds, 1)
	fmt.Printf("%s, %d × %s, %d games: won %d (%d%%), mean win %d ticks, mean score %d; all replays match\n",
		cfg.Skill(), *players, *kind, *seeds, wins, pct, winTicks/max(wins, 1), score/max(*seeds, 1))
	if *minWin >= 0 && pct < *minWin {
		fail(fmt.Errorf("win rate %d%% below -min-win %d%%", pct, *minWin))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "simbot:", err)
	os.Exit(1)
}
