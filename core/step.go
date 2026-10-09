package core

type moveResult uint8

const (
	moveOK moveResult = iota
	moveBlocked
	moveDied
)

// Large snipe placement next to a hive for each heading, relative to the
// hive's top-left tile; every spot lies inside the hive's cell interior.
var hiveExit = [8][2]int32{{2, 0}, {2, 2}, {0, 2}, {-2, 2}, {-2, 0}, {-2, -1}, {0, -1}, {2, -1}}

// Step advances the game by one tick. Order: players (by slot), bullets,
// spears, large snipes, small snipes, hives, debris — each over entities in
// id order. Entities created during the tick first act on the next one.
func (s *State) Step(in [MaxPlayers]Input) {
	if s.Phase != PhaseRunning {
		return
	}
	s.Events = s.Events[:0]
	n := len(s.Ents)
	for slot := 0; slot < MaxPlayers; slot++ {
		s.stepPlayer(slot, in[slot])
	}
	for _, k := range [...]Kind{KindBullet, KindSpear, KindSnipe, KindGhost, KindHive, KindDebris} {
		for i := 0; i < n; i++ {
			if s.Ents[i].Kind != k || s.Ents[i].Dead {
				continue
			}
			switch k {
			case KindBullet:
				s.stepBullet(i)
			case KindSpear:
				s.stepSpear(i)
			case KindSnipe:
				s.stepSnipe(i)
			case KindGhost:
				s.stepGhost(i)
			case KindHive:
				s.stepHive(i)
			case KindDebris:
				if s.Ents[i].Timer--; s.Ents[i].Timer <= 0 {
					s.Ents[i].Dead = true
				}
			}
		}
	}
	s.compact()
	s.Tick++
	switch {
	case s.HivesAlive == 0 && s.SnipesAlive == 0:
		s.Phase = PhaseWon
	case s.allOut():
		s.Phase = PhaseLost
	}
}

func (s *State) compact() {
	live := s.Ents[:0]
	for _, e := range s.Ents {
		if !e.Dead {
			live = append(live, e)
		}
	}
	for i := len(live); i < len(s.Ents); i++ {
		s.Ents[i] = Entity{}
	}
	s.Ents = live
	s.rebuildOcc()
}

func (s *State) allOut() bool {
	for _, p := range s.Players {
		if p.Joined && p.Lives > 0 {
			return false
		}
	}
	return true
}

// axis returns +1, -1 or 0 for a pair of opposing bits; both set cancel.
func axis(mask, pos, neg uint8) int32 {
	v := int32(0)
	if mask&pos != 0 {
		v++
	}
	if mask&neg != 0 {
		v--
	}
	return v
}

// MoveDir and FireDir decode an input mask into an 8-way direction.
func MoveDir(mask uint8) (uint8, bool) {
	dx, dy := axis(mask, MoveR, MoveL), axis(mask, MoveD, MoveU)
	if dx == 0 && dy == 0 {
		return 0, false
	}
	return dirOf(dx, dy)
}

// FireDir is MoveDir for the fire bits.
func FireDir(mask uint8) (uint8, bool) {
	dx, dy := axis(mask, FireR, FireL), axis(mask, FireD, FireU)
	if dx == 0 && dy == 0 {
		return 0, false
	}
	return dirOf(dx, dy)
}

func (s *State) stepPlayer(slot int, in Input) {
	p := &s.Players[slot]
	if !p.Joined {
		return
	}
	if in.ToggleMirror {
		p.Mirror = !p.Mirror
		s.emit(EvMirror, p.SpawnX, p.SpawnY, int8(slot))
	}
	i := s.PlayerEntity(slot)
	if i < 0 {
		if p.Respawn > 0 {
			if p.Respawn--; p.Respawn == 0 {
				s.respawn(slot)
			}
		}
		return
	}
	if p.Cooldown > 0 {
		p.Cooldown--
	}
	if d, ok := MoveDir(in.Mask); ok {
		steps := 1
		if in.Fast {
			steps = 2
		}
		for k := 0; k < steps; k++ {
			if s.movePlayer(i, d) != moveOK {
				break
			}
		}
	}
	if s.Ents[i].Dead {
		return
	}
	if d, ok := FireDir(in.Mask); ok && p.Cooldown == 0 {
		p.Cooldown = FireCooldown
		s.fire(i, d)
	}
}

// movePlayer moves one tile; a blocked diagonal slides along whichever
// axis is free, horizontal first.
func (s *State) movePlayer(i int, d uint8) moveResult {
	r := s.tryMove(i, d)
	if r != moveBlocked || d&1 == 0 {
		return r
	}
	dx, dy := DirDelta(d)
	h, _ := dirOf(dx, 0)
	if r = s.tryMove(i, h); r != moveBlocked {
		return r
	}
	v, _ := dirOf(0, dy)
	return s.tryMove(i, v)
}

