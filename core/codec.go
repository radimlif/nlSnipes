package core

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
)

const codecVersion = 1

type enc []byte

func (b *enc) u8(v uint8)   { *b = append(*b, v) }
func (b *enc) u32(v uint32) { *b = binary.LittleEndian.AppendUint32(*b, v) }
func (b *enc) i32(v int32)  { b.u32(uint32(v)) }
func (b *enc) bool(v bool) {
	if v {
		b.u8(1)
	} else {
		b.u8(0)
	}
}

type dec struct {
	b   []byte
	err error
}

func (d *dec) u8() uint8 {
	if len(d.b) < 1 {
		d.err = errors.New("core: state truncated")
		return 0
	}
	v := d.b[0]
	d.b = d.b[1:]
	return v
}

func (d *dec) u32() uint32 {
	if len(d.b) < 4 {
		d.err = errors.New("core: state truncated")
		return 0
	}
	v := binary.LittleEndian.Uint32(d.b)
	d.b = d.b[4:]
	return v
}

func (d *dec) i32() int32 { return int32(d.u32()) }
func (d *dec) bool() bool { return d.u8() != 0 }

// MarshalBinary is the canonical encoding of everything that determines the
// game's future: identical states encode to identical bytes. Events and the
// occupancy index are derived and not included.
func (s *State) MarshalBinary() ([]byte, error) {
	b := make(enc, 0, 512+len(s.Tiles)+len(s.Ents)*24)
	b.u8(codecVersion)
	b.u32(s.Tick)
	b.u32(s.Rng.A)
	b.u32(s.Rng.B)
	b.u32(s.Rng.C)
	b.u32(s.Rng.D)
	b.u8(s.Cfg.Letter)
	b.u8(s.Cfg.Digit)
	b.u8(s.Cfg.Players)
	b.bool(s.Cfg.FriendlyFire)
	b = append(b, s.Maze[:]...)
	b = append(b, s.Tiles...)
	b.u32(s.NextID)
	b.u32(uint32(len(s.Ents)))
	for _, e := range s.Ents {
		b.u32(e.ID)
		b.u8(uint8(e.Kind))
		b.i32(e.X)
		b.i32(e.Y)
		b.u8(e.Dir)
		b.u8(uint8(e.Turn))
		b.u8(uint8(e.Owner))
		b.u8(e.Bounces)
		b.i32(e.Timer)
		b.bool(e.Dead)
	}
	for _, p := range s.Players {
		b.bool(p.Joined)
		b.i32(p.Lives)
		b.i32(p.Score)
		b.i32(p.Respawn)
		b.i32(p.Cooldown)
		b.i32(p.SpawnX)
		b.i32(p.SpawnY)
	}
	b.i32(s.Score)
	b.i32(s.HivesAlive)
	b.i32(s.SnipesAlive)
	b.u8(uint8(s.Phase))
	return b, nil
}

// UnmarshalBinary restores a state written by MarshalBinary.
func (s *State) UnmarshalBinary(data []byte) error {
	d := &dec{b: data}
	if v := d.u8(); v != codecVersion {
		return errors.New("core: unknown state version")
	}
	n := State{}
	n.Tick = d.u32()
	n.Rng = Rand{d.u32(), d.u32(), d.u32(), d.u32()}
	letter, digit, players, ff := d.u8(), d.u8(), d.u8(), d.bool()
	if d.err != nil {
		return d.err
	}
	if letter > 25 || digit < 1 || digit > 9 || players < 1 || players > MaxPlayers {
		return errors.New("core: invalid config in state")
	}
	n.Cfg = configFor(letter, digit, players, ff)
	if len(d.b) < NumCells+GridWidth*GridHeight {
		return errors.New("core: state truncated")
	}
	copy(n.Maze[:], d.b)
	n.Tiles = append([]uint8(nil), d.b[NumCells:NumCells+GridWidth*GridHeight]...)
	d.b = d.b[NumCells+GridWidth*GridHeight:]
	n.NextID = d.u32()
	count := d.u32()
	if count > uint32(len(d.b))/24 {
		return errors.New("core: entity count exceeds data")
	}
	n.Ents = make([]Entity, count)
	for i := range n.Ents {
		e := &n.Ents[i]
		e.ID = d.u32()
		e.Kind = Kind(d.u8())
		e.X, e.Y = d.i32(), d.i32()
		e.Dir = d.u8()
		e.Turn = int8(d.u8())
		e.Owner = int8(d.u8())
		e.Bounces = d.u8()
		e.Timer = d.i32()
		e.Dead = d.bool()
		if e.Kind < KindPlayer || e.Kind > KindDebris || e.X < 0 || e.X >= GridWidth || e.Y < 0 || e.Y >= GridHeight ||
			e.Owner < -1 || e.Owner >= MaxPlayers || (e.Kind == KindPlayer && e.Owner < 0) {
			return errors.New("core: invalid entity in state")
		}
	}
	for i := range n.Players {
		p := &n.Players[i]
		p.Joined = d.bool()
		p.Lives, p.Score, p.Respawn, p.Cooldown = d.i32(), d.i32(), d.i32(), d.i32()
		p.SpawnX, p.SpawnY = d.i32(), d.i32()
	}
	n.Score, n.HivesAlive, n.SnipesAlive = d.i32(), d.i32(), d.i32()
	n.Phase = Phase(d.u8())
	if d.err != nil {
		return d.err
	}
	if len(d.b) != 0 {
		return errors.New("core: trailing bytes after state")
	}
	n.rebuildOcc()
	*s = n
	return nil
}

// Hash is FNV-1a 32 over the canonical encoding.
func (s *State) Hash() uint32 {
	b, _ := s.MarshalBinary()
	h := fnv.New32a()
	h.Write(b)
	return h.Sum32()
}

// Clone returns an independent deep copy.
func (s *State) Clone() *State {
	c := *s
	c.Tiles = append([]uint8(nil), s.Tiles...)
	c.Ents = append([]Entity(nil), s.Ents...)
	c.Events = append([]Event(nil), s.Events...)
	c.occ = append([]int32(nil), s.occ...)
	return &c
}
