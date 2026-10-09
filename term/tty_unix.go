//go:build !windows

package term

import (
	"os"
	"sync/atomic"
	"time"

	xterm "golang.org/x/term"
)

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[2J"
	leaveScreen = "\x1b[0m\x1b[2J\x1b[?25h\x1b[?1049l"
	// Ask for the kitty keyboard flags, then primary device attributes; a
	// terminal that knows the protocol answers the first before the second.
	queryKitty = "\x1b[?u\x1b[c"
	// Push flags 1 (disambiguate) + 2 (report press/repeat/release) +
	// 8 (report every key as an escape code, so letters release too).
	pushKitty = "\x1b[>11u"
	popKitty  = "\x1b[<u"
	// escTimeout is how long a lone ESC waits before it counts as the Esc key.
	escTimeout = 40 * time.Millisecond
)

type tty struct {
	in, out *os.File
	state   *xterm.State
	events  chan Event
	kitty   atomic.Bool
}

// Open takes over the controlling terminal: raw input, alternate screen,
// hidden cursor. Close restores it.
func Open() (Terminal, error) {
	in, out := os.Stdin, os.Stdout
	state, err := xterm.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	t := &tty{in: in, out: out, state: state, events: make(chan Event, 64)}
	if _, err := out.WriteString(enterScreen + queryKitty); err != nil {
		xterm.Restore(int(in.Fd()), state)
		return nil, err
	}
	go t.readLoop()
	return t, nil
}

func (t *tty) readLoop() {
	chunks := make(chan []byte, 16)
	go func() {
		defer close(chunks)
		for {
			b := make([]byte, 256)
			n, err := t.in.Read(b)
			if n > 0 {
				chunks <- b[:n]
			}
			if err != nil {
				return
			}
		}
	}()
	var p Parser
	timer := time.NewTimer(time.Hour)
	defer close(t.events)
	for {
		select {
		case b, ok := <-chunks:
			if !ok {
				return
			}
			t.deliver(p.Feed(b))
			if len(p.buf) > 0 {
				timer.Reset(escTimeout)
			}
		case <-timer.C:
			t.deliver(p.Flush())
		}
	}
}

func (t *tty) deliver(evs []Event) {
	for _, ev := range evs {
		switch ev.Kind {
		case EvDeviceAttributes:
			continue
		case EvKittySupported:
			if t.kitty.Swap(true) {
				continue
			}
			t.out.WriteString(pushKitty)
		}
		t.events <- ev
	}
}

func (t *tty) Events() <-chan Event { return t.events }
func (t *tty) RealReleases() bool   { return t.kitty.Load() }

func (t *tty) Size() (int, int) {
	w, h, err := xterm.GetSize(int(t.out.Fd()))
	if err != nil {
		return Width, Height
	}
	return w, h
}

func (t *tty) Write(b []byte) error {
	_, err := t.out.Write(b)
	return err
}

func (t *tty) Bell() { t.out.WriteString("\a") }

func (t *tty) Close() error {
	if t.kitty.Load() {
		t.out.WriteString(popKitty)
	}
	t.out.WriteString(leaveScreen)
	return xterm.Restore(int(t.in.Fd()), t.state)
}
