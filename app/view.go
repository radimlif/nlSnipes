package app

import (
	"fmt"
	"sort"

	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/term"
)

var banner = [...]string{
	"╔╗╔ ╦   ╔═╗ ╔╗╔ ╦ ╔═╗ ╔═╗ ╔═╗",
	"║║║ ║   ╚═╗ ║║║ ║ ╠═╝ ║╣  ╚═╗",
	"╝╚╝ ╩═╝ ╚═╝ ╝╚╝ ╩ ╩   ╚═╝ ╚═╝",
}

// frameSize is the classic 40 × 25, or as much of the terminal as is
// useful (the full view, default).
func (a *Solo) frameSize() (int, int) {
	if a.classic {
		return term.Width, term.Height
	}
	w, h := a.t.Size()
	return min(max(w, term.Width), term.MaxWidth), min(max(h, term.Height), term.MaxHeight)
}

func (a *Solo) compose() {
	w, h := a.frameSize()
	if a.frame.W != w || a.frame.H != h {
		a.frame.Resize(w, h)
	}
	f := a.frame
	f.Clear()
	if a.screen == screenTitle {
		a.composeTitle()
		return
	}
	term.DrawWorld(f, a.game, a.cam, term.View{X: 0, Y: term.HUDRows, W: f.W, H: f.H - term.HUDRows})
	a.composeHUD()
	if a.help {
		a.composeHelp()
	}
	if a.screen == screenResult {
		a.composeResult()
	}
}

// centre writes s centred on row y of the frame.
func centre(f *term.Frame, y int, s string, fg uint8) {
	n := len([]rune(s))
	f.Text((f.W-n)/2, y, s, fg, term.Black)
}

// panelTop is the top row of an h-row panel centred in the frame.
func (a *Solo) panelTop(h int) int { return (a.frame.H - h) / 2 }

func (a *Solo) composeTitle() {
	f := a.frame
	y0 := a.panelTop(term.Height)
	for i, l := range banner {
		centre(f, y0+2+i, l, term.LightBlue)
	}
	centre(f, y0+5, "L I G H T", term.LightCyan)
	centre(f, y0+7, "the 1982 maze shooter, back on your LAN", term.LightGray)

	centre(f, y0+10, "Skill level (A1 easy - Z9 brutal)", term.White)
	in := a.skillInput
	if len(in) < 2 && (a.frameTick()/9)%2 == 0 {
		in += "_"
	}
	centre(f, y0+12, fmt.Sprintf("[ %-2s ]", in), term.Yellow)
	if a.errMsg != "" {
		centre(f, y0+13, a.errMsg, term.LightRed)
	} else {
		centre(f, y0+13, "Enter to start  ·  Esc to quit", term.DarkGray)
	}

	centre(f, y0+15, "Arrows move   Space fast", term.LightGray)
	centre(f, y0+16, "W A S D fire   Q E Z C fire diagonally", term.LightGray)
	centre(f, y0+17, "Letter = tricks, digit = how many", term.DarkGray)

	if len(a.scores) > 0 {
		var keys []string
		for k := range a.scores {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return a.scores[keys[i]] > a.scores[keys[j]] })
		parts := ""
		for i, k := range keys[:min(len(keys), 3)] {
			if i > 0 {
				parts += "   "
			}
			parts += fmt.Sprintf("%s %d", k, a.scores[k])
		}
		centre(f, y0+19, "Best: "+parts, term.LightCyan)
	}
	if !a.keys.Real {
		centre(f, y0+22, "Tip: for smoother controls play in", term.DarkGray)
		centre(f, y0+23, "Windows, kitty, WezTerm, Ghostty, iTerm2", term.DarkGray)
	}
}

// frameTick is a clock for blinking on screens without a running game.
func (a *Solo) frameTick() int64 { return a.opt.Now().UnixMilli() / int64(core.TicksPerSecond*3) }

