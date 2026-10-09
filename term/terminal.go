package term

// Terminal is a real or simulated screen and keyboard.
type Terminal interface {
	// Events delivers keyboard events; it is closed when input ends.
	Events() <-chan Event
	// RealReleases reports whether EvRelease events are delivered. On
	// Unix it can turn true after EvKittySupported arrives.
	RealReleases() bool
	// Size is the terminal size in characters.
	Size() (w, h int)
	// Write sends raw output (ANSI sequences and UTF-8 text).
	Write(b []byte) error
	// Bell rings the terminal bell.
	Bell()
	// Close restores the terminal.
	Close() error
}

// Present renders f to t through r.
func Present(t Terminal, r *Renderer, f *Frame) error {
	w, h := t.Size()
	if out := r.Render(f, w, h); len(out) > 0 {
		return t.Write(out)
	}
	return nil
}
