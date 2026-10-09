// Package lan is NLSNIPES's LAN protocol over UDP: discovery by broadcast
// probe, then the host sends every client the game as a full snapshot plus
// a stream of per-tick inputs, from which each client steps its own copy of
// the deterministic core and checks the host's state hash every tick.
// See docs/DESIGN-LIGHT.md ("Network protocol") and docs/decisions/0008.
//
// Named lan rather than net so it can use the standard library's net
// without an alias (docs/decisions/0002).
package lan

import (
	"encoding/binary"
	"errors"
	"net/netip"

	"github.com/radimlif/nlSnipes/core"
)

// Port is the first UDP port a host listens on; a busy port moves the host
// up to Port+PortRange-1, and probes go to all of them.
const (
	Port      = 5108
	PortRange = 8
)

// Magic prefixes every datagram; Version is the protocol version.
const (
	Magic   = "NLS1"
	Version = 1
)

// MaxDatagram keeps every packet under typical LAN MTUs.
const MaxDatagram = 1200

// Message types.
const (
	TProbe    uint8 = 1
	TBeacon   uint8 = 2
	TJoin     uint8 = 3
	TWelcome  uint8 = 4
	TInput    uint8 = 5
	TSnapshot uint8 = 6
	TTick     uint8 = 7
	TBye      uint8 = 8
	TRoster   uint8 = 9
	TResync   uint8 = 10
)

// Limits.
const (
	MaxNick        = 12
	MaxInputFrames = 4  // client → host: last frames, so a lost packet costs nothing
	MaxTickRecords = 8  // host → client: last ticks, ditto
	MaxRoster      = 24 // players + spectators listed
	Spectator      = -1 // slot of a spectator
)

// Probe asks "is a game running?". Sent to the broadcast address.
type Probe struct {
	GameID   uint32
	ClientID uint32
}

// Beacon answers a probe, unicast to the prober.
type Beacon struct {
	GameID     uint32
	Skill      string // e.g. "M5"; "" while the host is still choosing
	Tick       uint32
	Players    uint8
	Spectators uint8
	Port       uint16 // the game's port when the beacon comes from a discovery-only socket; 0 = the sender's
}

// Join asks for a seat.
type Join struct {
	GameID   uint32
	ClientID uint32
	Nick     string
}

// Welcome tells a client its slot (0..3) or Spectator. Sent again whenever
// it changes (a spectator gets a free slot).
type Welcome struct {
	ClientID uint32
	Slot     int8
}

// Frame is one tick of a player's controls.
type Frame struct {
	Seq    uint32 // client's own frame counter
	Mask   uint8
	Fast   bool
	Mirror bool // player wants mirror shots (IDDQD state, not a toggle)
}

// Input carries a player's last frames; spectators send it empty as a keep-alive.
type Input struct {
	ClientID uint32
	Frames   []Frame
}

// Snapshot is one part of a full game state (flate-compressed core encoding).
type Snapshot struct {
	Epoch uint32 // maze number: a new maze is a new epoch
	Tick  uint32
	Hash  uint32
	Part  uint8
	Parts uint8
	Data  []byte
}

// Record is what the host fed into one Step and the hash it got.
type Record struct {
	Tick   uint32 // state tick the inputs were applied to
	Inputs [core.MaxPlayers]core.Input
	Hash   uint32 // state hash after the step
}

// Tick carries the latest records, oldest first.
type Tick struct {
	Epoch   uint32
	Records []Record
}

// Bye says a client or host is leaving.
type Bye struct {
	ClientID uint32
}

// Resync asks the host for a full snapshot now: the client has no state,
// is in an old maze, or missed more ticks than a Tick packet repeats.
type Resync struct {
	ClientID uint32
}

// Seat is one roster line. Addr is where the host reaches that client (so
// everyone can find a successor host); zero for the host's own seat.
type Seat struct {
	Slot     int8
	ClientID uint32
	Nick     string
	Addr     netip.AddrPort
}

// Roster lists who is in the game, players by slot then spectators in
// queue order. StartIn counts down the seconds until a host waiting on its
// title screen starts the game anyway (0 = no countdown).
type Roster struct {
	HostID  uint32 // ClientID of the host's own seat
	Seats   []Seat
	StartIn uint8
}

// ErrBadPacket is returned for anything that is not a valid message.
var ErrBadPacket = errors.New("lan: bad packet")

