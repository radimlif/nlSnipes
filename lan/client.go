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
					return Found{Addr: pk.from, Beacon: b}, true, nil
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
	Stats  ClientStats
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
	c := &Client{n: n, cfg: cfg, slot: Spectator}
	c.sendJoin()
	return c, nil
}

func (c *Client) sendJoin() {
	c.n.send(c.cfg.Host, Join{GameID: c.cfg.GameID, ClientID: c.cfg.ClientID, Nick: c.cfg.Nick})
}

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
		case Snapshot:
			if s, ok := c.asm.add(m); ok {
				c.snapshot(m.Epoch, s)
			}
		case Tick:
			c.tick(m)
		case Bye:
			c.gone = true
		}
	}
}

func (c *Client) snapshot(epoch uint32, s *core.State) {
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

// Send reports this tick's controls (players) or a periodic keep-alive
// (spectators, and players before the game starts). Call once per tick.
func (c *Client) Send(f Frame) {
	if !c.joined {
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
	c.n.send(c.cfg.Host, Bye{ClientID: c.cfg.ClientID})
	c.n.close()
}
