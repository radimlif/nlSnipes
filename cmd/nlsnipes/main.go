// Command nlsnipes is the game. Usage: nlsnipes [skill code, e.g. M5].
// Without a skill code it asks on the title screen.
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
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: nlsnipes [flags] [skill code A1-Z9]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Println("nlsnipes", app.Version)
		return
	}
	opt := app.Options{Skill: flag.Arg(0), Seed: uint32(*seed), ScoreFile: app.DefaultScorePath(), Classic: *classic}

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
	err = app.Run(t, opt)
	t.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nlsnipes:", err)
		os.Exit(1)
	}
}
