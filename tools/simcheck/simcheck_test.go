package simcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A small C++-flavoured "reference" written for this test (not from the port).
const fakeRef = `
// spawn logic
static void SpawnFromHive(Hive *hive, World *world) {
    if (hive->timer > 0) { hive->timer--; return; }
    int dir = world->rng.Next() & 7;
    Snipe *s = AllocSnipe(world);
    if (!s) return;
    s->x = hive->x + offsetX[dir];
    s->y = hive->y + offsetY[dir];
    s->heading = dir;
    hive->timer = BaseDelay(world, hive) + DistancePenalty(world, hive);
    world->snipeCount++;
}
`

// The same logic transliterated to Go: identical token stream after
// normalisation, which is what a lazy port of the reference would look like.
const plantedGo = `package core

func spawnFromHive(hive *Hive, world *World) {
	if (hive->timer > 0) { hive->timer--; return; }
	int dir = world->rng.Next() & 7;
	Snipe *s = AllocSnipe(world);
	if (!s) return;
	s->x = hive->x + offsetX[dir];
	s->y = hive->y + offsetY[dir];
	s->heading = dir;
	hive->timer = BaseDelay(world, hive) + DistancePenalty(world, hive);
	world->snipeCount++;
}
`

const originalGo = `package core

// Step advances the hive's countdown and reports whether it should spawn.
func (h *Hive) Step(r *Rand) (spawn bool, heading uint8) {
	if h.Cooldown > 0 {
		h.Cooldown--
		return false, 0
	}
	h.Cooldown = h.Delay
	return true, uint8(r.Uint32() % 8)
}
`

func TestTokenizeNormalises(t *testing.T) {
	a := Tokenize("x = 0x7F; // c\ny = \"hi\"")
	b := Tokenize("X = 127 /* other */\nY = 'yo'")
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Text != b[i].Text {
			t.Errorf("token %d: %q vs %q", i, a[i].Text, b[i].Text)
		}
	}
	if a[len(a)-1].Line != 2 {
		t.Errorf("line tracking: got %d", a[len(a)-1].Line)
	}
}

func TestCatchesPlantedCopy(t *testing.T) {
	ix := NewIndex(DefaultOptions)
	ix.Add("ref/Snipes.cpp", fakeRef)
	m := ix.Check("core/spawn.go", plantedGo)
	if len(m) == 0 {
		t.Fatal("planted copy not detected")
	}
	if m[0].RefFile != "ref/Snipes.cpp" {
		t.Errorf("wrong ref file %q", m[0].RefFile)
	}
}

func TestOriginalCodePasses(t *testing.T) {
	ix := NewIndex(DefaultOptions)
	ix.Add("ref/Snipes.cpp", fakeRef)
	if m := ix.Check("core/hive.go", originalGo); len(m) != 0 {
		t.Fatalf("false positive: %+v", m)
	}
}

func TestNumericTablesAreExempt(t *testing.T) {
	table := "{2,3,4,3,4,4,3,4,3,4,4,5,3,4,3,4,3,4,3,4,4,5,4,4,5,5}"
	ix := NewIndex(DefaultOptions)
	ix.Add("ref/tables.h", "static const int accuracy[26] = "+table+";")
	if m := ix.Check("core/tables.go", "var accuracy = [26]int"+table); len(m) != 0 {
		t.Fatalf("data table flagged as copy: %+v", m)
	}
}

// TestCatchesPlantedCopyOfRealReference plants a chunk of the actual
// reference port in a temporary tree and runs the full tree scan on it.
func TestCatchesPlantedCopyOfRealReference(t *testing.T) {
	refDir := filepath.Join("..", "..", "reference")
	src, err := os.ReadFile(filepath.Join(refDir, "Snipes.cpp"))
	if err != nil {
		t.Skip("reference submodule not checked out")
	}
	ix, err := IndexDir(refDir, DefaultOptions)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	mid := len(lines) / 2
	chunk := strings.Join(lines[mid:mid+60], "\n")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "planted.go"), []byte("package x\n"+chunk), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clean.go"), []byte(originalGo), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := CheckTree(root, refDir, ix)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) == 0 {
		t.Fatal("planted copy of reference/Snipes.cpp not detected")
	}
	for _, x := range m {
		if filepath.Base(x.File) != "planted.go" {
			t.Errorf("unexpected match in %s", x.File)
		}
	}
}
