package term

import (
	"reflect"
	"testing"
)

func TestParserSequences(t *testing.T) {
	press := func(k Key) Event { return Event{Kind: EvPress, Key: k} }
	run := func(k Key, r rune) Event { return Event{Kind: EvPress, Key: k, Rune: r} }
	for _, tc := range []struct {
		name string
		in   string
		want []Event
	}{
		{"csi arrows", "\x1b[A\x1b[B\x1b[C\x1b[D", []Event{press(KeyUp), press(KeyDown), press(KeyRight), press(KeyLeft)}},
		{"ss3 arrows", "\x1bOA\x1bOD", []Event{press(KeyUp), press(KeyLeft)}},
		{"modified arrow", "\x1b[1;2C", []Event{press(KeyRight)}},
		{"f1 variants", "\x1bOP\x1b[11~\x1b[P", []Event{press(KeyF1), press(KeyF1), press(KeyF1)}},
		{"letters lowercased", "Wa", []Event{run(KeyRune, 'w'), run(KeyRune, 'a')}},
		{"space enter tab bs", " \r\t\x7f", []Event{run(KeyRune, ' '), press(KeyEnter), press(KeyTab), press(KeyBackspace)}},
		{"ctrl-c", "\x03", []Event{press(KeyCtrlC)}},
		{"utf8 rune", "é", []Event{run(KeyRune, 'é')}},
		{"kitty arrow press repeat release", "\x1b[1;1:1A\x1b[1;1:2A\x1b[1;1:3A", []Event{
			{Kind: EvPress, Key: KeyUp}, {Kind: EvRepeat, Key: KeyUp}, {Kind: EvRelease, Key: KeyUp}}},
		{"kitty letters", "\x1b[119u\x1b[119;1:3u\x1b[32;1:2u", []Event{
			{Kind: EvPress, Key: KeyRune, Rune: 'w'}, {Kind: EvRelease, Key: KeyRune, Rune: 'w'}, {Kind: EvRepeat, Key: KeyRune, Rune: ' '}}},
		{"kitty functional", "\x1b[13u\x1b[27u\x1b[9u\x1b[127u\x1b[99;5u", []Event{
			press(KeyEnter), press(KeyEsc), press(KeyTab), press(KeyBackspace), press(KeyCtrlC)}},
		{"kitty private-use key ignored", "\x1b[57441u", nil},
		{"kitty and DA answers", "\x1b[?0u\x1b[?62;22c", []Event{{Kind: EvKittySupported}, {Kind: EvDeviceAttributes}}},
		{"unknown csi ignored", "\x1b[200~x", []Event{run(KeyRune, 'x')}},
	} {
		var p Parser
		got := p.Feed([]byte(tc.in))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestParserSplitsAndLoneEsc(t *testing.T) {
	var p Parser
	if ev := p.Feed([]byte("\x1b")); len(ev) != 0 {
		t.Fatalf("lone ESC decided too early: %+v", ev)
	}
	if ev := p.Feed([]byte("[")); len(ev) != 0 {
		t.Fatalf("partial CSI: %+v", ev)
	}
	if ev := p.Feed([]byte("C")); len(ev) != 1 || ev[0].Key != KeyRight {
		t.Fatalf("split arrow: %+v", ev)
	}
	p.Feed([]byte("\x1b"))
	if ev := p.Flush(); len(ev) != 1 || ev[0].Key != KeyEsc {
		t.Fatalf("flushed ESC: %+v", ev)
	}
	if ev := p.Feed([]byte{0xc3}); len(ev) != 0 {
		t.Fatalf("partial utf8: %+v", ev)
	}
	if ev := p.Feed([]byte{0xa9}); len(ev) != 1 || ev[0].Rune != 'é' {
		t.Fatalf("split utf8: %+v", ev)
	}
}

func FuzzParser(f *testing.F) {
	f.Add([]byte("\x1b[1;1:3A\x1bOPabc\x1b[?1u"))
	f.Fuzz(func(t *testing.T, b []byte) {
		var p Parser
		p.Feed(b)
		p.Flush()
		if len(p.buf) != 0 {
			t.Fatal("Flush left bytes behind")
		}
	})
}
