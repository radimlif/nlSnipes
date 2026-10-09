package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/lan"
	"github.com/radimlif/nlSnipes/term"
)

// TickDuration is one simulation tick (18 per second).
const TickDuration = time.Second / core.TicksPerSecond

// Ticks a finished maze's result stays up before the next one starts.
const resultTicks = 10 * core.TicksPerSecond

// Options configure the game.
type Options struct {
	Skill     string           // skill code; empty = ask on the title screen (host)
	Seed      uint32           // 0 = derived from the clock
	ScoreFile string           // "" = no high scores kept
	Classic   bool             // start in the original 40 × 25 view
	Now       func() time.Time // clock; nil = time.Now

	Nick         string // shown to other players
	GameName     string // --game: separate games on one subnet
	HostAddr     string // --host: skip discovery, join this address
	FriendlyFire bool   // host: players' bullets hit each other
	AllowMirror  bool   // host: IDDQD for everyone, not only while alone
	Offline      bool   // no network at all (tests)
	Listen       string // host socket address; "" = first free LAN port (tests use 127.0.0.1:0)
}

type screen uint8

const (
	screenTitle   screen = iota // host choosing the skill
	screenWaiting               // client waiting for the host's game
	screenPlay
	screenResult
	screenHostLeft
)

// lobbyTicks is how long a host may sit on the title screen while others
// wait before the game starts by itself; typing there restarts it.
const lobbyTicks = 20 * core.TicksPerSecond

// cheatCode toggles mirror shots when typed during play.
const cheatCode = "iddqd"

// Game is the terminal game: a host (you run the game, others may join), a
// client (you joined someone's game) or offline. Run drives it in real
// time; tests call HandleEvent and Tick directly.
type Game struct {
	t      term.Terminal
	r      term.Renderer
	keys   *term.KeyState
	opt    Options
	frame  *term.Frame
	screen screen

	host   *lan.Host
	client *lan.Client
	local  *core.State // offline game

	skillInput string
	errMsg     string

	cam          term.Camera
	watch        int8 // spectator camera: slot being followed
	classic      bool
	help         bool
	msg          string
	msgColor     uint8
	msgUntil     uint32
	resultT      int
	scores       Scores
	seq          uint32
	typed        string
	mirrorWanted bool
	epochSeen    uint32
	lobbyT       int // host title countdown while others wait
}

func newGame(t term.Terminal, opt Options) *Game {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	g := &Game{t: t, opt: opt, keys: term.NewKeyState(t.RealReleases()), skillInput: "A1",
		classic: opt.Classic, frame: term.NewFrame(term.Width, term.Height), lobbyT: lobbyTicks}
	g.scores = Scores{}
	if opt.ScoreFile != "" {
		g.scores = LoadScores(opt.ScoreFile)
	}
	if opt.Skill != "" {
		g.skillInput = strings.ToUpper(opt.Skill)
	}
	return g
}

// NewOffline starts a game without any network.
func NewOffline(t term.Terminal, opt Options) *Game {
	opt.Offline = true
	g := newGame(t, opt)
	if opt.Skill != "" {
		g.start()
	}
	g.compose()
	return g
}

// NewHost starts hosting: others on the LAN can join.
func NewHost(t term.Terminal, opt Options) (*Game, error) {
	g := newGame(t, opt)
	h, err := lan.NewHost(lan.HostConfig{Listen: opt.Listen, GameID: lan.GameID(opt.GameName), Nick: opt.Nick,
		FriendlyFire: opt.FriendlyFire, AllowMirror: opt.AllowMirror, Now: opt.Now})
	if err != nil {
		return nil, err
	}
	g.host = h
	if opt.Skill != "" {
		g.start()
	}
	g.compose()
	return g, nil
}