// tryMove moves a player, large snipe or small snipe one tile in direction d.
// Walls and solid entities block (electric walls kill players); a player and
// a snipe that meet both die; a player walking into a spear dies.
func (s *State) tryMove(i int, d uint8) moveResult {
	e := s.Ents[i]
	dx, dy := DirDelta(d)
	nx, ny := wrapX(e.X+dx), wrapY(e.Y+dy)
	w, h := e.Kind.Size()
	contact := -1
	for ty := int32(0); ty < h; ty++ {
		for tx := int32(0); tx < w; tx++ {
			if s.Wall(nx+tx, ny+ty) {
				if e.Kind == KindPlayer && s.Cfg.ElectricWalls {
					s.killPlayer(i)
					return moveDied
				}
				return moveBlocked
			}
		}
	}
	for ty := int32(0); ty < h; ty++ {
		for tx := int32(0); tx < w; tx++ {
			o := s.At(nx+tx, ny+ty)
			if o < 0 || o == i {
				continue
			}
			ok := s.Ents[o].Kind
			switch {
			case e.Kind == KindPlayer && (ok == KindSnipe || ok == KindGhost || ok == KindSpear):
				contact = o
			case e.Kind != KindPlayer && ok == KindPlayer:
				contact = o
			default:
				return moveBlocked
			}
		}
	}
	if contact >= 0 {
		player, other := i, contact
		if e.Kind != KindPlayer {
			player, other = contact, i
		}
		if s.Ents[other].Kind == KindSpear {
			s.remove(other)
		} else {
			s.killEntity(other, -1, false)
		}
		s.killPlayer(player)
		return moveDied
	}
	s.stamp(i, 0)
	s.Ents[i].X, s.Ents[i].Y, s.Ents[i].Dir = nx, ny, d
	s.stamp(i, int32(i+1))
	return moveOK
}

// EdgeTile is the tile just outside entity e's footprint in direction d:
// where a bullet or spear fired that way appears.
func EdgeTile(e *Entity, d uint8) (int32, int32) {
	w, h := e.Kind.Size()
	dx, dy := DirDelta(d)
	x, y := e.X, e.Y
	switch {
	case dx > 0:
		x += w
	case dx < 0:
		x--
	}
	switch {
	case dy > 0:
		y += h
	case dy < 0:
		y--
	}
	return wrapX(x), wrapY(y)
}

func (s *State) fire(i int, d uint8) {
	bx, by := EdgeTile(&s.Ents[i], d)
	owner := s.Ents[i].Owner
	if s.Wall(bx, by) {
		return
	}
	var bounces uint8
	switch {
	case s.Players[owner].Mirror:
		bounces = MirrorBounces
	case s.Cfg.Bounce && d&1 == 1:
		bounces = uint8(1 + s.Rng.Mask(7))
	}
	s.emit(EvShot, bx, by, owner)
	if o := s.At(bx, by); o >= 0 {
		s.bulletHits(o, owner)
		return
	}
	s.spawn(Entity{Kind: KindBullet, X: bx, Y: by, Dir: d, Owner: owner, Bounces: bounces})
}

// bulletHits applies a player bullet striking entity o.
func (s *State) bulletHits(o int, owner int8) {
	switch t := s.Ents[o]; t.Kind {
	case KindHive, KindGhost:
		s.killEntity(o, owner, false)
	case KindSnipe:
		s.killEntity(o, owner, true)
	case KindSpear, KindBullet:
		s.remove(o)
	case KindPlayer:
		if s.Cfg.FriendlyFire && t.Owner != owner {
			s.killPlayer(o)
		}
	}
}

func (s *State) stepBullet(i int) {
	for k := 0; k < 2; k++ {
		e := &s.Ents[i]
		dx, dy := DirDelta(e.Dir)
		if s.Wall(e.X+dx, e.Y+dy) {
			if e.Bounces == 0 {
				s.remove(i)
				return
			}
			hx, vy := s.Wall(e.X+dx, e.Y), s.Wall(e.X, e.Y+dy)
			if hx {
				dx = -dx
			}
			if vy {
				dy = -dy
			}
			if !hx && !vy {
				dx, dy = -dx, -dy
			}
			e.Dir, _ = dirOf(dx, dy)
			e.Bounces--
			s.emit(EvBounce, e.X, e.Y, e.Owner)
			if s.Wall(e.X+dx, e.Y+dy) {
				s.remove(i)
				return
			}
		}
		nx, ny := wrapX(e.X+dx), wrapY(e.Y+dy)
		if o := s.At(nx, ny); o >= 0 {
			owner := e.Owner
			s.remove(i)
			s.bulletHits(o, owner)
			return
		}
		s.stamp(i, 0)
		e.X, e.Y = nx, ny
		s.stamp(i, int32(i+1))
	}
}

