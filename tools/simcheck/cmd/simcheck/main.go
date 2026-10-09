// Command simcheck fails when any Go source in the repository shares a
// 40-token window with the vendored reference port (see docs/decisions/0003).
//
//	go run ./tools/simcheck/cmd/simcheck -ref reference -root .
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/radimlif/nlSnipes/tools/simcheck"
)

func main() {
	ref := flag.String("ref", "reference", "reference source tree (git submodule)")
	root := flag.String("root", ".", "repository root to scan")
	k := flag.Int("k", simcheck.DefaultOptions.K, "window length in tokens")
	minIdents := flag.Int("min-idents", simcheck.DefaultOptions.MinIdents, "skip windows with fewer identifiers")
	flag.Parse()

	opt := simcheck.Options{K: *k, MinIdents: *minIdents}
	ix, err := simcheck.IndexDir(*ref, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simcheck:", err)
		os.Exit(2)
	}
	if ix.Len() == 0 {
		fmt.Fprintf(os.Stderr, "simcheck: no fingerprints in %s — is the submodule checked out? (git submodule update --init)\n", *ref)
		os.Exit(2)
	}
	matches, err := simcheck.CheckTree(*root, *ref, ix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simcheck:", err)
		os.Exit(2)
	}
	for _, m := range matches {
		fmt.Printf("%s:%d-%d matches %s:%d\n", m.File, m.StartLine, m.EndLine, m.RefFile, m.RefLine)
	}
	if len(matches) > 0 {
		fmt.Fprintf(os.Stderr, "simcheck: %d copied region(s) found; rewrite them from the behaviour, not the code\n", len(matches))
		os.Exit(1)
	}
	fmt.Printf("simcheck: OK (%d reference fingerprints, k=%d)\n", ix.Len(), opt.K)
}