// NewClient joins the host found at addr.
func NewClient(t term.Terminal, opt Options, addr lan.Found, clientID uint32) (*Game, error) {
	g := newGame(t, opt)
	c, err := lan.NewClient(lan.ClientConfig{Host: addr.Addr, GameID: lan.GameID(opt.GameName),
		ClientID: clientID, Nick: opt.Nick, Listen: opt.Listen, Now: opt.Now})
	if err != nil {
		return nil, err
	}
	g.client = c
	g.screen = screenWaiting
	g.compose()
	return g, nil
}

// State is the game being shown, or nil.
func (g *Game) State() *core.State {
	switch {
	case g.host != nil:
		return g.host.State()
	case g.client != nil:
		return g.client.State()
	}
	return g.local
}

// Slot is the local player's slot, or lan.Spectator.
func (g *Game) Slot() int8 {
	if g.client != nil {
		return g.client.Slot()
	}
	return 0
}

// Frame returns the last drawn frame.
func (g *Game) Frame() *term.Frame { return g.frame }

// Close leaves the game: tells the host or the clients.
func (g *Game) Close() {
	g.saveScore()
	if g.host != nil {
		g.host.Close()
	}
	if g.client != nil {
		g.client.Close()
	}
}

// Run plays in real time until the player quits or input ends.
func Run(t term.Terminal, g *Game) error {
	defer g.Close()
	tick := time.NewTicker(TickDuration)
	defer tick.Stop()
	if err := g.draw(); err != nil {
		return err
	}
	for {
		select {
		case ev, ok := <-t.Events():
			if !ok || g.HandleEvent(ev) {
				return nil
			}
		case <-tick.C:
			g.Tick()
			if err := g.draw(); err != nil {
				return err
			}
		}
	}
}

// HandleEvent processes one input event; it returns true to quit.
func (g *Game) HandleEvent(ev term.Event) bool {
	now := g.opt.Now()
	if ev.Kind == term.EvKittySupported {
		g.keys.Real = true
		return false
	}
	if ev.Key == term.KeyCtrlC {
		return true
	}
	if ev.Kind == term.EvRelease || ev.Kind == term.EvRepeat {
		g.keys.Handle(physical(ev), now)
		return false
	}
	switch g.screen {
	case screenTitle:
		return g.titleKey(ev)
	case screenWaiting, screenHostLeft:
		return ev.Key == term.KeyEsc
	case screenResult:
		switch ev.Key {
		case term.KeyEsc:
			return true
		case term.KeyEnter:
			if g.client == nil {
				g.start()
			}
		}
		return false
	}
	switch {
	case ev.Key == term.KeyEsc:
		return true
	case ev.Key == term.KeyF1:
		g.help = !g.help // pauses only an offline game; a LAN game goes on
		return false
	case ev.Key == term.KeyTab:
		g.nextWatch()
		return false
	case ev.Key == term.KeyRune && physical(ev).Rune == 'v':
		g.classic = !g.classic
		return false
	}
	if ev.Key == term.KeyRune && ev.Rune >= 'a' && ev.Rune <= 'z' {
		g.typed += string(ev.Rune)
		if len(g.typed) > len(cheatCode) {
			g.typed = g.typed[len(g.typed)-len(cheatCode):]
		}
		if g.typed == cheatCode {
			g.mirrorWanted = !g.mirrorWanted
			g.typed = ""
		}
	}
	g.keys.Handle(physical(ev), now)
	return false
}

// layoutFallback maps characters from common non-US layouts to the US key
// in the same position, for terminals that report only the typed character:
// QWERTZ's bottom-left Y, and the Czech number row.
var layoutFallback = map[rune]rune{
	'y': 'z',
	'+': '1', 'ě': '2', 'š': '3', 'č': '4', 'ř': '5', 'ž': '6', 'ý': '7', 'á': '8', 'í': '9', 'é': '0',
}

// physical rewrites a key event to the key position the bindings use: the
// US-layout base key when the terminal reports it, else the typed character
// with layout fallbacks.
func physical(ev term.Event) term.Event {
	if ev.Key != term.KeyRune {
		return ev
	}
	r := ev.Base
	if r == 0 {
		r = ev.Rune
		if f, ok := layoutFallback[r]; ok {
			r = f
		}
	}
	ev.Rune, ev.Base = r, r
	return ev
}

