// Command nlsnipes is the game: launch it on any machine on the LAN to host
// or join. Usage: nlsnipes [skill code, e.g. M5].
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/radimlif/nlSnipes/app"
)

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version {
		fmt.Println("nlsnipes", app.Version)
		return
	}
	fmt.Fprintf(os.Stderr, "nlsnipes %s: not playable yet (scaffold, milestone L0)\n", app.Version)
	os.Exit(1)
}
