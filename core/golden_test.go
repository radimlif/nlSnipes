package core

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/golden.txt (explain why in the PR)")

// TestGoldenHashes pins the simulation: any rule change, or any platform
// that computes differently, changes these hashes. CI builds on Linux; the
// same file must pass on every OS and architecture.
func TestGoldenHashes(t *testing.T) {
	got := map[string]uint32{}
	var names []string
	for _, skill := range []string{"A1", "M5", "Z9"} {
		for _, players := range []int{1, 4} {
			cfg, _ := NewConfig(skill, players, true)
			name := fmt.Sprintf("new/%s/p%d/seed1", skill, players)
			got[name] = NewGame(cfg, 1).Hash()
			names = append(names, name)
			name = fmt.Sprintf("random/%s/p%d/seed1/2000", skill, players)
			got[name] = randomGame(cfg, 1, 2000, nil).Hash()
			names = append(names, name)
		}
	}
	path := filepath.Join("testdata", "golden.txt")
	if *update {
		var sb strings.Builder
		sb.WriteString("# name hash — regenerate with: go test ./core -run Golden -update\n")
		for _, n := range names {
			fmt.Fprintf(&sb, "%s %08x\n", n, got[n])
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	defer f.Close()
	want := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" && !strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			want[parts[0]] = parts[1]
		}
	}
	for _, n := range names {
		if g := fmt.Sprintf("%08x", got[n]); want[n] != g {
			t.Errorf("%s: hash %s, golden %s", n, g, want[n])
		}
	}
}