func (g *Game) titleKey(ev term.Event) bool {
	g.errMsg = ""
	g.lobbyT = lobbyTicks
	switch ev.Key {
	case term.KeyEsc:
		return true
	case term.KeyBackspace:
		if n := len(g.skillInput); n > 0 {
			g.skillInput = g.skillInput[:n-1]
		}
	case term.KeyEnter:
		g.start()
	case term.KeyRune:
		r := ev.Rune
		if d := physical(ev).Rune; d >= '0' && d <= '9' {
			r = d // digit row on any layout (Czech types +ěščřžýáí there)
		}
		if r >= 'a' && r <= 'z' {
			g.skillInput = strings.ToUpper(string(r))
		} else if r >= '1' && r <= '9' && len(g.skillInput) == 1 {
			g.skillInput += string(r)
		}
	}
	return false
}

// start begins a maze (host or offline) with the chosen skill.
func (g *Game) start() {
	if _, err := core.NewConfig(g.skillInput, 1, true); err != nil {
		g.screen, g.errMsg = screenTitle, "Type a letter A-Z, then a digit 1-9"
		return
	}
	seed := g.opt.Seed
	if seed == 0 {
		seed = uint32(g.opt.Now().UnixNano())
	}
	seed += g.seq
	g.seq++
	if g.host != nil {
		g.host.Start(g.skillInput, seed)
	} else {
		cfg, _ := core.NewConfig(g.skillInput, 1, g.opt.FriendlyFire)
		g.local = core.NewGame(cfg, seed)
	}
	g.enterPlay()
}

func (g *Game) enterPlay() {
	g.screen, g.help, g.msg = screenPlay, false, ""
	g.keys.Reset()
	g.follow()
	g.flash("Shoot the hives! F1 for help", term.White, 3*core.TicksPerSecond)
}

func (g *Game) flash(msg string, colour uint8, ticks uint32) {
	g.msg, g.msgColor = msg, colour
	if s := g.State(); s != nil {
		g.msgUntil = s.Tick + ticks
	}
}

func rk(r rune) term.KeyID { return term.KeyID{Key: term.KeyRune, Rune: r} }

// bindings map key positions (US layout names) to input bits. Arrows, the
// numpad and Home/PgUp/End/PgDn move; the 3 × 3 block Q W E / A S D / Z X C
// fires in the direction of each key from S (S and X both fire down).
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
	{rk('d'), core.FireR}, {rk('a'), core.FireL}, {rk('s'), core.FireD}, {rk('w'), core.FireU}, {rk('x'), core.FireD},
	{rk('e'), core.FireR | core.FireU}, {rk('q'), core.FireL | core.FireU},
	{rk('c'), core.FireR | core.FireD}, {rk('z'), core.FireL | core.FireD},
}

const moveBits = core.MoveR | core.MoveL | core.MoveD | core.MoveU

// Input samples the keyboard into one tick's controls.
func (g *Game) Input() lan.Frame {
	now := g.opt.Now()
	var f lan.Frame
	guessing := false
	for _, b := range bindings {
		if g.keys.Active(b.id, now) {
			f.Mask |= b.bits
			if b.bits&moveBits != 0 && g.keys.Guessing(b.id, now) {
				guessing = true
			}
		}
	}
	f.Fast = g.keys.Active(rk(' '), now)
	f.Mirror = g.mirrorWanted
	// Without key releases a tap is indistinguishable from a hold until the
	// first auto-repeat. Never let such a guess walk the player into a wall.
	if guessing && g.touchesWall(f) {
		f.Mask &^= moveBits
	}
	return f
}

