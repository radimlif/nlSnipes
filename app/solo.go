package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/term"
)

// TickDuration is one simulation tick (18 per second).
const TickDuration = time.Second / core.TicksPerSecond

// Ticks a finished game's result stays up before the next maze starts.
const resultTicks = 10 * core.TicksPerSecond

// Options configure a solo game.
type Options struct {
	Skill     string           // skill code; empty = ask on the title screen
	Seed      uint32           // 0 = derived from the clock
	ScoreFile string           // "" = no high scores kept
	Classic   bool             // start in the original 40 × 25 view
	Now       func() time.Time // clock; nil = time.Now
}

type screen uint8

const (
	screenTitle screen = iota
	screenPlay
	screenResult
)

// cheatCode toggles mirror shots when typed during play.
const cheatCode = "iddqd"

// Solo is a single-player game in a terminal. Run drives it in real time;
// tests call HandleEvent and Tick directly.
type Solo struct {
	t      term.Terminal
	r      term.Renderer
	keys   *term.KeyState
	opt    Options
	frame  *term.Frame
	screen screen

	skillInput string
	errMsg     string

	game         *core.State
	cam          term.Camera
	classic      bool
	help         bool
	msg          string
	msgColor     uint8
	msgUntil     uint32
	resultT      int
	scores       Scores
	seq          uint32
	typed        string // last few letters typed in play, for the cheat code
	mirrorWanted bool   // IDDQD state; kept across mazes
}

// NewSolo prepares a game on terminal t.
func NewSolo(t term.Terminal, opt Options) *Solo {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	a := &Solo{t: t, opt: opt, keys: term.NewKeyState(t.RealReleases()), skillInput: "A1",
		classic: opt.Classic, frame: term.NewFrame(term.Width, term.Height)}
	if opt.ScoreFile != "" {
		a.scores = LoadScores(opt.ScoreFile)
	} else {
		a.scores = Scores{}
	}
	if opt.Skill != "" {
		a.skillInput = strings.ToUpper(opt.Skill)
		a.start()
	}
	a.compose()
	return a
}

// Game returns the running game, or nil on the title screen.
func (a *Solo) Game() *core.State { return a.game }

// Frame returns the last drawn frame.
func (a *Solo) Frame() *term.Frame { return a.frame }

// Run plays in real time until the player quits or input ends.
func Run(t term.Terminal, opt Options) error {
	a := NewSolo(t, opt)
	tick := time.NewTicker(TickDuration)
	defer tick.Stop()
	if err := a.draw(); err != nil {
		return err
	}
	for {
		select {
		case ev, ok := <-t.Events():
			if !ok || a.HandleEvent(ev) {
				a.saveScore()
				return nil
			}
		case <-tick.C:
			a.Tick()
			if err := a.draw(); err != nil {
				return err
			}
		}
	}
}

// HandleEvent processes one input event; it returns true to quit.
func (a *Solo) HandleEvent(ev term.Event) bool {
	now := a.opt.Now()
	if ev.Kind == term.EvKittySupported {
		a.keys.Real = true
		return false
	}
	if ev.Key == term.KeyCtrlC {
		return true
	}
	if ev.Kind == term.EvRelease || ev.Kind == term.EvRepeat {
		a.keys.Handle(ev, now)
		return false
	}
	switch a.screen {
	case screenTitle:
		return a.titleKey(ev)
	case screenResult:
		switch ev.Key {
		case term.KeyEsc:
			return true
		case term.KeyEnter:
			a.start()
		}
		return false
	}
	switch {
	case ev.Key == term.KeyEsc:
		return true
	case ev.Key == term.KeyF1:
		a.help = !a.help
		return false
	case ev.Key == term.KeyRune && ev.Rune == 'v':
		a.classic = !a.classic
		return false
	}
	if ev.Key == term.KeyRune && ev.Rune >= 'a' && ev.Rune <= 'z' {
		a.typed += string(ev.Rune)
		if len(a.typed) > len(cheatCode) {
			a.typed = a.typed[len(a.typed)-len(cheatCode):]
		}
		if a.typed == cheatCode {
			a.mirrorWanted = !a.mirrorWanted
			a.typed = ""
		}
	}
	a.keys.Handle(ev, now)
	return false
}

