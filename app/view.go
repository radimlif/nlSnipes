package app

import (
	"fmt"
	"sort"

	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/lan"
	"github.com/radimlif/nlSnipes/term"
)

var banner = [...]string{
	"╔╗╔ ╦   ╔═╗ ╔╗╔ ╦ ╔═╗ ╔═╗ ╔═╗",
	"║║║ ║   ╚═╗ ║║║ ║ ╠═╝ ║╣  ╚═╗",
	"╝╚╝ ╩═╝ ╚═╝ ╝╚╝ ╩ ╩   ╚═╝ ╚═╝",
}

// frameSize is the classic 40 × 25, or as much of the terminal as is
// useful (the full view, default).
func (g *Game) frameSize() (int, int) {
	if g.classic {
		return term.Width, term.Height
	}
	w, h := g.t.Size()
	return min(max(w, term.Width), term.MaxWidth), min(max(h, term.Height), term.MaxHeight)
}

func (g *Game) compose() {
	w, h := g.frameSize()
	if g.frame.W != w || g.frame.H != h {
		g.frame.Resize(w, h)
	}
	f := g.frame
	f.Clear()
	switch g.screen {
	case screenTitle:
		g.composeTitle()
		return
	case screenWaiting:
		g.composeWaiting()
		return
	case screenHostLeft:
		g.composeMessage("The host left the game.", "Esc to quit, then start again to host")
		return
	}
	term.DrawWorld(f, g.State(), g.cam, term.View{X: 0, Y: term.HUDRows, W: f.W, H: f.H - term.HUDRows})
	g.composeHUD()
	if g.help {
		g.composeHelp()
	}
	if g.screen == screenResult {
		g.composeResult()
	}
}

// centre writes s centred on row y of the frame.
func centre(f *term.Frame, y int, s string, fg uint8) {
	n := len([]rune(s))
	f.Text((f.W-n)/2, y, s, fg, term.Black)
}

// panelTop is the top row of an h-row panel centred in the frame.
func (g *Game) panelTop(h int) int { return (g.frame.H - h) / 2 }

func (g *Game) composeBanner(y0 int) {
	for i, l := range banner {
		centre(g.frame, y0+2+i, l, term.LightBlue)
	}
	centre(g.frame, y0+5, "L I G H T", term.LightCyan)
}

func (g *Game) composeMessage(line, hint string) {
	y0 := g.panelTop(term.Height)
	g.composeBanner(y0)
	centre(g.frame, y0+11, line, term.White)
	centre(g.frame, y0+13, hint, term.DarkGray)
}

// composeWaiting is the client's lobby: who hosts, who is in, and what we
// are waiting for.
func (g *Game) composeWaiting() {
	f := g.frame
	y0 := g.panelTop(term.Height)
	g.composeBanner(y0)
	r := g.client.Roster()
	if !g.client.Joined() || len(r.Seats) == 0 {
		centre(f, y0+10, "Joining the game on the LAN...", term.White)
		centre(f, y0+13, "Esc to quit", term.DarkGray)
		return
	}
	host := r.Seats[0].Nick
	centre(f, y0+8, fmt.Sprintf("Joined %s's game", host), term.LightGreen)
	centre(f, y0+9, fmt.Sprintf("waiting for %s to choose the skill and start", host), term.White)
	y := y0 + 11
	for _, seat := range r.Seats {
		line, colour := "watching  "+seat.Nick, term.DarkGray
		if seat.Slot >= 0 {
			line, colour = fmt.Sprintf("player %d  %s", seat.Slot+1, seat.Nick), term.PlayerColours[seat.Slot]
		}
		if seat.ClientID != 0 && seat.Slot == g.client.Slot() && seat.Nick == g.opt.Nick {
			line += "  (you)"
		}
		centre(f, y, line, colour)
		y++
	}
	centre(f, y+1, "Esc to quit", term.DarkGray)
}

func (g *Game) composeTitle() {
	f := g.frame
	y0 := g.panelTop(term.Height)
	g.composeBanner(y0)
	centre(f, y0+7, "the 1982 maze shooter, back on your LAN", term.LightGray)

	centre(f, y0+10, "Skill level (A1 easy - Z9 brutal)", term.White)
	in := g.skillInput
	if len(in) < 2 && (g.frameTick()/9)%2 == 0 {
		in += "_"
	}
	centre(f, y0+12, fmt.Sprintf("[ %-2s ]", in), term.Yellow)
	if g.errMsg != "" {
		centre(f, y0+13, g.errMsg, term.LightRed)
	} else {
		centre(f, y0+13, "Enter to start  ·  Esc to quit", term.DarkGray)
	}

	centre(f, y0+15, "Arrows move   Space fast", term.LightGray)
	centre(f, y0+16, "Fire: Q W E / A S D / Z X C around S", term.LightGray)
	if g.host != nil {
		waiting := len(g.host.Roster().Seats) - 1
		msg := "Others on the LAN can join: just start the game"
		if waiting > 0 {
			msg = fmt.Sprintf("%d %s waiting to play", waiting, plural(int32(waiting), "player is", "players are"))
		}
		centre(f, y0+17, msg, term.LightGreen)
	}

	if len(g.scores) > 0 {
		var keys []string
		for k := range g.scores {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return g.scores[keys[i]] > g.scores[keys[j]] })
		parts := ""
		for i, k := range keys[:min(len(keys), 3)] {
			if i > 0 {
				parts += "   "
			}
			parts += fmt.Sprintf("%s %d", k, g.scores[k])
		}
		centre(f, y0+19, "Best: "+parts, term.LightCyan)
	}
	if !g.keys.Real {
		centre(f, y0+22, "Tip: for smoother controls play in", term.DarkGray)
		centre(f, y0+23, "Windows, kitty, WezTerm, Ghostty, iTerm2", term.DarkGray)
	}
}

