package term

import (
	"bytes"
	"strings"
	"testing"
)

func TestRendererSendsOnlyChanges(t *testing.T) {
	var r Renderer
	var f Frame
	f.Clear()
	f.Text(0, 0, "Score 00000", White, Black)
	first := r.Render(&f, 80, 25)
	if !bytes.Contains(first, []byte("\x1b[2J")) || !bytes.Contains(first, []byte("Score")) {
		t.Fatal("first frame not a full redraw")
	}
	if out := r.Render(&f, 80, 25); len(out) != 0 {
		t.Fatalf("unchanged frame sent %d bytes", len(out))
	}
	f.Text(10, 0, "5", White, Black)
	out := string(r.Render(&f, 80, 25))
	if !strings.Contains(out, "\x1b[1;31H") || !strings.HasSuffix(out, "5") {
		t.Fatalf("change not positioned at the centred column: %q", out)
	}
	if len(out) > 32 {
		t.Fatalf("one changed cell cost %d bytes", len(out))
	}
}

func TestRendererRepositionsAfterNonASCII(t *testing.T) {
	var r Renderer
	var f Frame
	f.Clear()
	f.Text(0, 5, "☺→ab", Green, Black)
	out := string(r.Render(&f, 40, 25))
	if !strings.Contains(out, "☺\x1b[6;2H") {
		t.Fatalf("no cursor reposition after a symbol: %q", out)
	}
}

func TestRendererTooSmall(t *testing.T) {
	var r Renderer
	var f Frame
	f.Clear()
	if out := string(r.Render(&f, 30, 20)); !strings.Contains(out, "enlarge") {
		t.Fatalf("no enlarge message: %q", out)
	}
	if out := r.Render(&f, 30, 20); len(out) != 0 {
		t.Fatal("message repeated every frame")
	}
	if out := r.Render(&f, 40, 25); !bytes.Contains(out, []byte("\x1b[2J")) {
		t.Fatal("no full redraw after growing")
	}
}