func (a *Solo) titleKey(ev term.Event) bool {
	a.errMsg = ""
	switch ev.Key {
	case term.KeyEsc:
		return true
	case term.KeyBackspace:
		if n := len(a.skillInput); n > 0 {
			a.skillInput = a.skillInput[:n-1]
		}
	case term.KeyEnter:
		a.start()
	case term.KeyRune:
		r := ev.Rune
		if r >= 'a' && r <= 'z' {
			a.skillInput = strings.ToUpper(string(r))
		} else if r >= '1' && r <= '9' && len(a.skillInput) == 1 {
			a.skillInput += string(r)
		}
	}
	return false
}

func (a *Solo) start() {
	cfg, err := core.NewConfig(a.skillInput, 1, true)
	if err != nil {
		a.screen, a.errMsg = screenTitle, "Type a letter A-Z, then a digit 1-9"
		return
	}
	seed := a.opt.Seed
	if seed == 0 {
		seed = uint32(a.opt.Now().UnixNano())
	}
	seed += a.seq
	a.seq++
	a.game = core.NewGame(cfg, seed)
	a.screen, a.help, a.msg = screenPlay, false, ""
	a.keys.Reset()
	if i := a.game.PlayerEntity(0); i >= 0 {
		a.cam = term.CameraOn(&a.game.Ents[i])
	}
	a.flash("Shoot the hives! F1 for help", term.White, 3*core.TicksPerSecond)
}

func (a *Solo) flash(msg string, colour uint8, ticks uint32) {
	a.msg, a.msgColor = msg, colour
	if a.game != nil {
		a.msgUntil = a.game.Tick + ticks
	}
}

func rk(r rune) term.KeyID { return term.KeyID{Key: term.KeyRune, Rune: r} }

// bindings map keys to input bits. Arrows, the numpad and Home/PgUp/End/PgDn
// move; W A S D fire straight and Q E Z C fire diagonally.
var bindings = [...]struct {
	id   term.KeyID
	bits uint8
}{
	{term.KeyID{Key: term.KeyRight}, core.MoveR}, {term.KeyID{Key: term.KeyLeft}, core.MoveL},
	{term.KeyID{Key: term.KeyDown}, core.MoveD}, {term.KeyID{Key: term.KeyUp}, core.MoveU},
	{term.KeyID{Key: term.KeyHome}, core.MoveL | core.MoveU}, {term.KeyID{Key: term.KeyPgUp}, core.MoveR | core.MoveU},
	{term.KeyID{Key: term.KeyEnd}, core.MoveL | core.MoveD}, {term.KeyID{Key: term.KeyPgDn}, core.MoveR | core.MoveD},
	{rk('8'), core.MoveU}, {rk('2'), core.MoveD}, {rk('4'), core.MoveL}, {rk('6'), core.MoveR},
	{rk('7'), core.MoveL | core.MoveU}, {rk('9'), core.MoveR | core.MoveU},
	{rk('1'), core.MoveL | core.MoveD}, {rk('3'), core.MoveR | core.MoveD},
	{rk('d'), core.FireR}, {rk('a'), core.FireL}, {rk('s'), core.FireD}, {rk('w'), core.FireU},
	{rk('e'), core.FireR | core.FireU}, {rk('q'), core.FireL | core.FireU},
	{rk('c'), core.FireR | core.FireD}, {rk('z'), core.FireL | core.FireD},
}

const moveBits = core.MoveR | core.MoveL | core.MoveD | core.MoveU

