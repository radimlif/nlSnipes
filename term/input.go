package term

import (
	"strconv"
	"strings"
)

// Key identifies a key. Printable keys are KeyRune with the rune in Event.Rune.
type Key uint8

// Keys the game uses.
const (
	KeyNone Key = iota
	KeyRune
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyEsc
	KeyTab
	KeyBackspace
	KeyF1
	KeyCtrlC
)

// EventKind says what an input event is.
type EventKind uint8

// Event kinds.
const (
	EvPress            EventKind = iota + 1 // first press, or a press when releases are not reported
	EvRepeat                                // auto-repeat of a held key
	EvRelease                               // key let go (only where the platform reports it)
	EvKittySupported                        // terminal answered the kitty keyboard query
	EvDeviceAttributes                      // terminal answered the primary device attributes query
)

// Event is one keyboard event.
type Event struct {
	Kind EventKind
	Key  Key
	Rune rune // lowercase for letters; set for KeyRune
}

// Parser turns raw terminal input bytes into events. It understands the
// legacy VT sequences every terminal sends and the kitty keyboard protocol
// (CSI … u with event types), which reports releases.
type Parser struct {
	buf []byte
}

// Feed appends bytes and returns every complete event. A lone ESC stays
// buffered (it may start a sequence) until Flush.
func (p *Parser) Feed(b []byte) []Event {
	p.buf = append(p.buf, b...)
	var out []Event
	for len(p.buf) > 0 {
		ev, n, ok := parseOne(p.buf)
		if !ok {
			break
		}
		p.buf = p.buf[n:]
		if ev.Kind != 0 {
			out = append(out, ev)
		}
	}
	return out
}

// Flush resolves whatever is buffered: a pending ESC becomes the Esc key.
// Call it when no more input has arrived for a short while.
func (p *Parser) Flush() []Event {
	var out []Event
	for len(p.buf) > 0 {
		if ev, n, ok := parseOne(p.buf); ok {
			p.buf = p.buf[n:]
			if ev.Kind != 0 {
				out = append(out, ev)
			}
			continue
		}
		if p.buf[0] == 0x1b {
			out = append(out, Event{Kind: EvPress, Key: KeyEsc})
		}
		p.buf = p.buf[1:]
	}
	return out
}

// parseOne decodes the event at the start of b. ok is false when b holds
// only the start of a sequence. A consumed but meaningless sequence returns
// a zero Event.
func parseOne(b []byte) (ev Event, n int, ok bool) {
	c := b[0]
	switch {
	case c == 0x1b:
		if len(b) == 1 {
			return Event{}, 0, false
		}
		switch b[1] {
		case '[':
			return parseCSI(b)
		case 'O':
			if len(b) < 3 {
				return Event{}, 0, false
			}
			return press(ss3Keys[b[2]]), 3, true
		case 0x1b:
			return Event{Kind: EvPress, Key: KeyEsc}, 1, true
		}
		return Event{}, 2, true // Alt+key: ignored
	case c == '\r' || c == '\n':
		return press(KeyEnter), 1, true
	case c == '\t':
		return press(KeyTab), 1, true
	case c == 0x7f || c == 0x08:
		return press(KeyBackspace), 1, true
	case c == 0x03:
		return press(KeyCtrlC), 1, true
	case c < ' ':
		return Event{}, 1, true
	}
	r, size := decodeRune(b)
	if size == 0 {
		return Event{}, 0, false
	}
	return Event{Kind: EvPress, Key: KeyRune, Rune: lower(r)}, size, true
}

func press(k Key) Event {
	if k == KeyNone {
		return Event{}
	}
	return Event{Kind: EvPress, Key: k}
}

var ss3Keys = map[byte]Key{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'P': KeyF1}

func lower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 'a' - 'A'
	}
	return r
}

func decodeRune(b []byte) (rune, int) {
	c := b[0]
	size := 1
	switch {
	case c >= 0xf0:
		size = 4
	case c >= 0xe0:
		size = 3
	case c >= 0xc0:
		size = 2
	}
	if len(b) < size {
		return 0, 0
	}
	return []rune(string(b[:size]))[0], size
}

// parseCSI decodes ESC [ params final.
func parseCSI(b []byte) (Event, int, bool) {
	i := 2
	for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
		i++
	}
	if i >= len(b) {
		if i > 64 {
			return Event{}, i, true // runaway sequence: drop it
		}
		return Event{}, 0, false
	}
	params, final, n := string(b[2:i]), b[i], i+1
	switch {
	case final == 'u' && strings.HasPrefix(params, "?"):
		return Event{Kind: EvKittySupported}, n, true
	case final == 'c' && strings.HasPrefix(params, "?"):
		return Event{Kind: EvDeviceAttributes}, n, true
	}
	fields := strings.Split(params, ";")
	kind := EvPress
	if len(fields) > 1 {
		// Modifier field "mods:event" — kitty event types 1 press, 2 repeat, 3 release.
		if _, ev, found := strings.Cut(fields[1], ":"); found {
			switch ev {
			case "2":
				kind = EvRepeat
			case "3":
				kind = EvRelease
			}
		}
	}
	var key Key
	var r rune
	switch final {
	case 'A':
		key = KeyUp
	case 'B':
		key = KeyDown
	case 'C':
		key = KeyRight
	case 'D':
		key = KeyLeft
	case 'P':
		key = KeyF1
	case '~':
		if code, _, _ := strings.Cut(fields[0], ":"); code == "11" {
			key = KeyF1
		}
	case 'u':
		code, _, _ := strings.Cut(fields[0], ":")
		v, err := strconv.Atoi(code)
		if err != nil {
			return Event{}, n, true
		}
		switch v {
		case 13:
			key = KeyEnter
		case 27:
			key = KeyEsc
		case 9:
			key = KeyTab
		case 127, 8:
			key = KeyBackspace
		default:
			if v >= ' ' && v < 0xE000 { // below kitty's private-use functional keys
				key, r = KeyRune, lower(rune(v))
			}
		}
		if key == KeyRune && r == 'c' && len(fields) > 1 {
			if mods, _, _ := strings.Cut(fields[1], ":"); mods == "5" { // ctrl
				key, r = KeyCtrlC, 0
			}
		}
	}
	if key == KeyNone {
		return Event{}, n, true
	}
	return Event{Kind: kind, Key: key, Rune: r}, n, true
}
