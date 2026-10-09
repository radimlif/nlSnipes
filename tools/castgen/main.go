// Command castgen records a bot playing the real game, through the real
// renderer, as an asciinema v2 cast — the source of the README GIF:
//
//	go run ./tools/castgen -out docs/demo.cast
//	agg --theme <CGA palette> docs/demo.cast docs/demo.gif   (see docs/README-GIF.md)
//
// It is deterministic: the same flags give the same recording.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/radimlif/nlSnipes/app"
	"github.com/radimlif/nlSnipes/bots"
	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/term"
)

// keyFor maps a core input bit to the key that produces it.
var keyFor = map[uint8]term.Event{
	core.MoveR: {Key: term.KeyRight}, core.MoveL: {Key: term.KeyLeft},
	core.MoveD: {Key: term.KeyDown}, core.MoveU: {Key: term.KeyUp},
	core.FireR: {Key: term.KeyRune, Rune: 'd', Base: 'd'}, core.FireL: {Key: term.KeyRune, Rune: 'a', Base: 'a'},
	core.FireD: {Key: term.KeyRune, Rune: 's', Base: 's'}, core.FireU: {Key: term.KeyRune, Rune: 'w', Base: 'w'},
}

func main() {
	skill := flag.String("skill", "G4", "skill code typed on the title screen")
	seed := flag.Uint("seed", 3, "maze seed")
	seconds := flag.Int("seconds", 30, "seconds of play to record")
	iddqd := flag.Int("iddqd", 12, "second at which the bot types IDDQD (0 = never)")
	w := flag.Int("w", 80, "terminal width")
	h := flag.Int("h", 30, "terminal height")
	out := flag.String("out", "docs/demo.cast", "output cast file")
	poster := flag.Int("poster", -1, "write only one full frame, at this second of play (a still image)")
	flag.Parse()

	clock := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	sim := term.NewSim(*w, *h, true)
	g := app.NewOffline(sim, app.Options{Seed: uint32(*seed), Now: now, FriendlyFire: true, Nick: "bot"})

	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.Encode(map[string]any{"version": 2, "width": *w, "height": *h,
		"env": map[string]string{"TERM": "xterm-256color"}})

	var r term.Renderer
	elapsed := 0.0
	playTick := -1 // ticks of play so far; -1 on the title screen
	frame := func() {
		if *poster >= 0 {
			if playTick == *poster*core.TicksPerSecond {
				r.Invalidate()
				enc.Encode([]any{0.0, "o", string(r.Render(g.Frame(), *w, *h))})
			}
		} else if b := r.Render(g.Frame(), *w, *h); len(b) > 0 {
			enc.Encode([]any{elapsed, "o", string(b)})
		}
		clock = clock.Add(app.TickDuration)
		elapsed += app.TickDuration.Seconds()
	}
	press := func(ev term.Event) {
		ev.Kind = term.EvPress
		g.HandleEvent(ev)
		ev.Kind = term.EvRelease
		g.HandleEvent(ev)
	}

	// Title screen: a moment to read it, then type the skill code.
	for i := 0; i < 2*core.TicksPerSecond; i++ {
		g.Tick()
		frame()
	}
	for _, c := range *skill {
		r := c
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		press(term.Event{Key: term.KeyRune, Rune: r, Base: r})
		for i := 0; i < core.TicksPerSecond/2; i++ {
			g.Tick()
			frame()
		}
	}
	press(term.Event{Key: term.KeyEnter})

	hunter := bots.NewHunter()
	var held uint8
	for tick := 0; tick < *seconds*core.TicksPerSecond; tick++ {
		if *iddqd > 0 && tick == *iddqd*core.TicksPerSecond {
			for _, c := range "iddqd" {
				press(term.Event{Key: term.KeyRune, Rune: c, Base: c})
			}
		}
		var want uint8
		if s := g.State(); s != nil && s.Phase == core.PhaseRunning {
			want = hunter.Input(s, 0).Mask
		}
		for bit := uint8(1); bit != 0; bit <<= 1 {
			ev := keyFor[bit]
			switch {
			case want&bit != 0 && held&bit == 0:
				ev.Kind = term.EvPress
				g.HandleEvent(ev)
			case want&bit == 0 && held&bit != 0:
				ev.Kind = term.EvRelease
				g.HandleEvent(ev)
			}
		}
		held = want
		playTick = tick
		g.Tick()
		frame()
	}
	fmt.Fprintf(os.Stderr, "castgen: %.1f s recorded to %s\n", elapsed, *out)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "castgen:", err)
	os.Exit(1)
}