// touchesWall reports whether moving with f would put the player's body
// against a wall within this tick's steps.
func (g *Game) touchesWall(f lan.Frame) bool {
	s := g.State()
	if s == nil || g.Slot() < 0 {
		return false
	}
	i := s.PlayerEntity(int(g.Slot()))
	d, ok := core.MoveDir(f.Mask)
	if i < 0 || !ok {
		return false
	}
	e := s.Ents[i]
	dx, dy := core.DirDelta(d)
	steps := int32(1)
	if f.Fast {
		steps = 2
	}
	for k := int32(1); k <= steps; k++ {
		x, y := e.X+dx*k, e.Y+dy*k
		if s.Wall(x, y) || s.Wall(x+1, y) || s.Wall(x, y+1) || s.Wall(x+1, y+1) {
			return true
		}
	}
	return false
}

// Tick advances one tick: networking, the game, messages, and the frame.
func (g *Game) Tick() {
	switch {
	case g.host != nil:
		g.host.Poll()
	case g.client != nil:
		g.client.Poll()
	}
	switch g.screen {
	case screenTitle:
		g.tickLobby()
	case screenWaiting:
		if g.client.HostGone() {
			g.screen = screenHostLeft
		} else if s := g.client.State(); s != nil {
			g.epochSeen = g.client.Epoch()
			g.enterPlay()
		}
		g.client.Send(lan.Frame{})
	case screenPlay:
		g.tickPlay()
	case screenResult:
		switch {
		case g.client != nil:
			g.client.Send(lan.Frame{})
			if g.client.HostGone() {
				g.screen = screenHostLeft
			} else if g.client.Epoch() != g.epochSeen && g.client.State() != nil {
				g.epochSeen = g.client.Epoch()
				g.enterPlay()
			}
		default:
			if g.host != nil {
				g.host.Step(lan.Frame{}) // keep clients fed during the results
			}
			if g.resultT--; g.resultT <= 0 {
				g.start()
			}
		}
	}
	g.compose()
}

// tickLobby counts down while others wait for a host on its title screen,
// then starts with the skill shown (A1 if it is incomplete).
func (g *Game) tickLobby() {
	if g.host == nil {
		return
	}
	if g.host.Waiting() == 0 {
		g.lobbyT = lobbyTicks
		g.host.SetCountdown(0)
		return
	}
	if g.lobbyT--; g.lobbyT <= 0 {
		if _, err := core.NewConfig(g.skillInput, 1, true); err != nil {
			g.skillInput = "A1"
		}
		g.start()
		return
	}
	g.host.SetCountdown(uint8((g.lobbyT + core.TicksPerSecond - 1) / core.TicksPerSecond))
}

func (g *Game) tickPlay() {
	var events []core.Event
	switch {
	case g.host != nil:
		if g.help {
			g.host.Step(lan.Frame{Mirror: g.mirrorWanted}) // a LAN game cannot pause
		} else {
			g.host.Step(g.Input())
		}
		events = g.host.State().Events
	case g.client != nil:
		if g.client.HostGone() {
			g.screen = screenHostLeft
			return
		}
		f := lan.Frame{Mirror: g.mirrorWanted}
		if !g.help {
			f = g.Input()
		}
		g.client.Send(f)
		events = g.client.TakeEvents()
		if g.client.State() == nil {
			return // resyncing
		}
	default:
		if g.help {
			return // F1 pauses an offline game
		}
		g.local.Step([core.MaxPlayers]core.Input{g.offlineInput(g.Input())})
		events = g.local.Events
	}
	g.react(events)
	g.follow()
	if g.State().Phase != core.PhaseRunning {
		g.screen, g.resultT = screenResult, resultTicks
		g.saveScore()
	}
}

func (g *Game) offlineInput(f lan.Frame) core.Input {
	return core.Input{Mask: f.Mask, Fast: f.Fast, ToggleMirror: f.Mirror != g.local.Players[0].Mirror}
}

// follow points the camera at our player, or the watched player when we
// spectate or are dead; the camera stays put while nobody is alive.
func (g *Game) follow() {
	s := g.State()
	if s == nil {
		return
	}
	if slot := g.Slot(); slot >= 0 {
		if i := s.PlayerEntity(int(slot)); i >= 0 {
			g.cam = term.CameraOn(&s.Ents[i])
			return
		}
		if s.Players[slot].Lives > 0 {
			return // respawning: hold the camera
		}
	}
	if s.PlayerEntity(int(g.watch)) < 0 {
		g.nextWatch()
	}
	if i := s.PlayerEntity(int(g.watch)); i >= 0 {
		g.cam = term.CameraOn(&s.Ents[i])
	}
}

