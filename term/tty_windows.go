//go:build windows

package term

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[2J"
	leaveScreen = "\x1b[0m\x1b[2J\x1b[?25h\x1b[?1049l"

	keyEvent         = 0x0001
	leftCtrlPressed  = 0x0008
	rightCtrlPressed = 0x0004
)

var procReadConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

// inputRecord mirrors INPUT_RECORD with its KEY_EVENT_RECORD union member.
type inputRecord struct {
	eventType   uint16
	_           uint16
	keyDown     int32
	repeatCount uint16
	virtualKey  uint16
	scanCode    uint16
	char        uint16
	controlKeys uint32
}

type console struct {
	in, out         windows.Handle
	inMode, outMode uint32
	outCP           uint32
	events          chan Event
}

// Open takes over the console: key events with real releases, VT output,
// UTF-8, alternate screen. Close restores it.
func Open() (Terminal, error) {
	c := &console{events: make(chan Event, 64)}
	var err error
	if c.in, err = windows.GetStdHandle(windows.STD_INPUT_HANDLE); err != nil {
		return nil, err
	}
	if c.out, err = windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err != nil {
		return nil, err
	}
	if err = windows.GetConsoleMode(c.in, &c.inMode); err != nil {
		return nil, err
	}
	if err = windows.GetConsoleMode(c.out, &c.outMode); err != nil {
		return nil, err
	}
	c.outCP, _ = windows.GetConsoleOutputCP()
	if err = windows.SetConsoleMode(c.in, windows.ENABLE_EXTENDED_FLAGS|windows.ENABLE_WINDOW_INPUT); err != nil {
		return nil, err
	}
	if err = windows.SetConsoleMode(c.out, windows.ENABLE_PROCESSED_OUTPUT|
		windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN); err != nil {
		windows.SetConsoleMode(c.in, c.inMode)
		return nil, err
	}
	windows.SetConsoleOutputCP(65001) // UTF-8
	os.Stdout.WriteString(enterScreen)
	go c.readLoop()
	return c, nil
}

// scanBase maps PC set-1 scan codes (physical key positions) to the
// character that position types on a US layout.
var scanBase = map[uint16]rune{
	0x02: '1', 0x03: '2', 0x04: '3', 0x05: '4', 0x06: '5', 0x07: '6', 0x08: '7', 0x09: '8', 0x0A: '9', 0x0B: '0',
	0x10: 'q', 0x11: 'w', 0x12: 'e', 0x13: 'r', 0x14: 't', 0x15: 'y', 0x16: 'u', 0x17: 'i', 0x18: 'o', 0x19: 'p',
	0x1E: 'a', 0x1F: 's', 0x20: 'd', 0x21: 'f', 0x22: 'g', 0x23: 'h', 0x24: 'j', 0x25: 'k', 0x26: 'l',
	0x2C: 'z', 0x2D: 'x', 0x2E: 'c', 0x2F: 'v', 0x30: 'b', 0x31: 'n', 0x32: 'm', 0x39: ' ',
}

var vkKeys = map[uint16]Key{
	0x25: KeyLeft, 0x26: KeyUp, 0x27: KeyRight, 0x28: KeyDown,
	0x0D: KeyEnter, 0x1B: KeyEsc, 0x09: KeyTab, 0x08: KeyBackspace, 0x70: KeyF1,
	0x21: KeyPgUp, 0x22: KeyPgDn, 0x23: KeyEnd, 0x24: KeyHome,
}

func (c *console) readLoop() {
	defer close(c.events)
	var recs [16]inputRecord
	down := map[uint16]bool{}
	for {
		var n uint32
		r, _, _ := procReadConsoleInputW.Call(uintptr(c.in), uintptr(unsafe.Pointer(&recs[0])), uintptr(len(recs)), uintptr(unsafe.Pointer(&n)))
		if r == 0 {
			return
		}
		for _, rec := range recs[:n] {
			if rec.eventType != keyEvent {
				continue
			}
			ev := Event{Kind: EvPress}
			switch {
			case rec.keyDown == 0:
				ev.Kind = EvRelease
				delete(down, rec.virtualKey)
			case down[rec.virtualKey]:
				ev.Kind = EvRepeat
			default:
				down[rec.virtualKey] = true
			}
			vk := rec.virtualKey
			base := scanBase[rec.scanCode]
			switch {
			case vkKeys[vk] != KeyNone:
				ev.Key = vkKeys[vk]
			case vk >= 0x60 && vk <= 0x69: // numpad digits
				ev.Key, ev.Rune, ev.Base = KeyRune, rune('0'+vk-0x60), rune('0'+vk-0x60)
			case base == 'c' && rec.controlKeys&(leftCtrlPressed|rightCtrlPressed) != 0:
				ev.Key = KeyCtrlC
			case base != 0:
				ev.Key, ev.Base, ev.Rune = KeyRune, base, base
				if ch := rune(rec.char); ch >= ' ' {
					ev.Rune = lower(ch) // what the layout types, for text entry
				}
			default:
				continue
			}
			c.events <- ev
		}
	}
}

func (c *console) Events() <-chan Event { return c.events }
func (c *console) RealReleases() bool   { return true }

func (c *console) Size() (int, int) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(c.out, &info); err != nil {
		return Width, Height
	}
	return int(info.Window.Right-info.Window.Left) + 1, int(info.Window.Bottom-info.Window.Top) + 1
}

func (c *console) Write(b []byte) error {
	_, err := os.Stdout.Write(b)
	return err
}

func (c *console) Bell() { os.Stdout.WriteString("\a") }

func (c *console) Close() error {
	os.Stdout.WriteString(leaveScreen)
	windows.SetConsoleOutputCP(c.outCP)
	windows.SetConsoleMode(c.out, c.outMode)
	return windows.SetConsoleMode(c.in, c.inMode)
}