func (a *Solo) composeHUD() {
	f := a.frame
	g := a.game
	p := g.Players[0]
	secs := int(g.Tick) / core.TicksPerSecond
	label, value := term.LightGray, term.White
	x := 1
	put := func(y int, l, v string) {
		f.Text(x, y, l, label, term.Black)
		x += len([]rune(l))
		f.Text(x, y, v, value, term.Black)
		x += len([]rune(v)) + 3
	}
	put(0, "Skill ", g.Cfg.Skill())
	put(0, "Time ", fmt.Sprintf("%02d:%02d", secs/60, secs%60))
	put(0, "Men Left ", fmt.Sprint(max(p.Lives, 0)))
	x = 1
	put(1, "Score ", fmt.Sprintf("%05d", p.Score))
	put(1, "Best ", fmt.Sprintf("%05d", max(a.scores[g.Cfg.Skill()], p.Score)))
	put(1, "Hives ", fmt.Sprintf("%d/%d", g.HivesAlive, g.Cfg.Hives))

	if a.msg != "" && g.Tick < a.msgUntil {
		for i := 0; i < f.W; i++ {
			f.Set(i, 2, term.Cell{Ch: ' ', Fg: a.msgColor})
		}
		centre(f, 2, a.msg, a.msgColor)
		return
	}
	for i := 0; i < f.W; i++ {
		f.Set(i, 2, term.Cell{Ch: '─', Fg: term.Blue})
	}
	f.Text(2, 2, fmt.Sprintf(" Snipes %d ", g.SnipesAlive), term.Green, term.Black)
	if p.Mirror {
		f.Text(15, 2, " MIRROR ", term.LightMagenta, term.Black)
	}
	hint := " F1 help "
	if f.W > term.Width {
		hint = " V classic view · F1 help "
	}
	f.Text(f.W-len([]rune(hint))-2, 2, hint, term.DarkGray, term.Black)
}

func (a *Solo) box(w, h int, fg uint8) (x0, y0 int) {
	f := a.frame
	x0, y0 = (f.W-w)/2, a.panelTop(h)
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			ch := ' '
			switch {
			case y == y0 && x == x0:
				ch = '╔'
			case y == y0 && x == x0+w-1:
				ch = '╗'
			case y == y0+h-1 && x == x0:
				ch = '╚'
			case y == y0+h-1 && x == x0+w-1:
				ch = '╝'
			case y == y0 || y == y0+h-1:
				ch = '═'
			case x == x0 || x == x0+w-1:
				ch = '║'
			}
			f.Set(x, y, term.Cell{Ch: ch, Fg: fg})
		}
	}
	return x0, y0
}

func (a *Solo) composeHelp() {
	f := a.frame
	x0, y0 := a.box(38, 21, term.LightCyan)
	centre(f, y0+1, "HOW TO PLAY  (paused)", term.White)
	rows := []struct {
		glyph  string
		colour uint8
		text   string
	}{
		{"ôô", term.White, "you: arrows, numpad, Home/PgUp/"},
		{"  ", term.White, "End/PgDn move; Space fast"},
		{"○ ", term.Yellow, "bullet: WASD straight, QEZC diag"},
		{"┌┐", term.Yellow, "hive: shoot it, +50"},
		{"☺→", term.Green, "snipe: +1, deadly to touch"},
		{"☻ ", term.Green, "small snipe: +1, fast"},
		{"↑ ", term.LightGreen, "spear: snipes shoot these"},
		{"══", term.LightBlue, "wall: deadly from letter M"},
	}
	for i, r := range rows {
		f.Text(x0+3, y0+3+i*2, r.glyph, r.colour, term.Black)
		f.Text(x0+6, y0+3+i*2, r.text, term.LightGray, term.Black)
	}
	centre(f, y0+19, "V view  ·  F1 resume  ·  Esc quit", term.DarkGray)
}

func (a *Solo) composeResult() {
	f := a.frame
	g := a.game
	_, y0 := a.box(28, 9, term.Yellow)
	title, colour := "MAZE CLEARED!", term.LightGreen
	if g.Phase == core.PhaseLost {
		title, colour = "GAME OVER", term.LightRed
	}
	centre(f, y0+2, title, colour)
	centre(f, y0+4, fmt.Sprintf("Score %d", g.Players[0].Score), term.White)
	if best := a.scores[g.Cfg.Skill()]; g.Players[0].Score >= best && best > 0 {
		centre(f, y0+5, "NEW BEST!", term.Yellow)
	}
	centre(f, y0+7, fmt.Sprintf("Enter: next maze (%ds)", (a.resultT+core.TicksPerSecond-1)/core.TicksPerSecond), term.DarkGray)
}