// frameTick is a clock for blinking on screens without a running game.
func (g *Game) frameTick() int64 { return g.opt.Now().UnixMilli() / int64(core.TicksPerSecond*3) }

func (g *Game) composeHUD() {
	f := g.frame
	s := g.State()
	secs := int(s.Tick) / core.TicksPerSecond
	label, value := term.LightGray, term.White
	x := 1
	put := func(y int, l, v string) {
		f.Text(x, y, l, label, term.Black)
		x += len([]rune(l))
		f.Text(x, y, v, value, term.Black)
		x += len([]rune(v)) + 3
	}
	me := g.Slot()
	put(0, "Skill ", s.Cfg.Skill())
	put(0, "Time ", fmt.Sprintf("%02d:%02d", secs/60, secs%60))
	if me >= 0 {
		p := s.Players[me]
		put(0, "Men Left ", fmt.Sprint(max(p.Lives, 0)))
		x = 1
		put(1, "Score ", fmt.Sprintf("%05d", p.Score))
		put(1, "Best ", fmt.Sprintf("%05d", max(g.scores[s.Cfg.Skill()], p.Score)))
	} else {
		x = 1
		f.Text(x, 1, "SPECTATING ", term.LightCyan, term.Black)
		x += 11
		put(1, "", g.nick(g.watch))
	}
	put(1, "Hives ", fmt.Sprintf("%d/%d", s.HivesAlive, s.Cfg.Hives))
	g.composeRoster(s)

	if g.msg != "" && s.Tick < g.msgUntil {
		for i := 0; i < f.W; i++ {
			f.Set(i, 2, term.Cell{Ch: ' ', Fg: g.msgColor})
		}
		centre(f, 2, g.msg, g.msgColor)
		return
	}
	for i := 0; i < f.W; i++ {
		f.Set(i, 2, term.Cell{Ch: '─', Fg: term.Blue})
	}
	f.Text(2, 2, fmt.Sprintf(" Snipes %d ", s.SnipesAlive), term.Green, term.Black)
	if me >= 0 && s.Players[me].Mirror {
		f.Text(15, 2, " MIRROR ", term.LightMagenta, term.Black)
	}
	hint := " F1 help "
	switch {
	case me < 0:
		hint = " Tab: next player · F1 help "
	case f.W > term.Width:
		hint = " V classic view · F1 help "
	}
	f.Text(f.W-len([]rune(hint))-2, 2, hint, term.DarkGray, term.Black)
}

// composeRoster lists the other players at the right of the top HUD rows
// when the frame is wide enough: slot colour, nick, score, men left.
func (g *Game) composeRoster(s *core.State) {
	f := g.frame
	if f.W < 70 || (g.host == nil && g.client == nil) {
		return
	}
	var lines []string
	var colours []uint8
	for _, seat := range g.roster().Seats {
		if seat.Slot < 0 {
			continue
		}
		p := s.Players[seat.Slot]
		lines = append(lines, fmt.Sprintf("◄► %-12s %5d  %d", seat.Nick, p.Score, max(p.Lives, 0)))
		colours = append(colours, term.PlayerColours[seat.Slot])
	}
	specs := 0
	for _, seat := range g.roster().Seats {
		if seat.Slot == lan.Spectator {
			specs++
		}
	}
	col := f.W - 28
	for i, l := range lines {
		row, c := i%2, col
		if i >= 2 {
			c = col - 29
		}
		if c > 40 {
			f.Text(c, row, l, colours[i], term.Black)
		}
	}
	if specs > 0 {
		f.Text(col, 2, fmt.Sprintf(" %d watching ", specs), term.DarkGray, term.Black)
	}
}

func (g *Game) box(w, h int, fg uint8) (x0, y0 int) {
	f := g.frame
	x0, y0 = (f.W-w)/2, g.panelTop(h)
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

func (g *Game) composeHelp() {
	f := g.frame
	x0, y0 := g.box(38, 21, term.LightCyan)
	title := "HOW TO PLAY  (paused)"
	if g.host != nil || g.client != nil {
		title = "HOW TO PLAY  (game goes on!)"
	}
	centre(f, y0+1, title, term.White)
	rows := []struct {
		glyph  string
		colour uint8
		text   string
	}{
		{"ôô", term.White, "you: arrows, numpad, Home/PgUp/"},
		{"  ", term.White, "End/PgDn move; Space fast"},
		{"○ ", term.Yellow, "fire: QWE/ASD/ZXC, away from S"},
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

func (g *Game) composeResult() {
	f := g.frame
	s := g.State()
	rows := 0
	for _, p := range s.Players {
		if p.Joined {
			rows++
		}
	}
	_, y0 := g.box(32, 9+rows, term.Yellow)
	title, colour := "MAZE CLEARED!", term.LightGreen
	if s.Phase == core.PhaseLost {
		title, colour = "GAME OVER", term.LightRed
	}
	centre(f, y0+2, title, colour)
	y := y0 + 4
	for slot, p := range s.Players {
		if !p.Joined {
			continue
		}
		centre(f, y, fmt.Sprintf("%-12s %6d", g.nick(int8(slot)), p.Score), term.PlayerColours[slot])
		y++
	}
	if me := g.Slot(); me >= 0 {
		if best := g.scores[s.Cfg.Skill()]; s.Players[me].Score >= best && best > 0 {
			centre(f, y, "NEW BEST!", term.Yellow)
		}
	}
	next := fmt.Sprintf("Enter: next maze (%ds)", (g.resultT+core.TicksPerSecond-1)/core.TicksPerSecond)
	if g.client != nil {
		next = "next maze starts soon"
	}
	centre(f, y0+7+rows, next, term.DarkGray)
}
