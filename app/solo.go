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
	Now       func() time.Time // clock; nil = time.Now
}

type screen uint8

const (
	screenTitle screen = iota
	screenPlay
	screenResult
)

// Solo is a single-player game in a terminal. Run drives it in real time;
// tests call HandleEvent and Tick directly.
type Solo struct {
	t      term.Terminal
	r      term.Renderer
	keys   *term.KeyState
	opt    Options
	frame  term.Frame
	screen screen

	skillInput string
	errMsg     string

	game     *core.State
	cam      term.Camera
	help     bool
	msg      string
	msgColor uint8
	msgUntil uint32
	resultT  int
	scores   Scores
	seq      uint32
}

// NewSolo prepares a game on terminal t.
func NewSolo(t term.Terminal, opt Options) *Solo {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	a := &Solo{t: t, opt: opt, keys: term.NewKeyState(t.RealReleases()), skillInput: "A1"}
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
func (a *Solo) Frame() *term.Frame { return &a.frame }

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
	switch ev.Key {
	case term.KeyEsc:
		return true
	case term.KeyF1:
		a.help = !a.help
		return false
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

var (
	idRight = term.KeyID{Key: term.KeyRight}
	idLeft  = term.KeyID{Key: term.KeyLeft}
	idDown  = term.KeyID{Key: term.KeyDown}
	idUp    = term.KeyID{Key: term.KeyUp}
	idFireR = term.KeyID{Key: term.KeyRune, Rune: 'd'}
	idFireL = term.KeyID{Key: term.KeyRune, Rune: 'a'}
	idFireD = term.KeyID{Key: term.KeyRune, Rune: 's'}
	idFireU = term.KeyID{Key: term.KeyRune, Rune: 'w'}
	idFast  = term.KeyID{Key: term.KeyRune, Rune: ' '}
)

// Input samples the keyboard into one tick's controls.
func (a *Solo) Input() core.Input {
	now := a.opt.Now()
	var in core.Input
	for _, m := range [...]struct {
		id  term.KeyID
		bit uint8
	}{{idRight, core.MoveR}, {idLeft, core.MoveL}, {idDown, core.MoveD}, {idUp, core.MoveU},
		{idFireR, core.FireR}, {idFireL, core.FireL}, {idFireD, core.FireD}, {idFireU, core.FireU}} {
		if a.keys.Active(m.id, now) {
			in.Mask |= m.bit
		}
	}
	in.Fast = a.keys.Active(idFast, now)
	return in
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
		switch ev.Kind {
		case core.EvHiveDown:
			if ev.Slot == 0 {
				a.flash(fmt.Sprintf("HIVE DESTROYED! +50   %d to go", a.game.HivesAlive), term.Yellow, 2*core.TicksPerSecond)
			}
		case core.EvPlayerDied:
			if ev.Slot == 0 {
				a.t.Bell()
				if l := a.game.Players[0].Lives; l > 0 {
					a.flash(fmt.Sprintf("OUCH! %d %s left", l, plural(l, "man", "men")), term.LightRed, 2*core.TicksPerSecond)
				}
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
	return term.Present(a.t, &a.r, &a.frame)
}