type writer []byte

func (w *writer) u8(v uint8)   { *w = append(*w, v) }
func (w *writer) u32(v uint32) { *w = binary.LittleEndian.AppendUint32(*w, v) }
func (w *writer) u16(v uint16) { *w = binary.LittleEndian.AppendUint16(*w, v) }

// addr writes an IPv4 address and port (6 bytes; zero for none).
func (w *writer) addr(a netip.AddrPort) {
	ip := [4]byte{}
	if a.Addr().Is4() {
		ip = a.Addr().As4()
	}
	*w = append(*w, ip[:]...)
	w.u16(a.Port())
}

func (w *writer) str(s string, max int) {
	if len(s) > max {
		s = s[:max]
	}
	w.u8(uint8(len(s)))
	*w = append(*w, s...)
}

type reader struct {
	b   []byte
	bad bool
}

func (r *reader) u8() uint8 {
	if len(r.b) < 1 {
		r.bad = true
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *reader) u32() uint32 {
	if len(r.b) < 4 {
		r.bad = true
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *reader) u16() uint16 {
	if len(r.b) < 2 {
		r.bad = true
		return 0
	}
	v := binary.LittleEndian.Uint16(r.b)
	r.b = r.b[2:]
	return v
}

func (r *reader) addr() netip.AddrPort {
	if len(r.b) < 6 {
		r.bad = true
		return netip.AddrPort{}
	}
	ip := [4]byte(r.b[:4])
	r.b = r.b[4:]
	port := r.u16()
	if ip == [4]byte{} && port == 0 {
		return netip.AddrPort{}
	}
	return netip.AddrPortFrom(netip.AddrFrom4(ip), port)
}

func (r *reader) str(max int) string {
	n := int(r.u8())
	if n > max || n > len(r.b) {
		r.bad = true
		return ""
	}
	s := string(r.b[:n])
	r.b = r.b[n:]
	return s
}

func header(t uint8) writer { return append(writer(Magic), t) }

func inputFlags(in core.Input) uint8 {
	var f uint8
	if in.Fast {
		f |= 1
	}
	if in.ToggleMirror {
		f |= 2
	}
	if in.Join {
		f |= 4
	}
	if in.Leave {
		f |= 8
	}
	return f
}

// Encode serialises a message. It panics on a type it does not know.
func Encode(m any) []byte {
	var w writer
	switch m := m.(type) {
	case Probe:
		w = header(TProbe)
		w.u8(Version)
		w.u32(m.GameID)
		w.u32(m.ClientID)
	case Beacon:
		w = header(TBeacon)
		w.u8(Version)
		w.u32(m.GameID)
		w.str(m.Skill, 2)
		w.u32(m.Tick)
		w.u8(m.Players)
		w.u8(m.Spectators)
		w.u16(m.Port)
	case Join:
		w = header(TJoin)
		w.u8(Version)
		w.u32(m.GameID)
		w.u32(m.ClientID)
		w.str(m.Nick, MaxNick)
	case Welcome:
		w = header(TWelcome)
		w.u32(m.ClientID)
		w.u8(uint8(m.Slot))
	case Input:
		w = header(TInput)
		w.u32(m.ClientID)
		fr := m.Frames
		if len(fr) > MaxInputFrames {
			fr = fr[len(fr)-MaxInputFrames:]
		}
		w.u8(uint8(len(fr)))
		for _, f := range fr {
			w.u32(f.Seq)
			w.u8(f.Mask)
			var flags uint8
			if f.Fast {
				flags |= 1
			}
			if f.Mirror {
				flags |= 2
			}
			w.u8(flags)
		}
	case Snapshot:
		w = header(TSnapshot)
		w.u32(m.Epoch)
		w.u32(m.Tick)
		w.u32(m.Hash)
		w.u8(m.Part)
		w.u8(m.Parts)
		w = append(w, m.Data...)
	case Tick:
		w = header(TTick)
		w.u32(m.Epoch)
		rs := m.Records
		if len(rs) > MaxTickRecords {
			rs = rs[len(rs)-MaxTickRecords:]
		}
		w.u8(uint8(len(rs)))
		for _, r := range rs {
			w.u32(r.Tick)
			for _, in := range r.Inputs {
				w.u8(in.Mask)
				w.u8(inputFlags(in))
			}
			w.u32(r.Hash)
		}
	case Bye:
		w = header(TBye)
		w.u32(m.ClientID)
	case Resync:
		w = header(TResync)
		w.u32(m.ClientID)
	case Roster:
		w = header(TRoster)
		w.u32(m.HostID)
		w.u8(m.StartIn)
		seats := m.Seats
		if len(seats) > MaxRoster {
			seats = seats[:MaxRoster]
		}
		w.u8(uint8(len(seats)))
		for _, s := range seats {
			w.u8(uint8(s.Slot))
			w.u32(s.ClientID)
			w.str(s.Nick, MaxNick)
			w.addr(s.Addr)
		}
	default:
		panic("lan: cannot encode message")
	}
	return w
}

// Decode parses a datagram into one of the message types (by value).
func Decode(b []byte) (any, error) {
	if len(b) < len(Magic)+1 || string(b[:len(Magic)]) != Magic {
		return nil, ErrBadPacket
	}
	r := &reader{b: b[len(Magic)+1:]}
	var m any
	switch b[len(Magic)] {
	case TProbe:
		if r.u8() != Version {
			return nil, ErrBadPacket
		}
		m = Probe{GameID: r.u32(), ClientID: r.u32()}
	case TBeacon:
		if r.u8() != Version {
			return nil, ErrBadPacket
		}
		var v Beacon
		v.GameID = r.u32()
		v.Skill = r.str(2)
		v.Tick = r.u32()
		v.Players = r.u8()
		v.Spectators = r.u8()
		v.Port = r.u16()
		m = v
	case TJoin:
		if r.u8() != Version {
			return nil, ErrBadPacket
		}
		m = Join{GameID: r.u32(), ClientID: r.u32(), Nick: r.str(MaxNick)}
	case TWelcome:
		v := Welcome{ClientID: r.u32(), Slot: int8(r.u8())}
		if v.Slot < Spectator || v.Slot >= core.MaxPlayers {
			return nil, ErrBadPacket
		}
		m = v
	case TInput:
		v := Input{ClientID: r.u32()}
		n := int(r.u8())
		if n > MaxInputFrames {
			return nil, ErrBadPacket
		}
		for i := 0; i < n; i++ {
			f := Frame{Seq: r.u32(), Mask: r.u8()}
			flags := r.u8()
			f.Fast, f.Mirror = flags&1 != 0, flags&2 != 0
			v.Frames = append(v.Frames, f)
		}
		m = v
	case TSnapshot:
		v := Snapshot{Epoch: r.u32(), Tick: r.u32(), Hash: r.u32(), Part: r.u8(), Parts: r.u8()}
		if v.Parts == 0 || v.Part >= v.Parts {
			return nil, ErrBadPacket
		}
		v.Data = append([]byte(nil), r.b...)
		r.b = nil
		m = v
	case TTick:
		v := Tick{Epoch: r.u32()}
		n := int(r.u8())
		if n > MaxTickRecords {
			return nil, ErrBadPacket
		}
		for i := 0; i < n; i++ {
			rec := Record{Tick: r.u32()}
			for p := range rec.Inputs {
				rec.Inputs[p].Mask = r.u8()
				f := r.u8()
				rec.Inputs[p].Fast = f&1 != 0
				rec.Inputs[p].ToggleMirror = f&2 != 0
				rec.Inputs[p].Join = f&4 != 0
				rec.Inputs[p].Leave = f&8 != 0
			}
			rec.Hash = r.u32()
			v.Records = append(v.Records, rec)
		}
		m = v
	case TBye:
		m = Bye{ClientID: r.u32()}
	case TResync:
		m = Resync{ClientID: r.u32()}
	case TRoster:
		var v Roster
		v.HostID = r.u32()
		v.StartIn = r.u8()
		n := int(r.u8())
		if n > MaxRoster {
			return nil, ErrBadPacket
		}
		for i := 0; i < n; i++ {
			s := Seat{Slot: int8(r.u8()), ClientID: r.u32(), Nick: r.str(MaxNick), Addr: r.addr()}
			if s.Slot < Spectator || s.Slot >= core.MaxPlayers {
				return nil, ErrBadPacket
			}
			v.Seats = append(v.Seats, s)
		}
		m = v
	default:
		return nil, ErrBadPacket
	}
	if r.bad || len(r.b) != 0 {
		return nil, ErrBadPacket
	}
	return m, nil
}
