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

// find returns the first frame row containing s, or "".
func find(f *term.Frame, s string) string {
	for y := 0; y < f.H; y++ {
		if l := f.Line(y); strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

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
	if a.Game() != nil || find(a.Frame(), "[ A1 ]") == "" {
		t.Fatal("title should offer A1")
	}
	for _, ev := range []term.Event{{Kind: term.EvPress, Key: term.KeyRune, Rune: 'm'}, {Kind: term.EvPress, Key: term.KeyRune, Rune: '5'}} {
		a.HandleEvent(ev)
	}
	a.Tick()
	if find(a.Frame(), "[ M5 ]") == "" {
		t.Fatal("typed M5, title does not show it")
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
	if find(a.Frame(), "digit") == "" {
		t.Fatal("no hint about the digit")
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
	if find(a.Frame(), "HOW TO PLAY") == "" {
		t.Fatal("legend not drawn")
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
	sim := term.NewSim(130, 120, true) // largest view: 120 × 113
	c := &clock{t: time.Unix(0, 0)}
	a := NewSolo(sim, Options{Skill: "Z9", Seed: 1, Now: c.now})
	a.Game().Players[0].Lives = 1 << 20
	var r term.Renderer
	var ns []time.Duration
	for i := 0; i < 3000; i++ {
		typeMask(a, 0, 0)
		c.t = c.t.Add(TickDuration)
		a.Game().Step([core.MaxPlayers]core.Input{{Mask: uint8(i * 37)}})
		t0 := time.Now()
		a.compose()
		r.Render(a.Frame(), 130, 120)
		ns = append(ns, time.Since(t0))
	}
	slices.Sort(ns)
	p99 := ns[len(ns)*99/100]
	t.Logf("frame: p50 %v, p99 %v, max %v", ns[len(ns)/2], p99, ns[len(ns)-1])
	if p99 >= 2*time.Millisecond {
		t.Fatalf("frame render p99 %v ≥ 2 ms", p99)
	}
}

func typeRunes(a *Solo, s string) {
	for _, r := range s {
		a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRune, Rune: r})
		a.HandleEvent(term.Event{Kind: term.EvRelease, Key: term.KeyRune, Rune: r})
	}
}

func TestIDDQDTogglesMirrorShotsAndSurvivesNewMaze(t *testing.T) {
	a, _, c := newTest(t, "A1", true)
	typeRunes(a, "iddqd")
	c.t = c.t.Add(TickDuration)
	a.Tick()
	if !a.Game().Players[0].Mirror {
		t.Fatal("IDDQD did not turn on mirror shots")
	}
	if find(a.Frame(), "Mirror shots") == "" {
		t.Fatal("no IDDQD message")
	}
	a.start() // next maze
	a.Tick()
	if !a.Game().Players[0].Mirror {
		t.Fatal("mirror shots lost on the next maze")
	}
	typeRunes(a, "xiddqd")
	a.Tick()
	if a.Game().Players[0].Mirror {
		t.Fatal("typing IDDQD again did not turn it off")
	}
}

func TestDiagonalFireKeys(t *testing.T) {
	for r, want := range map[rune]uint8{'q': core.FireL | core.FireU, 'e': core.FireR | core.FireU, 'z': core.FireL | core.FireD, 'c': core.FireR | core.FireD} {
		a, _, _ := newTest(t, "A1", true)
		a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRune, Rune: r})
		if got := a.Input().Mask; got != want {
			t.Errorf("%c: mask %08b, want %08b", r, got, want)
		}
	}
	a, _, _ := newTest(t, "A1", true)
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyPgUp})
	if got := a.Input().Mask; got != core.MoveR|core.MoveU {
		t.Errorf("PgUp: mask %08b", got)
	}
}

// Without key releases, a lone press must not carry the player into an
// electric wall while the game is only guessing that the key is held.
func TestGuessedHoldStopsAtWalls(t *testing.T) {
	a, _, c := newTest(t, "M1", false)
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRight})
	for i := 0; i < 12; i++ { // ~650 ms, past the default repeat delay; no repeats come
		c.t = c.t.Add(TickDuration)
		a.Tick()
	}
	if a.Game().PlayerEntity(0) < 0 {
		t.Fatal("a single tap walked the player into an electric wall")
	}
	e := a.Game().Ents[a.Game().PlayerEntity(0)]
	if e.X == a.Game().Players[0].SpawnX {
		t.Fatal("the press did not move the player at all")
	}
}

func TestVTogglesClassicView(t *testing.T) {
	a, _, _ := newTest(t, "A1", true)
	if a.Frame().W != 80 || a.Frame().H != 30 {
		t.Fatalf("full view is %dx%d, want the terminal's 80x30", a.Frame().W, a.Frame().H)
	}
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRune, Rune: 'v'})
	a.Tick()
	if a.Frame().W != term.Width || a.Frame().H != term.Height {
		t.Fatalf("classic view is %dx%d", a.Frame().W, a.Frame().H)
	}
}

func TestKeyboardLayouts(t *testing.T) {
	press := func(a *Solo, ev term.Event) uint8 {
		ev.Kind = term.EvPress
		a.HandleEvent(ev)
		return a.Input().Mask
	}
	// Czech QWERTZ in a terminal that reports base keys: the bottom-left
	// key types 'y' but sits where US has 'z'.
	a, _, _ := newTest(t, "A1", true)
	if m := press(a, term.Event{Key: term.KeyRune, Rune: 'y', Base: 'z'}); m != core.FireL|core.FireD {
		t.Errorf("QWERTZ bottom-left with base key: %08b", m)
	}
	// Same keyboard, legacy terminal: only 'y' arrives.
	a, _, _ = newTest(t, "A1", false)
	if m := press(a, term.Event{Key: term.KeyRune, Rune: 'y'}); m != core.FireL|core.FireD {
		t.Errorf("QWERTZ bottom-left without base key: %08b", m)
	}
	a, _, _ = newTest(t, "A1", true)
	if m := press(a, term.Event{Key: term.KeyRune, Rune: 'x'}); m != core.FireD {
		t.Errorf("x: %08b", m)
	}
	// Czech number row types ř for 5.
	a, _, _ = newTest(t, "", true)
	for _, r := range []rune{'m', 'ř'} {
		a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRune, Rune: r})
	}
	a.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyEnter})
	if a.Game() == nil || a.Game().Cfg.Skill() != "M5" {
		t.Fatal("Czech number row did not enter M5")
	}
}
