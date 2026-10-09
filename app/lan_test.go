package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radimlif/nlSnipes/core"
	"github.com/radimlif/nlSnipes/lan"
	"github.com/radimlif/nlSnipes/term"
)

type lanNode struct {
	g   *Game
	sim *term.Sim
}

func lanSetup(t *testing.T, clients int) (*lanNode, []*lanNode) {
	t.Helper()
	opt := Options{Skill: "A1", Seed: 5, Nick: "host", FriendlyFire: true, Listen: "127.0.0.1:0"}
	hs := term.NewSim(100, 40, true)
	hg, err := NewHost(hs, opt)
	if err != nil {
		t.Fatal(err)
	}
	host := &lanNode{hg, hs}
	var cs []*lanNode
	for i := 0; i < clients; i++ {
		s := term.NewSim(100, 40, true)
		copt := Options{Nick: fmt.Sprintf("guest%d", i), Listen: "127.0.0.1:0"}
		cg, err := NewClient(s, copt, lan.Found{Addr: hg.host.Addr()}, uint32(1000+i))
		if err != nil {
			t.Fatal(err)
		}
		cs = append(cs, &lanNode{cg, s})
	}
	t.Cleanup(func() {
		for _, c := range cs {
			c.g.Close()
		}
		hg.Close()
	})
	return host, cs
}

func lanTicks(host *lanNode, cs []*lanNode, n int) {
	for i := 0; i < n; i++ {
		host.g.Tick()
		time.Sleep(2 * time.Millisecond)
		for _, c := range cs {
			c.g.Tick()
		}
		time.Sleep(time.Millisecond)
	}
}

func TestClientJoinsAndPlays(t *testing.T) {
	host, cs := lanSetup(t, 1)
	c := cs[0]
	lanTicks(host, cs, 20)
	if c.g.screen != screenPlay || c.g.Slot() != 1 {
		t.Fatalf("client screen %d slot %d, want playing in slot 1", c.g.screen, c.g.Slot())
	}
	hs := host.g.State()
	if !hs.Players[1].Joined || hs.PlayerEntity(1) < 0 {
		t.Fatal("guest not in the host's game")
	}
	if c.g.State().Hash() != hs.Hash() && c.g.State().Tick == hs.Tick {
		t.Fatal("client state differs from host")
	}
	// The guest presses Down; the host's copy of the guest moves.
	i := hs.PlayerEntity(1)
	y0 := hs.Ents[i].Y
	c.g.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyDown})
	lanTicks(host, cs, 4)
	c.g.HandleEvent(term.Event{Kind: term.EvRelease, Key: term.KeyDown})
	lanTicks(host, cs, 2)
	if y := hs.Ents[hs.PlayerEntity(1)].Y; y == y0 {
		t.Fatal("guest's key press did not move the guest on the host")
	}
	if find(host.g.Frame(), "guest0") == "" {
		t.Fatal("host roster does not show the guest")
	}
}

func TestFifthPlayerSpectates(t *testing.T) {
	host, cs := lanSetup(t, 4)
	lanTicks(host, cs, 30)
	spec := cs[3]
	if spec.g.Slot() != lan.Spectator {
		t.Fatalf("4th guest has slot %d, want spectator", spec.g.Slot())
	}
	if find(spec.g.Frame(), "SPECTATING") == "" {
		t.Fatal("spectator HUD not shown")
	}
	before := spec.g.watch
	spec.g.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyTab})
	if spec.g.watch == before {
		t.Fatal("Tab did not switch the watched player")
	}
	joined := 0
	for _, p := range host.g.State().Players {
		if p.Joined {
			joined++
		}
	}
	if joined != core.MaxPlayers {
		t.Fatalf("%d players in the host's game", joined)
	}
}

func TestClientSeesHostLeave(t *testing.T) {
	host, cs := lanSetup(t, 1)
	lanTicks(host, cs, 10)
	host.g.Close()
	time.Sleep(5 * time.Millisecond)
	cs[0].g.Tick()
	if cs[0].g.screen != screenHostLeft {
		t.Fatalf("client screen %d after the host left", cs[0].g.screen)
	}
	if !strings.Contains(strings.Join(lines(cs[0].g.Frame()), "\n"), "host left") {
		t.Fatal("no host-left message")
	}
}

func lines(f *term.Frame) []string {
	var out []string
	for y := 0; y < f.H; y++ {
		out = append(out, f.Line(y))
	}
	return out
}

func TestLobbyShowsWhoWeWaitFor(t *testing.T) {
	opt := Options{Seed: 5, Nick: "radim", FriendlyFire: true, Listen: "127.0.0.1:0"} // no skill: host is on the title
	hs := term.NewSim(100, 40, true)
	hg, err := NewHost(hs, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer hg.Close()
	cs := term.NewSim(100, 40, true)
	cg, err := NewClient(cs, Options{Nick: "guest", Listen: "127.0.0.1:0"}, lan.Found{Addr: hg.host.Addr()}, 55)
	if err != nil {
		t.Fatal(err)
	}
	defer cg.Close()
	host, guests := &lanNode{hg, hs}, []*lanNode{{cg, cs}}
	lanTicks(host, guests, 10)
	if find(cg.Frame(), "waiting for radim") == "" || find(cg.Frame(), "guest  (you)") == "" {
		t.Fatalf("lobby screen:\n%s", strings.Join(lines(cg.Frame()), "\n"))
	}
	if find(hg.Frame(), "1 player waiting - press Enter to start") == "" {
		t.Fatal("host title does not say a player is waiting")
	}
	if find(cg.Frame(), "starts by itself in") == "" {
		t.Fatal("guest does not see the countdown")
	}
	hg.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyEnter})
	lanTicks(host, guests, 10)
	if cg.screen != screenPlay {
		t.Fatal("guest did not start playing when the host started")
	}
}

func TestLobbyCountdownStartsTheGame(t *testing.T) {
	opt := Options{Seed: 5, Nick: "radim", FriendlyFire: true, Listen: "127.0.0.1:0"}
	hs := term.NewSim(100, 40, true)
	hg, _ := NewHost(hs, opt)
	defer hg.Close()
	cs := term.NewSim(100, 40, true)
	cg, _ := NewClient(cs, Options{Nick: "guest", Listen: "127.0.0.1:0"}, lan.Found{Addr: hg.host.Addr()}, 56)
	defer cg.Close()
	host, guests := &lanNode{hg, hs}, []*lanNode{{cg, cs}}
	hg.HandleEvent(term.Event{Kind: term.EvPress, Key: term.KeyRune, Rune: 'm'}) // incomplete code
	lanTicks(host, guests, lobbyTicks+10)
	if hg.State() == nil || hg.State().Cfg.Skill() != "A1" || cg.screen != screenPlay {
		t.Fatal("countdown did not start an A1 game for both")
	}
}
