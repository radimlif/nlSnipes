package lan

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/radimlif/nlSnipes/core"
)

// Timing, in ticks unless noted.
const (
	SnapshotEvery = 2 * core.TicksPerSecond // full state to every client
	ClientTimeout = 3 * time.Second         // silence before a client counts as gone
	maxQueued     = 2                       // input frames buffered per player
	resyncGap     = 2                       // ticks between resync requests, and between snapshots sent on request to one client
)

// HostConfig configures a host.
type HostConfig struct {
	Listen       string // e.g. ":5108" or "127.0.0.1:0"; "" = first free of Port..Port+PortRange-1
	GameID       uint32
	Nick         string
	FriendlyFire bool
	AllowMirror  bool // IDDQD for everyone; otherwise only while one player is in the game
	Loss         float64
	Now          func() time.Time
}

// peer is someone connected to the host (the host itself is slot 0, peer nil).
type peer struct {
	id        uint32
	addr      netip.AddrPort
	nick      string
	slot      int8
	frames    []Frame
	lastSeq   uint32
	last      Frame
	lastHeard time.Time
	lastSnap  uint32 // host tick of the last snapshot sent on request
}

// Host runs the authoritative game and streams it to clients.
type Host struct {
	n   *node
	cfg HostConfig

	skill string
	state *core.State
	epoch uint32

	seats   [core.MaxPlayers]*peer // nil = free, or the host itself in slot 0 (hostSeat)
	peers   map[uint32]*peer
	queue   []*peer // spectators waiting for a slot, oldest first
	join    [core.MaxPlayers]bool
	leave   [core.MaxPlayers]bool
	records []Record
	local   Frame
	// rejoin lists slots emptied by reseat this tick; their new player
	// joins on the next tick, after the leave has been applied.
	rejoin []int
}

// hostSeat marks slot 0 as the host's own.
var hostSeat = &peer{slot: 0}