func (s *State) stepSpear(i int) {
	for k := 0; k < 2; k++ {
		e := &s.Ents[i]
		dx, dy := DirDelta(e.Dir)
		nx, ny := wrapX(e.X+dx), wrapY(e.Y+dy)
		if s.Wall(nx, ny) {
			s.remove(i)
			return
		}
		if o := s.At(nx, ny); o >= 0 {
			s.remove(i)
			switch s.Ents[o].Kind {
			case KindPlayer:
				s.killPlayer(o)
			case KindHive:
				if !s.Cfg.HivesResistSpears {
					s.killEntity(o, -1, false)
				}
			case KindBullet:
				s.remove(o)
			}
			return
		}
		s.stamp(i, 0)
		e.X, e.Y = nx, ny
		s.stamp(i, int32(i+1))
	}
}

func (s *State) stepSnipe(i int) {
	if s.Ents[i].Timer--; s.Ents[i].Timer > 0 {
		return
	}
	s.Ents[i].Timer = SnipeMoveRate
	e := &s.Ents[i]
	if s.Rng.Mask(3) == 0 {
		e.Dir = (e.Dir + uint8(e.Turn)) & 7
		if s.Rng.Mask(3) == 0 {
			e.Dir = uint8(s.Rng.Int(8))
		}
	}
	d := e.Dir
	switch s.tryMove(i, d) {
	case moveBlocked:
		s.Ents[i].Dir = (d + uint8(s.Ents[i].Turn)) & 7
	case moveDied:
		return
	}
	if s.snipeWantsToFire(i) {
		s.snipeFire(i, s.Ents[i].Dir)
	}
}

// snipeWantsToFire: a large snipe shoots only along its heading, and only
// when the nearest player lies on that line — straight ahead, or close to
// the diagonal (|6·|dx| − 8·|dy|| < 8, allowing for 8 × 6 cells). The odds
// fall with distance and rise with the letter's accuracy: with
// shift = (|dx|+|dy|) >> accuracy, it fires with chance 1 in 2^(shift+1),
// never when shift > 10.
func (s *State) snipeWantsToFire(i int) bool {
	e := &s.Ents[i]
	dx, dy, ok := s.nearestPlayer(e.X, e.Y)
	if !ok {
		return false
	}
	toward, _ := dirOf(sign32(dx), sign32(dy))
	if toward != e.Dir {
		return false
	}
	ax, ay := abs32(dx), abs32(dy)
	if ax != 0 && ay != 0 && abs32(6*ax-8*ay) >= 8 {
		return false
	}
	shift := uint32(ax+ay) >> s.Cfg.Accuracy
	if shift > 10 {
		return false
	}
	return s.Rng.Mask(0xFFFF>>(15-shift)) == 0
}

func (s *State) snipeFire(i int, d uint8) {
	x, y := EdgeTile(&s.Ents[i], d)
	if s.Wall(x, y) {
		return
	}
	if o := s.At(x, y); o >= 0 {
		if s.Ents[o].Kind == KindPlayer {
			s.killPlayer(o)
		}
		return
	}
	s.spawn(Entity{Kind: KindSpear, X: x, Y: y, Dir: d, Owner: -1})
}

func (s *State) stepGhost(i int) {
	if s.Rng.Mask(7) == 0 {
		s.Ents[i].Dir = uint8(s.Rng.Int(8))
	}
	if s.tryMove(i, s.Ents[i].Dir) == moveBlocked {
		s.Ents[i].Dir = uint8(s.Rng.Int(8))
	}
}

