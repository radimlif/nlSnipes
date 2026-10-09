// Command nlsnipes is the game. Launch it on any machine on the LAN: if a
// game is running there you join it, otherwise you host and the game starts.
// Usage: nlsnipes [flags] [skill code, e.g. M5]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/radimlif/nlSnipes/app"
	"github.com/radimlif/nlSnipes/term"
)

func main() {
	version := flag.Bool("version", false, "print version and exit")
	seed := flag.Uint("seed", 0, "maze seed (0 = random)")
	classic := flag.Bool("classic", false, "start in the original 40 x 25 view (V toggles in game)")
	nick := flag.String("nick", app.DefaultNick(), "your name as other players see it (12 characters)")
	game := flag.String("game", "", "game name: separate games on one network use different names")
	host := flag.String("host", "", "join the host at this address (IP or IP:port) instead of searching")
	noFF := flag.Bool("no-friendly-fire", false, "when hosting: players' bullets pass through each other")
	iddqd := flag.Bool("iddqd", false, "when hosting: allow IDDQD mirror shots even with several players")
	offline := flag.Bool("offline", false, "play alone without looking for or offering a LAN game")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: nlsnipes [flags] [skill code A1-Z9]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Println("nlsnipes", app.Version)
		return
	}
	opt := app.Options{
		Skill: flag.Arg(0), Seed: uint32(*seed), ScoreFile: app.DefaultScorePath(), Classic: *classic,
		Nick: *nick, GameName: *game, HostAddr: *host, FriendlyFire: !*noFF, AllowMirror: *iddqd, Offline: *offline,
	}

	t, err := term.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nlsnipes: this needs an interactive terminal:", err)
		os.Exit(1)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Close()
			panic(r)
		}
	}()
	g, err := app.Launch(t, opt)
	if err == nil {
		err = app.Run(t, g)
	}
	t.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nlsnipes:", err)
		os.Exit(1)
	}
}
