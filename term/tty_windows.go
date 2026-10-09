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
			switch {
			case vkKeys[vk] != KeyNone:
				ev.Key = vkKeys[vk]
			case vk == 'C' && rec.controlKeys&(leftCtrlPressed|rightCtrlPressed) != 0:
				ev.Key = KeyCtrlC
			case vk >= 'A' && vk <= 'Z':
				ev.Key, ev.Rune = KeyRune, rune(vk-'A'+'a')
			case vk >= '0' && vk <= '9', vk == ' ':
				ev.Key, ev.Rune = KeyRune, rune(vk)
			case vk >= 0x60 && vk <= 0x69: // numpad digits
				ev.Key, ev.Rune = KeyRune, rune('0'+vk-0x60)
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
