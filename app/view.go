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

func (a *Solo) compose() {
	f := &a.frame
	f.Clear()
	switch a.screen {
	case screenTitle:
		a.composeTitle()
		return
	}
	term.DrawWorld(f, a.game, a.cam)
	a.composeHUD()
	if a.help {
		a.composeHelp()
	}
	if a.screen == screenResult {
		a.composeResult()
	}
}

func centre(f *term.Frame, y int, s string, fg uint8) {
	n := len([]rune(s))
	f.Text((term.Width-n)/2, y, s, fg, term.Black)
}

func (a *Solo) composeTitle() {
	f := &a.frame
	for i, l := range banner {
		centre(f, 2+i, l, term.LightBlue)
	}
	centre(f, 5, "L I G H T", term.LightCyan)
	centre(f, 7, "the 1982 maze shooter, back on your LAN", term.LightGray)

	centre(f, 10, "Skill level (A1 easy - Z9 brutal)", term.White)
	in := a.skillInput
	if len(in) < 2 && (a.frameTick()/9)%2 == 0 {
		in += "_"
	}
	centre(f, 12, fmt.Sprintf("[ %-2s ]", in), term.Yellow)
	if a.errMsg != "" {
		centre(f, 13, a.errMsg, term.LightRed)
	} else {
		centre(f, 13, "Enter to start  ·  Esc to quit", term.DarkGray)
	}

	centre(f, 15, "Arrows move   W A S D fire   Space fast", term.LightGray)
	centre(f, 16, "Letter = tricks, digit = how many", term.DarkGray)

	if len(a.scores) > 0 {
		var keys []string
		for k := range a.scores {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return a.scores[keys[i]] > a.scores[keys[j]] })
		centre(f, 18, "Best scores", term.LightCyan)
		for i, k := range keys[:min(len(keys), 3)] {
			centre(f, 19+i, fmt.Sprintf("%s  %6d", k, a.scores[k]), term.White)
		}
	}
	if !a.keys.Real {
		centre(f, 23, "Tip: for smoother controls play in", term.DarkGray)
		centre(f, 24, "Windows, kitty, WezTerm, Ghostty, iTerm2", term.DarkGray)
	}
}

// frameTick is a clock for blinking on screens without a running game.
func (a *Solo) frameTick() int64 { return a.opt.Now().UnixMilli() / int64(core.TicksPerSecond*3) }

func (a *Solo) composeHUD() {
	f := &a.frame
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
		for i := 0; i < term.Width; i++ {
			f[2][i] = term.Cell{Ch: ' ', Fg: a.msgColor}
		}
		centre(f, 2, a.msg, a.msgColor)
		return
	}
	for i := 0; i < term.Width; i++ {
		f[2][i] = term.Cell{Ch: '─', Fg: term.Blue}
	}
	f.Text(2, 2, fmt.Sprintf(" Snipes %d ", g.SnipesAlive), term.Green, term.Black)
	f.Text(term.Width-11, 2, " F1 help ", term.DarkGray, term.Black)
}

func (a *Solo) box(x0, y0, w, h int, fg uint8) {
	f := &a.frame
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
			f[y][x] = term.Cell{Ch: ch, Fg: fg}
		}
	}
}

func (a *Solo) composeHelp() {
	f := &a.frame
	a.box(2, 4, 36, 19, term.LightCyan)
	centre(f, 5, "HOW TO PLAY  (paused)", term.White)
	rows := []struct {
		glyph  string
		colour uint8
		text   string
	}{
		{"ôô", term.White, "you: arrows move, Space fast"},
		{"○ ", term.Yellow, "your bullet: W A S D fire"},
		{"┌┐", term.Yellow, "hive: shoot it, +50"},
		{"☺→", term.Green, "snipe: +1, deadly to touch"},
		{"☻ ", term.Green, "small snipe: +1, fast"},
		{"↑ ", term.LightGreen, "spear: snipes shoot these"},
		{"══", term.LightBlue, "wall: deadly from letter M"},
	}
	for i, r := range rows {
		f.Text(5, 7+i*2, r.glyph, r.colour, term.Black)
		f.Text(9, 7+i*2, r.text, term.LightGray, term.Black)
	}
	centre(f, 21, "F1 resume  ·  Esc quit", term.DarkGray)
}

func (a *Solo) composeResult() {
	f := &a.frame
	g := a.game
	a.box(6, 8, 28, 9, term.Yellow)
	title, colour := "MAZE CLEARED!", term.LightGreen
	if g.Phase == core.PhaseLost {
		title, colour = "GAME OVER", term.LightRed
	}
	centre(f, 10, title, colour)
	centre(f, 12, fmt.Sprintf("Score %d", g.Players[0].Score), term.White)
	if best := a.scores[g.Cfg.Skill()]; g.Players[0].Score >= best && best > 0 {
		centre(f, 13, "NEW BEST!", term.Yellow)
	}
	centre(f, 15, fmt.Sprintf("Enter: next maze (%ds)", (a.resultT+core.TicksPerSecond-1)/core.TicksPerSecond), term.DarkGray)
}