// NewHost opens the host socket. The game starts with Start.
func NewHost(cfg HostConfig) (*Host, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	var n *node
	var err error
	if cfg.Listen != "" {
		n, err = newNode(cfg.Listen, cfg.Loss, 1)
	} else {
		for p := Port; p < Port+PortRange; p++ {
			if n, err = newNode(fmt.Sprintf(":%d", p), cfg.Loss, 1); err == nil {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}
	h := &Host{n: n, cfg: cfg, peers: map[uint32]*peer{}}
	h.seats[0] = hostSeat // the host always plays in slot 0, lobby included
	return h, nil
}

// Addr is the host's socket address.
func (h *Host) Addr() netip.AddrPort { return h.n.addr() }

// State is the running game, nil before Start.
func (h *Host) State() *core.State { return h.state }

// Epoch counts mazes played.
func (h *Host) Epoch() uint32 { return h.epoch }

// Start begins a game (or the next maze) with the given skill and seed.
// Everyone seated keeps their slot; the host sits in slot 0.
func (h *Host) Start(skill string, seed uint32) error {
	cfg, err := core.NewConfig(skill, 1, h.cfg.FriendlyFire)
	if err != nil {
		return err
	}
	h.skill = skill
	h.state = core.NewGame(cfg, seed)
	h.epoch++
	h.records = nil
	h.seats[0], h.rejoin = hostSeat, nil
	for slot := 1; slot < core.MaxPlayers; slot++ {
		h.join[slot], h.leave[slot] = false, false
		if h.seats[slot] == nil && len(h.queue) > 0 {
			h.seat(h.queue[0], int8(slot))
			h.queue = h.queue[1:]
		}
		h.join[slot] = h.seats[slot] != nil
	}
	h.broadcastSnapshot()
	h.sendRoster()
	return nil
}

func (h *Host) seat(p *peer, slot int8) {
	p.slot, p.frames, p.last = slot, nil, Frame{}
	h.seats[slot] = p
	h.n.send(p.addr, Welcome{ClientID: p.id, Slot: slot})
}

// Poll handles everything received: discovery, joins, inputs, goodbyes.
func (h *Host) Poll() {
	now := h.cfg.Now()
	for _, pk := range h.n.poll() {
		m, err := Decode(pk.data)
		if err != nil {
			continue
		}
		switch m := m.(type) {
		case Probe:
			if m.GameID == h.cfg.GameID {
				h.n.send(pk.from, h.beacon())
			}
		case Join:
			if m.GameID == h.cfg.GameID {
				h.admit(m, pk.from, now)
			}
		case Input:
			p := h.peers[m.ClientID]
			if p == nil || p.addr != pk.from {
				continue
			}
			p.lastHeard = now
			for _, f := range m.Frames {
				if f.Seq > p.lastSeq {
					p.lastSeq = f.Seq
					p.frames = append(p.frames, f)
				}
			}
			if len(p.frames) > maxQueued {
				p.frames = p.frames[len(p.frames)-maxQueued:]
			}
		case Bye:
			if p := h.peers[m.ClientID]; p != nil && p.addr == pk.from {
				h.drop(p)
			}
		case Resync:
			if p := h.peers[m.ClientID]; p != nil && p.addr == pk.from && h.state != nil {
				p.lastHeard = now
				if t := h.state.Tick; t-p.lastSnap >= resyncGap || t < p.lastSnap {
					p.lastSnap = t
					for _, s := range snapshotParts(h.epoch, h.state) {
						h.n.send(p.addr, s)
					}
				}
			}
		}
	}
	for _, p := range h.peers {
		if now.Sub(p.lastHeard) > ClientTimeout {
			h.drop(p)
		}
	}
}

func (h *Host) beacon() Beacon {
	b := Beacon{GameID: h.cfg.GameID, Skill: h.skill, Spectators: uint8(len(h.queue))}
	if h.state != nil {
		b.Tick = h.state.Tick
	}
	for _, s := range h.seats {
		if s != nil {
			b.Players++
		}
	}
	return b
}

func (h *Host) admit(m Join, from netip.AddrPort, now time.Time) {
	p := h.peers[m.ClientID]
	if p == nil {
		p = &peer{id: m.ClientID, addr: from, nick: m.Nick, slot: Spectator}
		h.peers[p.id] = p
		free := -1
		for slot := 1; slot < core.MaxPlayers; slot++ {
			if h.seats[slot] == nil {
				free = slot
				break
			}
		}
		if free > 0 && h.state != nil {
			h.seat(p, int8(free))
			h.joinSoon(free)
		} else if free > 0 {
			h.seat(p, int8(free)) // lobby: joins when the game starts
		} else {
			h.queue = append(h.queue, p)
		}
		h.sendRoster()
	}
	p.lastHeard = now
	h.n.send(p.addr, Welcome{ClientID: p.id, Slot: p.slot})
	if h.state != nil {
		for _, s := range snapshotParts(h.epoch, h.state) {
			h.n.send(p.addr, s)
		}
	}
}

func (h *Host) drop(p *peer) {
	delete(h.peers, p.id)
	if p.slot >= 0 && h.seats[p.slot] == p {
		slot := int(p.slot)
		h.seats[slot] = nil
		h.join[slot] = false
		if h.state != nil {
			h.leave[slot] = true
			if len(h.queue) > 0 && h.queue[0] != p {
				next := h.queue[0]
				h.queue = h.queue[1:]
				h.seat(next, int8(slot))
				h.joinSoon(slot)
			}
		}
	}
	for i, q := range h.queue {
		if q == p {
			h.queue = append(h.queue[:i], h.queue[i+1:]...)
			break
		}
	}
	h.sendRoster()
}

// Step advances the game one tick with the host's own controls, then sends
// the tick to every client (and a full snapshot every SnapshotEvery ticks).
func (h *Host) Step(local Frame) {
	if h.state == nil {
		return
	}
	h.local = local
	s := h.state
	for _, slot := range h.rejoin {
		h.join[slot] = h.seats[slot] != nil
	}
	h.rejoin = h.rejoin[:0]
	h.reseat()

	var in [core.MaxPlayers]core.Input
	players := 0
	for _, p := range h.seats {
		if p != nil {
			players++
		}
	}
	for slot, p := range h.seats {
		var f Frame
		switch {
		case p == hostSeat:
			f = h.local
		case p != nil:
			if len(p.frames) > 0 {
				p.last, p.frames = p.frames[0], p.frames[1:]
			}
			f = p.last
		}
		mirror := f.Mirror && (h.cfg.AllowMirror || players <= 1)
		in[slot] = core.Input{Mask: f.Mask, Fast: f.Fast,
			ToggleMirror: s.Players[slot].Joined && mirror != s.Players[slot].Mirror,
			Join:         h.join[slot], Leave: h.leave[slot]}
		h.join[slot], h.leave[slot] = false, false
	}
	t := s.Tick
	s.Step(in)
	h.records = append(h.records, Record{Tick: t, Inputs: in, Hash: s.Hash()})
	if len(h.records) > MaxTickRecords {
		h.records = h.records[len(h.records)-MaxTickRecords:]
	}
	msg := Tick{Epoch: h.epoch, Records: h.records}
	for _, p := range h.peers {
		h.n.send(p.addr, msg)
	}
	if s.Tick%SnapshotEvery == 0 {
		h.broadcastSnapshot()
	}
}

// joinSoon seats the slot's new player this tick, or next tick if the
// slot's previous player is leaving this tick.
func (h *Host) joinSoon(slot int) {
	if h.leave[slot] {
		h.rejoin = append(h.rejoin, slot)
	} else {
		h.join[slot] = true
	}
}

// reseat gives the slot of a client who is out of lives to the
// longest-waiting spectator, who joins next tick; the player goes to the
// back of the queue. The host keeps slot 0 for the whole maze.
func (h *Host) reseat() {
	s := h.state
	for slot := 1; slot < core.MaxPlayers && len(h.queue) > 0; slot++ {
		p := h.seats[slot]
		if p == nil || !s.Players[slot].Joined || s.Players[slot].Lives > 0 || h.join[slot] || h.leave[slot] {
			continue
		}
		next := h.queue[0]
		h.queue = append(h.queue[1:], p)
		p.slot = Spectator
		h.n.send(p.addr, Welcome{ClientID: p.id, Slot: Spectator})
		h.leave[slot] = true
		h.seat(next, int8(slot))
		h.rejoin = append(h.rejoin, slot)
		h.sendRoster()
	}
}

func (h *Host) broadcastSnapshot() {
	parts := snapshotParts(h.epoch, h.state)
	for _, p := range h.peers {
		for _, s := range parts {
			h.n.send(p.addr, s)
		}
	}
}

func (h *Host) sendRoster() {
	r := h.Roster()
	for _, p := range h.peers {
		h.n.send(p.addr, r)
	}
}

// Roster lists the host, seated players and spectators.
func (h *Host) Roster() Roster {
	var r Roster
	for slot, p := range h.seats {
		switch {
		case p == hostSeat:
			r.Seats = append(r.Seats, Seat{Slot: int8(slot), Nick: h.cfg.Nick})
		case p != nil:
			r.Seats = append(r.Seats, Seat{Slot: int8(slot), ClientID: p.id, Nick: p.nick})
		}
	}
	for _, p := range h.queue {
		r.Seats = append(r.Seats, Seat{Slot: Spectator, ClientID: p.id, Nick: p.nick})
	}
	return r
}

// Close says goodbye to everyone and closes the socket.
func (h *Host) Close() {
	for _, p := range h.peers {
		h.n.send(p.addr, Bye{})
	}
	h.n.close()
}
