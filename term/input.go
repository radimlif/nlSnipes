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
	KeyHome
	KeyEnd
	KeyPgUp
	KeyPgDn
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
	Rune rune // the character typed, lowercase for letters; set for KeyRune
	// Base is the key's position on a US layout ('z' for the bottom-left
	// letter on QWERTY, QWERTZ and AZERTY alike), when the terminal reports
	// it: kitty's alternate keys, Windows scan codes. 0 when unknown.
	Base rune
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

// Legacy "CSI n ~" keys (xterm, VT220 and rxvt variants).
var tildeKeys = map[string]Key{"11": KeyF1, "1": KeyHome, "7": KeyHome, "4": KeyEnd, "8": KeyEnd, "5": KeyPgUp, "6": KeyPgDn}

// Kitty keypad codes: KP_0..KP_9 and the keypad navigation keys.
const kittyKP0 = 57399

var kittyKeypad = map[int]Key{
	57417: KeyLeft, 57418: KeyRight, 57419: KeyUp, 57420: KeyDown,
	57421: KeyPgUp, 57422: KeyPgDn, 57423: KeyHome, 57424: KeyEnd,
}

var ss3Keys = map[byte]Key{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'P': KeyF1, 'H': KeyHome, 'F': KeyEnd}

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
	var r, base rune
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
	case 'H':
		key = KeyHome
	case 'F':
		key = KeyEnd
	case '~':
		code, _, _ := strings.Cut(fields[0], ":")
		key = tildeKeys[code]
	case 'u':
		// "code:shifted:base" — base is the US-layout key (flag 4).
		parts := strings.Split(fields[0], ":")
		v, err := strconv.Atoi(parts[0])
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
			switch {
			case v >= ' ' && v < 0xE000: // below kitty's private-use functional keys
				key, r = KeyRune, lower(rune(v))
			case v >= kittyKP0 && v <= kittyKP0+9: // keypad digits
				key, r = KeyRune, rune('0'+v-kittyKP0)
			default:
				key = kittyKeypad[v]
			}
		}
		if len(parts) > 2 {
			if b, err := strconv.Atoi(parts[2]); err == nil && b >= ' ' && b < 0x7f {
				base = lower(rune(b))
			}
		} else if key == KeyRune && r < 0x7f {
			base = r // no alternate reported: the key is the same on a US layout
		}
		if key == KeyRune && base == 'c' && len(fields) > 1 {
			if mods, _, _ := strings.Cut(fields[1], ":"); mods == "5" { // ctrl
				key, r, base = KeyCtrlC, 0, 0
			}
		}
	}
	if key == KeyNone {
		return Event{}, n, true
	}
	return Event{Kind: kind, Key: key, Rune: r, Base: base}, n, true
}
