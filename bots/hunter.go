package bots

import "github.com/radimlif/nlSnipes/core"

const (
	gridW = core.GridWidth
	gridH = core.GridHeight
	// rayRange is how far the hunter looks along each of its 8 fire lines.
	rayRange = 48
	// replanEvery is how often, in ticks, the distance field is rebuilt.
	replanEvery = 3
)

// Hunter heads for the nearest hive by breadth-first search over the
// positions its 2 × 2 body fits, and fires whenever one of its 8 fire lines
// has a hive or snipe as the first thing on it. Once the hives are gone it
// hunts the remaining snipes the same way.
type Hunter struct {
	pass  []bool  // position fits the body without touching a wall
	dist  []int32 // BFS distance to the nearest firing position, -1 = unreachable
	queue []int32
	tick  int
}

// NewHunter returns a Hunter bot.
func NewHunter() *Hunter { return &Hunter{} }

func pos(x, y int32) int32 {
	return ((y%gridH+gridH)%gridH)*gridW + (x%gridW+gridW)%gridW
}

// Input implements Bot.
func (h *Hunter) Input(s *core.State, slot int) core.Input {
	h.tick++
	i := s.PlayerEntity(slot)
	if i < 0 {
		return core.Input{}
	}
	me := s.Ents[i]
	var in core.Input
	if s.Players[slot].Cooldown == 0 {
		if d, ok := h.aim(s, &me); ok {
			in.Mask |= fireBits[d]
		}
	}
	if h.pass == nil {
		h.initPass(s)
	}
	if h.dist == nil || h.tick%replanEvery == 0 {
		h.plan(s)
	}
	if d, ok := h.step(s, i, &me); ok {
		in.Mask |= moveBits[d]
	}
	return in
}

var (
	fireBits = [8]uint8{core.FireR, core.FireR | core.FireD, core.FireD, core.FireL | core.FireD, core.FireL, core.FireL | core.FireU, core.FireU, core.FireR | core.FireU}
	moveBits = [8]uint8{core.MoveR, core.MoveR | core.MoveD, core.MoveD, core.MoveL | core.MoveD, core.MoveL, core.MoveL | core.MoveU, core.MoveU, core.MoveR | core.MoveU}
)

func isTarget(k core.Kind) bool {
	return k == core.KindHive || k == core.KindSnipe || k == core.KindGhost
}

// aim returns a fire direction whose line reaches something worth shooting
// before any wall or other entity. Priority: an incoming spear, then a
// hive, then the nearest snipe.
func (h *Hunter) aim(s *core.State, me *core.Entity) (uint8, bool) {
	const none = 1 << 30
	best, bestScore := uint8(0), int32(none)
	for d := uint8(0); d < 8; d++ {
		x, y := core.EdgeTile(me, d)
		dx, dy := core.DirDelta(d)
		for k := int32(0); k < rayRange; k++ {
			if s.Wall(x, y) {
				break
			}
			if o := s.At(x, y); o >= 0 {
				score := int32(none)
				switch kind := s.Ents[o].Kind; {
				case kind == core.KindSpear && k < 12:
					score = k
				case kind == core.KindHive:
					score = 100 + k
				case isTarget(kind):
					score = 200 + k
				}
				if score < bestScore {
					best, bestScore = d, score
				}
				break
			}
			x, y = x+dx, y+dy
		}
	}
	return best, bestScore < none
}

func (h *Hunter) initPass(s *core.State) {
	h.pass = make([]bool, gridW*gridH)
	h.dist = nil
	for y := int32(0); y < gridH; y++ {
		for x := int32(0); x < gridW; x++ {
			h.pass[pos(x, y)] = !s.Wall(x, y) && !s.Wall(x+1, y) && !s.Wall(x, y+1) && !s.Wall(x+1, y+1)
		}
	}
}

// plan rebuilds the distance field from every position touching a target.
func (h *Hunter) plan(s *core.State) {
	if h.dist == nil {
		h.dist = make([]int32, gridW*gridH)
	}
	for i := range h.dist {
		h.dist[i] = -1
	}
	h.queue = h.queue[:0]
	wantHives := s.HivesAlive > 0
	for _, e := range s.Ents {
		if e.Dead || !isTarget(e.Kind) || (e.Kind == core.KindHive) != wantHives {
			continue
		}
		w, hh := e.Kind.Size()
		for y := e.Y - 2; y <= e.Y+hh; y++ {
			for x := e.X - 2; x <= e.X+w; x++ {
				if p := pos(x, y); h.pass[p] && h.dist[p] < 0 {
					h.dist[p] = 0
					h.queue = append(h.queue, p)
				}
			}
		}
	}
	for q := 0; q < len(h.queue); q++ {
		p := h.queue[q]
		x, y := p%gridW, p/gridW
		for d := uint8(0); d < 8; d += 2 {
			dx, dy := core.DirDelta(d)
			if n := pos(x+dx, y+dy); h.pass[n] && h.dist[n] < 0 {
				h.dist[n] = h.dist[p] + 1
				h.queue = append(h.queue, n)
			}
		}
	}
}

// step picks the orthogonal move that lowers the distance most without
// touching another entity.
func (h *Hunter) step(s *core.State, self int, me *core.Entity) (uint8, bool) {
	cur := h.dist[pos(me.X, me.Y)]
	if cur <= 0 {
		return 0, false
	}
	best, bestDist := uint8(0), cur
	for d := uint8(0); d < 8; d += 2 {
		dx, dy := core.DirDelta(d)
		nx, ny := me.X+dx, me.Y+dy
		nd := h.dist[pos(nx, ny)]
		if nd < 0 || nd >= bestDist || !h.free(s, self, nx, ny) {
			continue
		}
		best, bestDist = d, nd
	}
	return best, bestDist < cur
}

// free reports whether the body fits at (x, y) with no other entity in it
// and no snipe or spear in the ring of tiles around it.
func (h *Hunter) free(s *core.State, self int, x, y int32) bool {
	for ty := int32(-1); ty < 3; ty++ {
		for tx := int32(-1); tx < 3; tx++ {
			o := s.At(x+tx, y+ty)
			if o < 0 || o == self {
				continue
			}
			inside := tx >= 0 && tx < 2 && ty >= 0 && ty < 2
			if k := s.Ents[o].Kind; inside || k == core.KindSnipe || k == core.KindGhost || k == core.KindSpear {
				return false
			}
		}
	}
	return true
}
