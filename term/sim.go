package term

import "sync"

// Sim is an in-memory terminal for headless tests: it records output,
// replays its own renderer's last frame, and takes scripted events.
type Sim struct {
	W, H     int
	Releases bool

	mu     sync.Mutex
	events chan Event
	Output []byte
	Bells  int
	Closed bool
}

// NewSim returns a simulated terminal of the given size.
func NewSim(w, h int, releases bool) *Sim {
	return &Sim{W: w, H: h, Releases: releases, events: make(chan Event, 1024)}
}

// Send queues an input event.
func (s *Sim) Send(ev Event) { s.events <- ev }

func (s *Sim) Events() <-chan Event { return s.events }
func (s *Sim) RealReleases() bool   { return s.Releases }
func (s *Sim) Size() (int, int)     { return s.W, s.H }

func (s *Sim) Write(b []byte) error {
	s.mu.Lock()
	s.Output = append(s.Output, b...)
	s.mu.Unlock()
	return nil
}

func (s *Sim) Bell() {
	s.mu.Lock()
	s.Bells++
	s.mu.Unlock()
}

func (s *Sim) Close() error {
	s.Closed = true
	return nil
}