// nextWatch moves the spectator camera to the next living player.
func (g *Game) nextWatch() {
	s := g.State()
	if s == nil {
		return
	}
	for k := 1; k <= core.MaxPlayers; k++ {
		slot := (int(g.watch) + k) % core.MaxPlayers
		if s.PlayerEntity(slot) >= 0 {
			g.watch = int8(slot)
			return
		}
	}
}

// nick returns the name of the player in slot.
func (g *Game) nick(slot int8) string {
	for _, s := range g.roster().Seats {
		if s.Slot == slot {
			return s.Nick
		}
	}
	return fmt.Sprintf("player %d", slot+1)
}

func (g *Game) roster() lan.Roster {
	switch {
	case g.host != nil:
		return g.host.Roster()
	case g.client != nil:
		return g.client.Roster()
	}
	return lan.Roster{Seats: []lan.Seat{{Slot: 0, Nick: g.opt.Nick}}}
}

// react turns game events into messages and the bell.
func (g *Game) react(events []core.Event) {
	s := g.State()
	me := g.Slot()
	for _, ev := range events {
		switch ev.Kind {
		case core.EvHiveDown:
			if ev.Slot == me {
				g.flash(fmt.Sprintf("HIVE DESTROYED! +50   %d to go", s.HivesAlive), term.Yellow, 2*core.TicksPerSecond)
			} else if ev.Slot >= 0 {
				g.flash(fmt.Sprintf("%s destroyed a hive - %d to go", g.nick(ev.Slot), s.HivesAlive), term.Yellow, 2*core.TicksPerSecond)
			}
		case core.EvPlayerDied:
			if ev.Slot == me {
				g.t.Bell()
				if l := s.Players[me].Lives; l > 0 {
					g.flash(fmt.Sprintf("OUCH! %d %s left", l, plural(l, "man", "men")), term.LightRed, 2*core.TicksPerSecond)
				} else {
					g.flash("Out of men - watching (Tab: next player)", term.LightRed, 3*core.TicksPerSecond)
				}
			}
		case core.EvMirror:
			if ev.Slot != me {
				continue
			}
			if s.Players[me].Mirror {
				g.flash("IDDQD! Mirror shots: bank off walls", term.LightMagenta, 3*core.TicksPerSecond)
			} else if g.mirrorWanted {
				g.flash("IDDQD is off while others play", term.LightGray, 2*core.TicksPerSecond)
			} else {
				g.flash("Mirror shots off", term.LightGray, 2*core.TicksPerSecond)
			}
		case core.EvJoin:
			if ev.Slot != me {
				g.flash(g.nick(ev.Slot)+" joined the game", term.LightCyan, 2*core.TicksPerSecond)
			}
		case core.EvLeave:
			if ev.Slot != me {
				g.flash("a player left", term.LightGray, 2*core.TicksPerSecond)
			}
		}
	}
	if s.HivesAlive == 0 && s.SnipesAlive > 0 && s.Tick >= g.msgUntil {
		g.flash(fmt.Sprintf("Hives gone - clear %d %s!", s.SnipesAlive, plural(s.SnipesAlive, "snipe", "snipes")), term.LightGreen, 1)
	}
}

func plural(n int32, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (g *Game) saveScore() {
	s := g.State()
	if s == nil || g.opt.ScoreFile == "" || g.Slot() < 0 {
		return
	}
	skill := s.Cfg.Skill()
	if sc := s.Players[g.Slot()].Score; sc > g.scores[skill] {
		g.scores[skill] = sc
		_ = g.scores.Save(g.opt.ScoreFile)
	}
}

func (g *Game) draw() error {
	return term.Present(g.t, &g.r, g.frame)
}
