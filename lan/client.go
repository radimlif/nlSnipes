package lan

import (
	"fmt"
	"hash/fnv"
	"net/netip"
	"time"

	"github.com/radimlif/nlSnipes/core"
)

// GameID turns a --game name into the id beacons carry ("" = the default game).
func GameID(name string) uint32 {
	h := fnv.New32a()
	h.Write([]byte("nlsnipes:" + name))
	return h.Sum32()
}

// Found is a host that answered a probe.
type Found struct {
	Addr   netip.AddrPort
	Beacon Beacon
}

// Discover probes for a host of game id: three probes 300 ms apart to the
// broadcast address and loopback on every port of the range, or only to
// target if one is given (--host). It returns the first host to answer
// within timeout.
func Discover(gameID, clientID uint32, target string, timeout time.Duration) (Found, bool, error) {
	n, err := newNode(":0", 0, clientID)
	if err != nil {
		return Found{}, false, err
	}
	defer n.close()
	var dests []netip.AddrPort
	if target != "" {
		ap, err := parseHost(target)
		if err != nil {
			return Found{}, false, err
		}
		dests = []netip.AddrPort{ap}
	} else {
		for p := Port; p < Port+PortRange; p++ {
			dests = append(dests,
				netip.AddrPortFrom(netip.AddrFrom4([4]byte{255, 255, 255, 255}), uint16(p)),
				netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), uint16(p)))
		}
	}
	deadline := time.Now().Add(timeout)
	nextProbe := time.Now()
	for time.Now().Before(deadline) {
		if time.Now().After(nextProbe) {
			for _, d := range dests {
				n.send(d, Probe{GameID: gameID, ClientID: clientID})
			}
			nextProbe = time.Now().Add(300 * time.Millisecond)
		}
		for _, pk := range n.wait(50 * time.Millisecond) {
			if m, err := Decode(pk.data); err == nil {
				if b, ok := m.(Beacon); ok && b.GameID == gameID {
					addr := pk.from
					if b.Port != 0 { // answered by a migrated host's discovery socket
						addr = netip.AddrPortFrom(addr.Addr(), b.Port)
					}
					return Found{Addr: addr, Beacon: b}, true, nil
				}
			}
		}
	}
	return Found{}, false, nil
}

func parseHost(s string) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap, nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("--host %q: want an IPv4 address, optionally with :port", s)
	}
	return netip.AddrPortFrom(a, Port), nil
}

// ClientConfig configures a client.
type ClientConfig struct {
	Host     netip.AddrPort
	GameID   uint32
	ClientID uint32
	Nick     string
	Listen   string // default ":0"
	Loss     float64
	Now      func() time.Time
}

// ClientStats count how well the client tracked the host.
type ClientStats struct {
	SnapshotsMatched uint64 // snapshot arrived when we were at its tick, and hashes agreed
	SnapshotsDiffer  uint64 // ... and hashes disagreed (a determinism bug)
	SnapshotsAdopted uint64 // snapshot replaced our state (first, behind, new maze, or after a desync)
	Desyncs          uint64 // a tick's hash did not match ours
	TicksApplied     uint64
	Resyncs          uint64 // snapshots asked for
}

// Client follows a host's game: it keeps its own copy of the state, steps
// it with the host's per-tick inputs and checks every hash.
type Client struct {
	n      *node
	cfg    ClientConfig
	slot   int8
	state  *core.State
	epoch  uint32
	asm    assembler
	roster Roster
	frames []Frame
	seq    uint32
	joined bool
	gone   bool
	quiet  int // ticks since the last keep-alive
	behind int // ticks we have known we are out of step
	events []core.Event

	// host migration
	hostID    uint32          // id of the host we follow
	lastHeard time.Time       // last packet from the host
	bye       bool            // the host said goodbye
	tried     map[uint32]bool // hosts that failed us
	adoptNext bool            // take the next snapshot whatever its tick (new host)
	promoted  bool            // this client became the host; its socket belongs to the host now

	Stats ClientStats
}

// NewClient opens a socket and asks the host for a seat.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Listen == "" {
		cfg.Listen = ":0"
	}
	n, err := newNode(cfg.Listen, cfg.Loss, cfg.ClientID^0x5EED)
	if err != nil {
		return nil, err
	}
	c := &Client{n: n, cfg: cfg, slot: Spectator, tried: map[uint32]bool{}, lastHeard: cfg.Now()}
	c.sendJoin()
	return c, nil
}

func (c *Client) sendJoin() {
	c.n.send(c.cfg.Host, Join{GameID: c.cfg.GameID, ClientID: c.cfg.ClientID, Nick: c.cfg.Nick})
}