// Input samples the keyboard into one tick's controls.
func (a *Solo) Input() core.Input {
	now := a.opt.Now()
	var in core.Input
	guessing := false
	for _, b := range bindings {
		if a.keys.Active(b.id, now) {
			in.Mask |= b.bits
			if b.bits&moveBits != 0 && a.keys.Guessing(b.id, now) {
				guessing = true
			}
		}
	}
	in.Fast = a.keys.Active(rk(' '), now)
	// Without key releases a tap is indistinguishable from a hold until the
	// first auto-repeat. Never let such a guess walk the player into a wall.
	if guessing && a.touchesWall(in) {
		in.Mask &^= moveBits
	}
	if a.game != nil && a.game.Players[0].Mirror != a.mirrorWanted {
		in.ToggleMirror = true
	}
	return in
}

// touchesWall reports whether moving with in would put the player's body
// against a wall within this tick's steps.
func (a *Solo) touchesWall(in core.Input) bool {
	i := a.game.PlayerEntity(0)
	d, ok := core.MoveDir(in.Mask)
	if i < 0 || !ok {
		return false
	}
	e := a.game.Ents[i]
	dx, dy := core.DirDelta(d)
	steps := int32(1)
	if in.Fast {
		steps = 2
	}
	for k := int32(1); k <= steps; k++ {
		x, y := e.X+dx*k, e.Y+dy*k
		if a.game.Wall(x, y) || a.game.Wall(x+1, y) || a.game.Wall(x, y+1) || a.game.Wall(x+1, y+1) {
			return true
		}
	}
	return false
}

// Tick advances the game by one tick (when playing) and redraws the frame.
func (a *Solo) Tick() {
	switch a.screen {
	case screenPlay:
		if a.help {
			break // F1 pauses
		}
		a.game.Step([core.MaxPlayers]core.Input{a.Input()})
		a.react()
		if i := a.game.PlayerEntity(0); i >= 0 {
			a.cam = term.CameraOn(&a.game.Ents[i])
		}
		if a.game.Phase != core.PhaseRunning {
			a.screen, a.resultT = screenResult, resultTicks
			a.saveScore()
		}
	case screenResult:
		if a.resultT--; a.resultT <= 0 {
			a.start()
		}
	}
	a.compose()
}

// react turns this tick's events into messages and the bell.
func (a *Solo) react() {
	for _, ev := range a.game.Events {
		if ev.Slot != 0 {
			continue
		}
		switch ev.Kind {
		case core.EvHiveDown:
			a.flash(fmt.Sprintf("HIVE DESTROYED! +50   %d to go", a.game.HivesAlive), term.Yellow, 2*core.TicksPerSecond)
		case core.EvPlayerDied:
			a.t.Bell()
			if l := a.game.Players[0].Lives; l > 0 {
				a.flash(fmt.Sprintf("OUCH! %d %s left", l, plural(l, "man", "men")), term.LightRed, 2*core.TicksPerSecond)
			}
		case core.EvMirror:
			if a.game.Players[0].Mirror {
				a.flash("IDDQD! Mirror shots: bank off walls", term.LightMagenta, 3*core.TicksPerSecond)
			} else {
				a.flash("Mirror shots off", term.LightGray, 2*core.TicksPerSecond)
			}
		}
	}
	if a.game.HivesAlive == 0 && a.game.SnipesAlive > 0 && a.game.Tick >= a.msgUntil {
		a.flash(fmt.Sprintf("Hives gone - clear %d %s!", a.game.SnipesAlive, plural(a.game.SnipesAlive, "snipe", "snipes")), term.LightGreen, 1)
	}
}

func plural(n int32, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (a *Solo) saveScore() {
	if a.game == nil || a.opt.ScoreFile == "" {
		return
	}
	skill := a.game.Cfg.Skill()
	if sc := a.game.Players[0].Score; sc > a.scores[skill] {
		a.scores[skill] = sc
		_ = a.scores.Save(a.opt.ScoreFile)
	}
}

func (a *Solo) draw() error {
	return term.Present(a.t, &a.r, a.frame)
}
