package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radimlif/nlSnipes/bots"
	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/term"
)

// clock is a fake time source advanced by the test.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTest(t *testing.T, skill string, releases bool) (*Solo, *term.Sim, *clock) {
	t.Helper()
	sim := term.NewSim(80, 30, releases)
	c := &clock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	return NewSolo(sim, Options{Skill: skill, Seed: 1, Now: c.now}), sim, c
}

// keyFor maps a core input bit to the key that produces it.
var keyFor = map[uint8]term.Event{
	core.MoveR: {Key: term.KeyRight}, core.MoveL: {Key: term.KeyLeft},
	core.MoveD: {Key: term.KeyDown}, core.MoveU: {Key: term.KeyUp},
	core.FireR: {Key: term.KeyRune, Rune: 'd'}, core.FireL: {Key: term.KeyRune, Rune: 'a'},
	core.FireD: {Key: term.KeyRune, Rune: 's'}, core.FireU: {Key: term.KeyRune, Rune: 'w'},
}

// typeMask presses and releases keys so the held set matches mask.
func typeMask(a *Solo, prev, mask uint8) {
	for bit := uint8(1); bit != 0; bit <<= 1 {
		ev := keyFor[bit]
		switch {
		case mask&bit != 0 && prev&bit == 0:
			ev.Kind = term.EvPress
			a.HandleEvent(ev)
		case mask&bit == 0 && prev&bit != 0:
			ev.Kind = term.EvRelease
			a.HandleEvent(ev)
		}
	}
}

// Gate L2: a scripted-input test drives a hive kill (score 50) headlessly
// through a simulated terminal. HunterBot decides; its decisions reach the
// game only as key presses and releases, through the same path as a human.
func TestGateScriptedHiveKill(t *testing.T) {
	a, sim, c := newTest(t, "A1", true)
	hunter := bots.NewHunter()
	var held uint8
	var r term.Renderer
	for tick := 0; tick < 3000; tick++ {
		want := hunter.Input(a.Game(), 0).Mask
		typeMask(a, held, want)
		held = want
		c.t = c.t.Add(TickDuration)
		a.Tick()
		if err := term.Present(sim, &r, a.Frame()); err != nil {
			t.Fatal(err)
		}
		if a.Game().HivesAlive < int32(a.Game().Cfg.Hives) {
			f := a.Frame()
			score := a.Game().Players[0].Score
			if score < 50 {
				t.Fatalf("hive down but score %d", score)
			}
			if hud := f.Line(1); !strings.Contains(hud, "Score "+pad5(score)) {
				t.Fatalf("HUD does not show the score: %q", hud)
			}
			if msg := f.Line(2); !strings.Contains(msg, "HIVE DESTROYED") {
				t.Fatalf("no hive message: %q", msg)
			}
			if !strings.Contains(string(sim.Output), "HIVE DESTROYED") {
				t.Fatal("hive message never reached the terminal output")
			}
			t.Logf("hive destroyed at tick %d, score %d", a.Game().Tick, score)
			return
		}
	}
	t.Fatal("no hive destroyed in 3000 ticks")
}

func pad5(n int32) string {
	s := "00000" + itoa(n)
	return s[len(s)-5:]
}

func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// Same drive without key releases: auto-repeat emulation must still let a
// hunter destroy a hive.
func TestEmulatedKeysStillPlay(t *testing.T) {
	a, _, c := newTest(t, "A1", false)
	hunter := bots.NewHunter()
	for tick := 0; tick < 4000 && a.Game().HivesAlive == int32(a.Game().Cfg.Hives); tick++ {
		want := hunter.Input(a.Game(), 0).Mask
		for bit := uint8(1); bit != 0; bit <<= 1 {
			if want&bit != 0 { // terminal auto-repeat keeps sending presses
				ev := keyFor[bit]
				ev.Kind = term.EvPress
				a.HandleEvent(ev)
			}
		}
		c.t = c.t.Add(TickDuration)
		a.Tick()
	}
	if a.Game().HivesAlive == int32(a.Game().Cfg.Hives) {
		t.Fatal("no hive destroyed with emulated key holds")
	}
}

