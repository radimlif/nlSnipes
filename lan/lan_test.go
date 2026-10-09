package lan

import (
	"fmt"
	"math/rand/v2"
	"net"
	"testing"
	"time"

	"github.com/radimlif/nlSnipes/bots"
	"github.com/radimlif/nlSnipes/core"
)

// lanGame is a host and its clients in one process, on loopback UDP.
type lanGame struct {
	t       *testing.T
	host    *Host
	clients []*Client
	hBot    *bots.Hunter
	cBots   []*bots.Hunter
	hashes  map[uint32]uint32 // host hash by tick, current epoch
	epoch   uint32
	seed    uint32
	skill   string
	wait    time.Duration
}

func newLANGame(t *testing.T, skill string, players, spectators int, loss float64) *lanGame {
	t.Helper()
	h, err := NewHost(HostConfig{Listen: "127.0.0.1:0", GameID: 7, Nick: "host", FriendlyFire: true, Loss: loss})
	if err != nil {
		t.Fatal(err)
	}
	g := &lanGame{t: t, host: h, hBot: bots.NewHunter(), hashes: map[uint32]uint32{}, seed: 42, skill: skill, wait: 30 * time.Millisecond}
	if err := h.Start(skill, g.seed); err != nil {
		t.Fatal(err)
	}
	g.epoch = h.Epoch()
	g.hashes[0] = h.State().Hash()
	for i := 0; i < players+spectators; i++ {
		c, err := NewClient(ClientConfig{Host: h.Addr(), GameID: 7, ClientID: uint32(100 + i),
			Nick: fmt.Sprintf("bot%d", i), Listen: "127.0.0.1:0", Loss: loss})
		if err != nil {
			t.Fatal(err)
		}
		g.clients = append(g.clients, c)
		g.cBots = append(g.cBots, bots.NewHunter())
	}
	t.Cleanup(func() {
		for _, c := range g.clients {
			c.Close()
		}
		h.Close()
	})
	return g
}

// frameFor asks a hunter bot for a slot's input on a given state.
func frameFor(b *bots.Hunter, s *core.State, slot int8) Frame {
	if s == nil || slot < 0 || s.PlayerEntity(int(slot)) < 0 {
		return Frame{}
	}
	in := b.Input(s, int(slot))
	return Frame{Mask: in.Mask, Fast: in.Fast}
}

// tick runs one game tick on every node and lets the network deliver.
func (g *lanGame) tick() {
	h := g.host
	h.Poll()
	if h.State().Phase != core.PhaseRunning { // next maze, as the app does
		g.seed++
		h.Start(g.skill, g.seed)
		g.epoch = h.Epoch()
		g.hashes = map[uint32]uint32{0: h.State().Hash()}
		g.hBot = bots.NewHunter()
		for i := range g.cBots {
			g.cBots[i] = bots.NewHunter()
		}
	}
	h.Step(frameFor(g.hBot, h.State(), 0))
	g.hashes[h.State().Tick] = h.State().Hash()
	target := h.State().Tick
	deadline := time.Now().Add(g.wait)
	for i, c := range g.clients {
		c.Poll()
		for (c.State() == nil || c.Epoch() != g.epoch || c.State().Tick < target) && time.Now().Before(deadline) {
			c.Wait(time.Millisecond)
		}
		c.Send(frameFor(g.cBots[i], c.State(), c.Slot()))
	}
}

// inSync reports whether client c is at most lag ticks behind the host in
// the current maze with an identical state at its tick.
func (g *lanGame) inSync(c *Client, lag uint32) bool {
	s := c.State()
	if s == nil || c.Epoch() != g.epoch || g.host.State().Tick-s.Tick > lag {
		return false
	}
	return g.hashes[s.Tick] == s.Hash()
}

// Gate L3: integration test on loopback — 1 host + 3 players + 2 spectators
// for 3 600 ticks; every client's hash equals the host's at each full
// snapshot.
func TestGateLoopbackGame(t *testing.T) {
	g := newLANGame(t, "M5", 3, 2, 0)
	for i := 0; i < 3600; i++ {
		g.tick()
	}
	slots, specs := 0, 0
	for i, c := range g.clients {
		st := c.Stats
		t.Logf("client %d slot %d: %d ticks applied, snapshots matched %d differ %d adopted %d, desyncs %d",
			i, c.Slot(), st.TicksApplied, st.SnapshotsMatched, st.SnapshotsDiffer, st.SnapshotsAdopted, st.Desyncs)
		if st.SnapshotsDiffer != 0 || st.Desyncs != 0 {
			t.Errorf("client %d diverged from the host", i)
		}
		if st.SnapshotsMatched+uint64(g.host.Epoch())*2+5 < 3600/SnapshotEvery {
			t.Errorf("client %d checked only %d snapshots", i, st.SnapshotsMatched)
		}
		if !g.inSync(c, 0) {
			t.Errorf("client %d is not at the host's tick with the host's state", i)
		}
		if c.Slot() == Spectator {
			specs++
		} else {
			slots++
		}
	}
	if slots != 3 || specs != 2 {
		t.Errorf("%d players and %d spectators, want 3 and 2", slots, specs)
	}
	joined := 0
	for _, p := range g.host.State().Players {
		if p.Joined {
			joined++
		}
	}
	t.Logf("%d mazes played, %d players in the last", g.host.Epoch(), joined)
	if joined != 4 {
		t.Errorf("%d players in the game, want 4", joined)
	}
}

