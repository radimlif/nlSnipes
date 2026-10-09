package core

// Kind identifies an entity type.
type Kind uint8

// Entity kinds.
const (
	KindPlayer Kind = iota + 1
	KindHive
	KindSnipe // large snipe, 2 × 1
	KindGhost // small snipe, 1 × 1
	KindBullet
	KindSpear
	KindDebris // explosion remnant: never collides; Dir holds the Kind that exploded
)

var kindW = [8]int32{0, 2, 2, 2, 1, 1, 1, 1}
var kindH = [8]int32{0, 2, 2, 1, 1, 1, 1, 1}

// Size returns the footprint of a kind in tiles.
func (k Kind) Size() (w, h int32) { return kindW[k], kindH[k] }

// Entity is anything on the map. X, Y is the top-left tile of its footprint.
type Entity struct {
	ID      uint32
	Kind    Kind
	X, Y    int32
	Dir     uint8 // heading; for debris, the Kind that exploded
	Turn    int8  // large snipe: fixed turning direction, +1 or -1
	Owner   int8  // player slot for players and bullets, -1 otherwise
	Bounces uint8 // bullet: wall bounces left
	Timer   int32 // hive spawn countdown, snipe move countdown, debris life
	Dead    bool  // removed at the end of the tick
}

// Player is a slot's persistent state; the on-map body is a KindPlayer entity.
type Player struct {
	Joined   bool
	Lives    int32
	Score    int32
	Respawn  int32 // ticks until the body reappears; 0 while alive or out
	Cooldown int32 // ticks until the next shot
	SpawnX   int32
	SpawnY   int32
}

// Phase is where the game is in its life.
type Phase uint8

// Game phases.
const (
	PhaseRunning Phase = iota
	PhaseWon
	PhaseLost
)

// EventKind tells renderers and sound what happened during a tick.
type EventKind uint8

// Events.
const (
	EvShot EventKind = iota + 1
	EvHit
	EvHiveDown
	EvPlayerDied
	EvSpawn
	EvBounce
	EvRespawn
)

// Event is one thing that happened this tick. Events are not part of the hash.
type Event struct {
	Kind EventKind
	X, Y int32
	Slot int8
}

// Input is one player's controls for one tick.
type Input struct {
	Mask uint8 // bits: 0 moveR, 1 moveL, 2 moveD, 3 moveU, 4 fireR, 5 fireL, 6 fireD, 7 fireU
	Fast bool
}

// Input mask bits.
const (
	MoveR uint8 = 1 << iota
	MoveL
	MoveD
	MoveU
	FireR
	FireL
	FireD
	FireU
)

// Timing constants (ticks).
const (
	FireCooldown  = 4
	RespawnDelay  = 36
	DebrisLife    = 18
	SnipeMoveRate = 2
)

// State is the complete authoritative game.
type State struct {
	Tick        uint32
	Rng         Rand
	Cfg         Config
	Maze        Maze
	Tiles       []uint8 // GridWidth*GridHeight
	Ents        []Entity
	NextID      uint32
	Players     [MaxPlayers]Player
	Score       int32
	HivesAlive  int32
	SnipesAlive int32 // large and small
	Phase       Phase
	Events      []Event

	occ []int32 // tile → entity index + 1; derived, rebuilt after each tick
}

// NewGame creates a game: maze, players in distinct cells, hives in cells
// at least three cells away from every player.
func NewGame(cfg Config, seed uint32) *State {
	s := &State{Rng: NewRand(seed), Cfg: cfg, Tiles: make([]uint8, GridWidth*GridHeight), NextID: 1}
	s.Maze = GenerateMaze(&s.Rng)
	s.Maze.Rasterize(s.Tiles)
	s.rebuildOcc()

	var used [NumCells]bool
	var playerCells []int32
	for slot := 0; slot < int(cfg.Players); slot++ {
		c := s.pickCell(&used, playerCells, 0)
		playerCells = append(playerCells, c)
		x, y := cellCentre(c)
		s.Players[slot] = Player{Joined: true, Lives: int32(cfg.Lives), SpawnX: x, SpawnY: y}
		s.spawn(Entity{Kind: KindPlayer, X: x, Y: y, Owner: int8(slot)})
	}
	for i := 0; i < int(cfg.Hives); i++ {
		x, y := cellCentre(s.pickCell(&used, playerCells, 3))
		s.spawn(Entity{Kind: KindHive, X: x, Y: y, Owner: -1, Timer: 1})
		s.HivesAlive++
	}
	return s
}

// cellCentre is where a 2 × 2 sprite sits inside a cell's 7 × 5 interior.
func cellCentre(c int32) (int32, int32) {
	return (c%CellsX)*CellWidth + 3, (c/CellsX)*CellHeight + 2
}

// pickCell draws an unused cell at least minDist cells (Chebyshev, wrapped)
// from every cell in away, relaxing the distance if no such cell turns up.
func (s *State) pickCell(used *[NumCells]bool, away []int32, minDist int32) int32 {
	for tries := 0; ; tries++ {
		c := int32(s.Rng.Int(NumCells))
		if used[c] {
			continue
		}
		ok := true
		for _, a := range away {
			dx := abs32(WrapDelta(a%CellsX, c%CellsX, CellsX))
			dy := abs32(WrapDelta(a/CellsX, c/CellsX, CellsY))
			if max(dx, dy) < minDist-int32(tries/200) {
				ok = false
				break
			}
		}
		if ok {
			used[c] = true
			return c
		}
	}
}

// spawn appends an entity, stamps its footprint and returns its index.
func (s *State) spawn(e Entity) int {
	e.ID = s.NextID
	s.NextID++
	s.Ents = append(s.Ents, e)
	i := len(s.Ents) - 1
	s.stamp(i, int32(i+1))
	return i
}

// stamp writes v into every tile of entity i's footprint (debris never stamps).
func (s *State) stamp(i int, v int32) {
	e := &s.Ents[i]
	if e.Kind == KindDebris {
		return
	}
	w, h := e.Kind.Size()
	for dy := int32(0); dy < h; dy++ {
		for dx := int32(0); dx < w; dx++ {
			s.occ[wrapY(e.Y+dy)*GridWidth+wrapX(e.X+dx)] = v
		}
	}
}

func (s *State) rebuildOcc() {
	if s.occ == nil {
		s.occ = make([]int32, GridWidth*GridHeight)
	}
	for i := range s.occ {
		s.occ[i] = 0
	}
	for i := range s.Ents {
		if !s.Ents[i].Dead {
			s.stamp(i, int32(i+1))
		}
	}
}

// Wall reports whether tile (x, y) is a wall (coordinates wrap).
func (s *State) Wall(x, y int32) bool { return s.Tiles[wrapY(y)*GridWidth+wrapX(x)] == TileWall }

// At returns the index of the live entity covering tile (x, y), or -1.
// Debris is never returned.
func (s *State) At(x, y int32) int { return int(s.occ[wrapY(y)*GridWidth+wrapX(x)]) - 1 }

// PlayerEntity returns the index of slot's body, or -1 while dead or out.
func (s *State) PlayerEntity(slot int) int {
	for i := range s.Ents {
		e := &s.Ents[i]
		if e.Kind == KindPlayer && int(e.Owner) == slot && !e.Dead {
			return i
		}
	}
	return -1
}

func (s *State) emit(k EventKind, x, y int32, slot int8) {
	s.Events = append(s.Events, Event{Kind: k, X: x, Y: y, Slot: slot})
}