func TestTitleSkillPrompt(t *testing.T) {
	a, _, _ := newTest(t, "", true)
	if a.Game() != nil || !strings.Contains(a.Frame().Line(12), "A1") {
		t.Fatalf("title should offer A1: %q", a.Frame().Line(12))
	}
	for _, ev := range []term.Event{{Kind: term.EvPress, Key: term.KeyRune, Rune: 'm'}, {Kind: term.EvPress, Key: term.KeyRune, Rune: '5'}} {
		a.HandleEvent(ev)
	}
	a.Tick()
	if !strings.Contains(a.Frame().Line(12), "M5") {
		t.Fatalf("typed M5, title shows %q", a.Frame().Line(12))
	}
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyEnter})
	if a.Game() == nil || a.Game().Cfg.Skill() != "M5" {
		t.Fatal("Enter did not start an M5 game")
	}
	if !a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyEsc}) {
		t.Fatal("Esc did not quit")
	}
}

func TestBadSkillStaysOnTitle(t *testing.T) {
	a, _, _ := newTest(t, "", true)
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRune, Rune: 'q'})
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyEnter})
	if a.Game() != nil {
		t.Fatal("started without a digit")
	}
	a.Tick()
	if !strings.Contains(a.Frame().Line(13), "digit") {
		t.Fatalf("no hint: %q", a.Frame().Line(13))
	}
}

func TestF1PausesAndShowsLegend(t *testing.T) {
	a, _, c := newTest(t, "A1", true)
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyF1})
	tick := a.Game().Tick
	for i := 0; i < 10; i++ {
		c.t = c.t.Add(TickDuration)
		a.Tick()
	}
	if a.Game().Tick != tick {
		t.Fatal("game ran while the legend was open")
	}
	if !strings.Contains(a.Frame().Line(5), "HOW TO PLAY") {
		t.Fatalf("legend not drawn: %q", a.Frame().Line(5))
	}
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyF1})
	a.Tick()
	if a.Game().Tick != tick+1 {
		t.Fatal("game did not resume")
	}
}

func TestDeathRingsBellAndHighScoreSaved(t *testing.T) {
	dir := t.TempDir()
	sim := term.NewSim(80, 30, true)
	c := &clock{t: time.Unix(0, 0)}
	a := NewSolo(sim, Options{Skill: "Z9", Seed: 3, ScoreFile: dir + "/s.json", Now: c.now})
	// Z9 has electric walls: walk right until something kills us.
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRight})
	for i := 0; i < 400 && sim.Bells == 0; i++ {
		a.Tick()
	}
	if sim.Bells == 0 {
		t.Fatal("no bell on death")
	}
	a.Game().Players[0].Score = 77
	a.saveScore()
	if got := LoadScores(dir + "/s.json")["Z9"]; got != 77 {
		t.Fatalf("saved best %d, want 77", got)
	}
}

// Gate L2: frame render < 2 ms (compose + ANSI encode), measured on a busy
// Z9 maze with the camera moving every tick.
func TestGateFrameRender(t *testing.T) {
	if raceEnabled {
		t.Skip("timing is meaningless under the race detector")
	}
	a, _, c := newTest(t, "Z9", true)
	a.Game().Players[0].Lives = 1 << 20
	var r term.Renderer
	var ns []time.Duration
	for i := 0; i < 3000; i++ {
		typeMask(a, 0, 0)
		c.t = c.t.Add(TickDuration)
		a.Game().Step([core.MaxPlayers]core.Input{{Mask: uint8(i * 37)}})
		t0 := time.Now()
		a.compose()
		r.Render(a.Frame(), 120, 40)
		ns = append(ns, time.Since(t0))
	}
	slices.Sort(ns)
	p99 := ns[len(ns)*99/100]
	t.Logf("frame: p50 %v, p99 %v, max %v", ns[len(ns)/2], p99, ns[len(ns)-1])
	if p99 >= 2*time.Millisecond {
		t.Fatalf("frame render p99 %v ≥ 2 ms", p99)
	}
}