// Gate L3: 20 % simulated loss in every direction still converges within
// one snapshot period. Every client is checked every tick; no client may be
// out of sync (behind by more than 2 ticks, in an old maze, or with a
// different state) for SnapshotEvery ticks or longer in a row.
func TestGateConvergesUnderLoss(t *testing.T) {
	g := newLANGame(t, "A1", 3, 2, 0.20)
	g.wait = 8 * time.Millisecond
	const ticks = 30 * SnapshotEvery
	out := make([]int, len(g.clients))   // current out-of-sync run, in ticks
	worst := make([]int, len(g.clients)) // longest run seen
	for i := 1; i <= ticks; i++ {
		g.tick()
		for k, c := range g.clients {
			if g.inSync(c, 2) {
				out[k] = 0
				continue
			}
			out[k]++
			worst[k] = max(worst[k], out[k])
		}
	}
	for k, c := range g.clients {
		st := c.Stats
		t.Logf("client %d: longest out of sync %d ticks; desyncs %d, resyncs asked %d, snapshots adopted %d, matched %d",
			k, worst[k], st.Desyncs, st.Resyncs, st.SnapshotsAdopted, st.SnapshotsMatched)
		if st.SnapshotsDiffer != 0 || st.Desyncs != 0 {
			t.Errorf("client %d diverged from the host", k)
		}
		if worst[k] >= SnapshotEvery {
			t.Errorf("client %d was out of sync for %d ticks (limit %d)", k, worst[k], SnapshotEvery-1)
		}
	}
	t.Logf("%d mazes in %d ticks", g.host.Epoch(), ticks)
}

// Gate L3: a packet fuzzer never crashes host or client. Garbage, truncated
// and mutated packets are fired at every socket during play; the game must
// carry on in sync.
func TestGateFuzzedPacketsDuringPlay(t *testing.T) {
	g := newLANGame(t, "D3", 2, 1, 0)
	attacker, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer attacker.Close()
	r := rand.New(rand.NewPCG(1, 2))
	valid := sampleMessages()
	targets := []*net.UDPAddr{net.UDPAddrFromAddrPort(g.host.Addr())}
	for _, c := range g.clients {
		targets = append(targets, net.UDPAddrFromAddrPort(c.n.addr()))
	}
	for i := 0; i < 1200; i++ {
		for k := 0; k < 5; k++ {
			var b []byte
			switch r.IntN(3) {
			case 0: // random bytes behind a valid header
				b = append([]byte(Magic), byte(r.IntN(12)))
				for n := r.IntN(64); n > 0; n-- {
					b = append(b, byte(r.UintN(256)))
				}
			case 1: // valid message, bytes flipped
				b = Encode(valid[r.IntN(len(valid))])
				for n := 1 + r.IntN(3); n > 0; n-- {
					b[r.IntN(len(b))] ^= byte(1 + r.IntN(255))
				}
			default: // valid message, truncated
				b = Encode(valid[r.IntN(len(valid))])
				b = b[:r.IntN(len(b)+1)]
			}
			attacker.WriteToUDP(b, targets[r.IntN(len(targets))])
		}
		g.tick()
	}
	for i, c := range g.clients {
		if !g.inSync(c, 0) || c.Stats.Desyncs != 0 {
			t.Errorf("client %d lost sync under fuzzing", i)
		}
	}
}

// A player who leaves while a spectator waits hands over the slot: the
// spectator must actually enter the game, not just be told its slot.
func TestSpectatorTakesLeaversSlot(t *testing.T) {
	g := newLANGame(t, "A1", 3, 1, 0)
	for i := 0; i < 30; i++ {
		g.tick()
	}
	var leaver, spec *Client
	for _, c := range g.clients {
		switch {
		case c.Slot() == Spectator:
			spec = c
		case leaver == nil:
			leaver = c
		}
	}
	slot := leaver.Slot()
	leaver.Close()
	for i, c := range g.clients {
		if c == leaver {
			g.clients = append(g.clients[:i], g.clients[i+1:]...)
			g.cBots = append(g.cBots[:i], g.cBots[i+1:]...)
			break
		}
	}
	for i := 0; i < 10; i++ {
		g.tick()
	}
	if spec.Slot() != slot {
		t.Fatalf("spectator has slot %d, want the leaver's %d", spec.Slot(), slot)
	}
	if s := g.host.State(); !s.Players[slot].Joined || s.PlayerEntity(int(slot)) < 0 {
		t.Fatal("spectator was seated but never entered the game")
	}
}