// ID is the client's id.
func (c *Client) ID() uint32 { return c.cfg.ClientID }

// Slot is the client's player slot, or Spectator.
func (c *Client) Slot() int8 { return c.slot }

// Joined reports whether the host has answered.
func (c *Client) Joined() bool { return c.joined }

// HostGone reports that the host said goodbye.
func (c *Client) HostGone() bool { return c.gone }

// State is the client's copy of the game, nil until the first snapshot.
func (c *Client) State() *core.State { return c.state }

// Epoch is the maze number the state belongs to.
func (c *Client) Epoch() uint32 { return c.epoch }

// Roster is the host's latest roster.
func (c *Client) Roster() Roster { return c.roster }

// Poll handles everything received from the host.
func (c *Client) Poll() { c.handle(c.n.poll()) }

// Wait blocks until something arrives (or d passes) and handles it.
func (c *Client) Wait(d time.Duration) { c.handle(c.n.wait(d)) }

func (c *Client) handle(pks []packet) {
	for _, pk := range pks {
		if pk.from != c.cfg.Host {
			continue
		}
		c.lastHeard = c.cfg.Now()
		m, err := Decode(pk.data)
		if err != nil {
			continue
		}
		switch m := m.(type) {
		case Welcome:
			if m.ClientID == c.cfg.ClientID {
				c.joined, c.slot = true, m.Slot
			}
		case Roster:
			c.roster = m
			c.hostID = m.HostID
		case Snapshot:
			if s, ok := c.asm.add(m); ok {
				c.snapshot(m.Epoch, s)
			}
		case Tick:
			c.tick(m)
		case Bye:
			c.gone, c.bye = true, true
		}
	}
}

func (c *Client) snapshot(epoch uint32, s *core.State) {
	if c.adoptNext { // first snapshot from a new host: take it as it is
		c.adoptNext = false
		c.state, c.epoch = s, epoch
		c.Stats.SnapshotsAdopted++
		return
	}
	if c.state != nil && epoch == c.epoch {
		switch {
		case c.state.Tick == s.Tick:
			if c.state.Hash() == s.Hash() {
				c.Stats.SnapshotsMatched++
				return
			}
			c.Stats.SnapshotsDiffer++
		case c.state.Tick > s.Tick:
			return // we are already ahead of it
		}
	}
	if epoch < c.epoch {
		return
	}
	c.state, c.epoch = s, epoch
	c.Stats.SnapshotsAdopted++
}

func (c *Client) tick(m Tick) {
	if c.adoptNext {
		return // wait for the new host's snapshot
	}
	if c.state == nil || m.Epoch != c.epoch || len(m.Records) == 0 {
		c.lost(true)
		return
	}
	if m.Records[0].Tick > c.state.Tick {
		c.lost(true) // a gap longer than the repeated records
		return
	}
	c.lost(false)
	for _, r := range m.Records {
		if r.Tick != c.state.Tick {
			continue
		}
		c.state.Step(r.Inputs)
		c.events = append(c.events, c.state.Events...)
		c.Stats.TicksApplied++
		if c.state.Hash() != r.Hash {
			c.Stats.Desyncs++
			c.state = nil // ask for a fresh snapshot
			c.lost(true)
			return
		}
	}
}

// lost tracks whether we are out of step, and asks the host for a snapshot
// at once, then every few ticks until one arrives.
func (c *Client) lost(behind bool) {
	if !behind {
		c.behind = 0
		return
	}
	if c.behind%resyncGap == 0 {
		c.n.send(c.cfg.Host, Resync{ClientID: c.cfg.ClientID})
		c.Stats.Resyncs++
	}
	c.behind++
}

// TakeEvents returns the game events of every tick applied since the last
// call (a Poll can apply several).
func (c *Client) TakeEvents() []core.Event {
	ev := c.events
	c.events = nil
	return ev
}

// Send reports this tick's controls (players) or a periodic keep-alive
// (spectators, and players before the game starts). Call once per tick.
func (c *Client) Send(f Frame) {
	if c.promoted {
		return
	}
	if !c.joined || c.adoptNext {
		if c.quiet++; c.quiet%6 == 0 {
			c.sendJoin() // the join or its welcome was lost: ask again
		}
		return
	}
	if c.slot == Spectator || c.state == nil {
		if c.quiet++; c.quiet%core.TicksPerSecond == 0 {
			c.n.send(c.cfg.Host, Input{ClientID: c.cfg.ClientID})
		}
		return
	}
	c.seq++
	f.Seq = c.seq
	c.frames = append(c.frames, f)
	if len(c.frames) > MaxInputFrames {
		c.frames = c.frames[len(c.frames)-MaxInputFrames:]
	}
	c.n.send(c.cfg.Host, Input{ClientID: c.cfg.ClientID, Frames: c.frames})
}

