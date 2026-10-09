package lan

import (
	"fmt"
	"testing"
	"time"

	"github.com/radimlif/nlSnipes/bots"
	"github.com/radimlif/nlSnipes/core"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// Gate L4: kill the host mid-game (no goodbye); the remaining clients
// resume within 3 s with the same roster and continue to identical hashes.
func TestGateHostMigration(t *testing.T) {
	for _, crash := range []bool{true, false} {
		name := "host quits"
		if crash {
			name = "host crashes"
		}
		t.Run(name, func(t *testing.T) { migrationRun(t, crash) })
	}
}

func migrationRun(t *testing.T, crash bool) {
	clk := &fakeClock{t: time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC)}
	host, err := NewHost(HostConfig{Listen: "127.0.0.1:0", GameID: 9, ClientID: 1, Nick: "host", FriendlyFire: true, Now: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	host.Start("M5", 11)
	var clients []*Client
	cBots := map[*Client]*bots.Hunter{}
	for i := 0; i < 4; i++ { // 3 players + 1 spectator
		c, err := NewClient(ClientConfig{Host: host.Addr(), GameID: 9, ClientID: uint32(200 + i),
			Nick: fmt.Sprintf("p%d", i), Listen: "127.0.0.1:0", Now: clk.now})
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
		cBots[c] = bots.NewHunter()
	}
	hostBot := bots.NewHunter()
	hashes := map[uint32]uint32{}
	cur := host
	defer func() {
		for _, c := range clients {
			c.Close()
		}
		cur.Close()
	}()

	step := func(alive bool) {
		clk.t = clk.t.Add(time.Second / core.TicksPerSecond)
		if alive {
			cur.Poll()
			if s := cur.State(); s.Phase != core.PhaseRunning {
				cur.Start("M5", s.Tick+7)
				hashes = map[uint32]uint32{0: cur.State().Hash()}
			}
			cur.Step(frameFor(hostBot, cur.State(), cur.Self()))
			hashes[cur.State().Tick] = cur.State().Hash()
		}
		for _, c := range clients {
			c.Poll()
			for i := 0; i < 20 && alive && (c.State() == nil || c.State().Tick < cur.State().Tick); i++ {
				c.Wait(time.Millisecond)
			}
			c.Send(frameFor(cBots[c], c.State(), c.Slot()))
		}
	}
	inSync := func(c *Client) bool {
		s := c.State()
		return s != nil && !c.adoptNext && cur.State().Tick-s.Tick <= 1 && hashes[s.Tick] == s.Hash()
	}

	for i := 0; i < 300; i++ {
		step(true)
	}
	before := map[uint32]int8{}
	for _, c := range clients {
		before[c.cfg.ClientID] = c.Slot()
		if !inSync(c) {
			t.Fatalf("client %d not in sync before the host goes", c.cfg.ClientID)
		}
	}
	oldSlot := host.Self()
	if crash {
		host.Crash()
	} else {
		host.Close()
	}
	gone := clk.t

	// Run the clients until one promotes itself and everyone follows it.
	var promoted *Client
	resumed := time.Duration(0)
	for k := 0; k < 10*core.TicksPerSecond && resumed == 0; k++ {
		step(promoted != nil)
		for i, c := range clients {
			switch m, seat := c.Migrate(clk.now()); m {
			case MigrateBecomeHost:
				promoted = c
				cur = c.Promote(HostConfig{FriendlyFire: true, Now: clk.now})
				clients = append(clients[:i:i], clients[i+1:]...)
				hostBot = cBots[c]
				hashes = map[uint32]uint32{cur.State().Tick: cur.State().Hash()}
				t.Logf("%v after the host left: %s (slot %d) is the new host", clk.t.Sub(gone), seat.Nick, seat.Slot)
			case MigrateRetarget:
				t.Logf("%v: client %d follows %s", clk.t.Sub(gone), c.cfg.ClientID, seat.Nick)
			case MigrateGiveUp:
				t.Fatalf("client %d gave up", c.cfg.ClientID)
			}
		}
		if promoted != nil {
			all := true
			for _, c := range clients {
				all = all && inSync(c)
			}
			if all {
				resumed = clk.t.Sub(gone)
			}
		}
	}
	if resumed == 0 || resumed > 3*time.Second {
		t.Fatalf("clients did not resume within 3 s (resumed after %v)", resumed)
	}
	t.Logf("all clients in sync with the new host %v after the old host left", resumed)

	// Same roster: everyone keeps their slot; the new host plays in its own.
	if want := before[promoted.cfg.ClientID]; cur.Self() != want || want < 0 {
		t.Errorf("new host plays in slot %d, had %d", cur.Self(), want)
	}
	for _, c := range clients {
		if c.Slot() != before[c.cfg.ClientID] && before[c.cfg.ClientID] >= 0 {
			t.Errorf("client %d moved from slot %d to %d", c.cfg.ClientID, before[c.cfg.ClientID], c.Slot())
		}
	}
	// The spectator who was waiting gets the old host's slot (checked right
	// after the handover: at the next maze the host moves back to slot 0).
	for i := 0; i < 3; i++ {
		step(true)
	}
	for _, c := range clients {
		if before[c.cfg.ClientID] == Spectator && c.Slot() != oldSlot {
			t.Errorf("waiting spectator got slot %d, want the old host's slot %d", c.Slot(), oldSlot)
		}
	}
	if !cur.State().Players[oldSlot].Joined {
		t.Errorf("old host's slot %d is empty though a spectator was waiting", oldSlot)
	}
	for i := 0; i < 600; i++ {
		step(true)
	}
	for _, c := range clients {
		if !inSync(c) || c.Stats.Desyncs != 0 || c.Stats.SnapshotsDiffer != 0 {
			t.Errorf("client %d not hash-identical after the migration", c.cfg.ClientID)
		}
	}
}