func (s *State) stepHive(i int) {
	if s.Ents[i].Timer--; s.Ents[i].Timer > 0 {
		return
	}
	hx, hy := s.Ents[i].X, s.Ents[i].Y
	dist := int32(GridWidth / 2)
	if dx, dy, ok := s.nearestPlayer(hx, hy); ok {
		dist = abs32(dx) + abs32(dy)
	}
	reload := int32(5)
	if shift := s.Tick/256 + 1; shift < 31 {
		reload += dist >> shift
	}
	s.Ents[i].Timer = reload
	if s.SnipesAlive >= int32(s.Cfg.MaxSnipes) {
		return
	}
	lost := int32(s.Cfg.Hives) - s.HivesAlive
	if s.Rng.Mask(0xF>>uint32(min(lost, 31))) != 0 {
		return
	}
	d := uint8(s.Rng.Int(8))
	x, y := wrapX(hx+hiveExit[d][0]), wrapY(hy+hiveExit[d][1])
	for tx := int32(0); tx < 2; tx++ {
		if s.Wall(x+tx, y) || s.At(x+tx, y) >= 0 {
			return
		}
	}
	turn := int8(1)
	if s.Rng.Mask(1) == 0 {
		turn = -1
	}
	s.spawn(Entity{Kind: KindSnipe, X: x, Y: y, Dir: d, Turn: turn, Owner: -1, Timer: SnipeMoveRate})
	s.SnipesAlive++
	s.emit(EvSpawn, x, y, -1)
}

// nearestPlayer returns the wrapped offset from (x, y) to the closest living
// player body (Chebyshev distance; lowest slot wins ties).
func (s *State) nearestPlayer(x, y int32) (int32, int32, bool) {
	best, bx, by, found := int32(1<<30), int32(0), int32(0), false
	for i := range s.Ents {
		e := &s.Ents[i]
		if e.Kind != KindPlayer || e.Dead {
			continue
		}
		dx, dy := WrapDelta(x, e.X, GridWidth), WrapDelta(y, e.Y, GridHeight)
		if d := max(abs32(dx), abs32(dy)); d < best {
			best, bx, by, found = d, dx, dy, true
		}
	}
	return bx, by, found
}

func (s *State) remove(i int) {
	s.Ents[i].Dead = true
	s.stamp(i, 0)
}

// killEntity destroys a hive or snipe. scorer is the player slot credited,
// or -1. A large snipe shot with split set may leave a small snipe behind
// (ghost letters) unless it explodes (odds 1 in ExplosionMask+1).
func (s *State) killEntity(i int, scorer int8, split bool) {
	e := s.Ents[i]
	s.remove(i)
	points := int32(1)
	switch e.Kind {
	case KindHive:
		s.HivesAlive--
		points = 50
		s.emit(EvHiveDown, e.X, e.Y, scorer)
	default:
		s.SnipesAlive--
		s.emit(EvHit, e.X, e.Y, scorer)
	}
	if scorer >= 0 {
		s.Score += points
		s.Players[scorer].Score += points
	}
	if split && s.Cfg.SmallSnipes && s.Rng.Mask(uint32(s.Cfg.ExplosionMask)) != 0 {
		s.spawn(Entity{Kind: KindGhost, X: e.X, Y: e.Y, Dir: uint8(s.Rng.Int(8)), Owner: -1})
		s.SnipesAlive++
		return
	}
	s.spawn(Entity{Kind: KindDebris, X: e.X, Y: e.Y, Dir: uint8(e.Kind), Owner: -1, Timer: DebrisLife})
}

func (s *State) killPlayer(i int) {
	e := s.Ents[i]
	s.remove(i)
	p := &s.Players[e.Owner]
	p.Lives--
	if p.Lives > 0 {
		p.Respawn = RespawnDelay
	}
	s.emit(EvPlayerDied, e.X, e.Y, e.Owner)
	s.spawn(Entity{Kind: KindDebris, X: e.X, Y: e.Y, Dir: uint8(e.Kind), Owner: -1, Timer: DebrisLife})
}

// respawn puts slot back at its spawn point, or the first cell centre after
// it (in cell order) whose surroundings are clear for two tiles.
func (s *State) respawn(slot int) {
	p := &s.Players[slot]
	start := cellIndex(p.SpawnX/CellWidth, p.SpawnY/CellHeight)
	for k := int32(0); k < NumCells; k++ {
		x, y := cellCentre((start + k) % NumCells)
		if s.clear(x-2, y-2, 6, 6) {
			s.spawn(Entity{Kind: KindPlayer, X: x, Y: y, Owner: int8(slot)})
			s.emit(EvRespawn, x, y, int8(slot))
			return
		}
	}
	p.Respawn = 1 // nowhere safe this tick; try again next tick
}

// clear reports whether a w × h box has no walls and no entities.
func (s *State) clear(x, y, w, h int32) bool {
	for ty := int32(0); ty < h; ty++ {
		for tx := int32(0); tx < w; tx++ {
			if s.At(x+tx, y+ty) >= 0 {
				return false
			}
		}
	}
	for ty := int32(2); ty < 4; ty++ {
		for tx := int32(2); tx < 4; tx++ {
			if s.Wall(x+tx, y+ty) {
				return false
			}
		}
	}
	return true
}