// Close tells the host we are leaving.
func (c *Client) Close() {
	if c.promoted {
		return
	}
	c.n.send(c.cfg.Host, Bye{ClientID: c.cfg.ClientID})
	c.n.close()
}

// HostTimeout is how long clients wait in silence before deciding the host is gone.
const HostTimeout = 2 * time.Second

// Migration says what a client should do about its host.
type Migration uint8

// Migration steps.
const (
	MigrateNone       Migration = iota // the host is fine
	MigrateRetarget                    // the host is gone; now following Seat
	MigrateBecomeHost                  // the host is gone and we are next: call Promote
	MigrateGiveUp                      // the host is gone and nobody can take over
)

// Migrate checks the host. When it has said goodbye or been silent for
// HostTimeout, the next host is the remaining player with the lowest slot
// in the last roster (spectators never host); if that is us we promote,
// otherwise we follow it. A successor that stays silent is skipped in turn.
func (c *Client) Migrate(now time.Time) (Migration, Seat) {
	if c.promoted || !c.joined || (!c.bye && now.Sub(c.lastHeard) <= HostTimeout) {
		return MigrateNone, Seat{}
	}
	c.tried[c.hostID] = true
	if c.state == nil {
		return MigrateGiveUp, Seat{}
	}
	var next *Seat
	for i := range c.roster.Seats {
		s := &c.roster.Seats[i]
		if s.Slot < 0 || c.tried[s.ClientID] || s.ClientID == 0 {
			continue
		}
		if next == nil || s.Slot < next.Slot {
			next = s
		}
	}
	if next == nil {
		return MigrateGiveUp, Seat{}
	}
	if next.ClientID == c.cfg.ClientID {
		return MigrateBecomeHost, *next
	}
	c.cfg.Host = c.reach(next.Addr)
	c.hostID, c.lastHeard, c.bye, c.gone, c.adoptNext = next.ClientID, now, false, false, true
	c.sendJoin()
	return MigrateRetarget, *next
}

// reach turns an address the old host saw into one we can use: a peer the
// old host saw on loopback ran on the old host's machine.
func (c *Client) reach(a netip.AddrPort) netip.AddrPort {
	if a.Addr().IsLoopback() && !c.cfg.Host.Addr().IsLoopback() {
		return netip.AddrPortFrom(c.cfg.Host.Addr(), a.Port())
	}
	return a
}

// Promote turns this client into the host, on the same socket, continuing
// from its own copy of the game: everyone keeps their slot, spectators
// their place in the queue; the players of failed hosts leave. The client
// must not be used afterwards.
func (c *Client) Promote(cfg HostConfig) *Host {
	cfg.GameID, cfg.ClientID = c.cfg.GameID, c.cfg.ClientID
	if cfg.Nick == "" {
		cfg.Nick = c.cfg.Nick
	}
	if cfg.Now == nil {
		cfg.Now = c.cfg.Now
	}
	h := &Host{n: c.n, cfg: cfg, peers: map[uint32]*peer{}, state: c.state, epoch: c.epoch, self: c.slot}
	h.skill = c.state.Cfg.Skill()
	h.seats[c.slot] = hostSeat
	now := cfg.Now()
	for _, s := range c.roster.Seats {
		switch {
		case s.ClientID == c.cfg.ClientID:
			continue
		case c.tried[s.ClientID]:
			if s.Slot >= 0 && c.state.Players[s.Slot].Joined {
				h.leave[s.Slot] = true
			}
			continue
		}
		p := &peer{id: s.ClientID, addr: c.reach(s.Addr), nick: s.Nick, slot: s.Slot, lastHeard: now}
		h.peers[p.id] = p
		if s.Slot >= 0 {
			h.seats[s.Slot] = p
		} else {
			h.queue = append(h.queue, p)
		}
	}
	for slot := range h.seats { // hand the leavers' slots to waiting spectators
		if h.leave[slot] && h.seats[slot] == nil && len(h.queue) > 0 {
			next := h.queue[0]
			h.queue = h.queue[1:]
			h.seat(next, int8(slot))
			h.rejoin = append(h.rejoin, slot)
		}
	}
	h.disc, _ = listenRange(0) // newcomers probe the well-known ports
	c.promoted = true
	h.broadcastSnapshot()
	h.sendRoster()
	return h
}
