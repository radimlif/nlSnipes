package term

import "time"

// KeyID names a key for hold tracking: arrows and lowercase runes.
type KeyID struct {
	Key  Key
	Rune rune
}

// KeyState answers "is this key held right now?".
//
// With real releases (Windows console, kitty protocol) that is exact. Other
// terminals only send a press, then — after the OS repeat delay — a stream
// of repeats, and nothing on release. There a key counts as held from the
// press through the repeat delay (so a hold never stutters while the
// repeats start), then for as long as repeats keep coming, until one is
// overdue. Until the first repeat the hold is only a guess — Guessing says
// so, and the game uses it to keep a tap from walking into a wall.
//
// The repeat delay and interval are learned from what the terminal sends.
type KeyState struct {
	Real bool // releases are reported

	keys     map[KeyID]*hold
	delay    time.Duration // learned initial repeat delay
	interval time.Duration // learned repeat interval
}

type hold struct {
	down, last time.Time
	repeats    int
	released   bool
	tapped     bool // pressed since the last Active call
}

// Emulation tuning.
const (
	delaySlack      = 80 * time.Millisecond // how long past the expected first repeat we keep waiting
	certainFor      = 60 * time.Millisecond // a fresh press is certain for this long
	defaultDelay    = 500 * time.Millisecond
	defaultInterval = 50 * time.Millisecond
)

// NewKeyState returns a tracker; real says whether releases will be reported.
func NewKeyState(real bool) *KeyState {
	return &KeyState{Real: real, keys: map[KeyID]*hold{}, delay: defaultDelay, interval: defaultInterval}
}

// Handle records an event at time now.
func (k *KeyState) Handle(ev Event, now time.Time) {
	id := KeyID{ev.Key, ev.Rune}
	h := k.keys[id]
	switch ev.Kind {
	case EvRelease:
		if h != nil {
			h.released = true
		}
	case EvPress, EvRepeat:
		if h == nil || h.released || (!k.Real && ev.Kind == EvPress && k.expired(h, now)) {
			k.keys[id] = &hold{down: now, last: now, tapped: true}
			k.dropOpposite(id)
			return
		}
		if !k.Real {
			gap := now.Sub(h.last)
			if h.repeats == 0 {
				if gap >= 150*time.Millisecond && gap <= time.Second {
					k.delay = (k.delay*3 + gap) / 4
				}
			} else if gap >= 10*time.Millisecond && gap <= 200*time.Millisecond {
				k.interval = (k.interval*7 + gap) / 8
			}
		}
		h.repeats++
		h.last = now
	}
}

// Pressing one direction ends a held opposite one (helps emulation, where
// the old key's release was never seen).
func (k *KeyState) dropOpposite(id KeyID) {
	opp := map[KeyID]KeyID{
		{KeyLeft, 0}: {KeyRight, 0}, {KeyRight, 0}: {KeyLeft, 0},
		{KeyUp, 0}: {KeyDown, 0}, {KeyDown, 0}: {KeyUp, 0},
		{KeyRune, 'a'}: {KeyRune, 'd'}, {KeyRune, 'd'}: {KeyRune, 'a'},
		{KeyRune, 'w'}: {KeyRune, 's'}, {KeyRune, 's'}: {KeyRune, 'w'},
	}
	if o, ok := opp[id]; ok {
		delete(k.keys, o)
	}
}

func (k *KeyState) releaseGrace() time.Duration {
	return min(max(2*k.interval, 60*time.Millisecond), 200*time.Millisecond)
}

func (k *KeyState) expired(h *hold, now time.Time) bool {
	if h.repeats == 0 {
		return now.Sub(h.down) > k.delay+delaySlack
	}
	return now.Sub(h.last) > k.releaseGrace()
}

// Held reports whether key id is down at time now.
func (k *KeyState) Held(id KeyID, now time.Time) bool {
	h := k.keys[id]
	if h == nil {
		return false
	}
	if k.Real {
		return !h.released
	}
	if k.expired(h, now) {
		delete(k.keys, id)
		return false
	}
	return true
}

// Guessing reports whether key id counts as held only because repeats may
// still be coming: emulation, no repeat seen yet, and the press is no
// longer fresh.
func (k *KeyState) Guessing(id KeyID, now time.Time) bool {
	h := k.keys[id]
	return !k.Real && h != nil && h.repeats == 0 && now.Sub(h.down) > certainFor && k.Held(id, now)
}

// Active is Held, except that a press since the last Active call counts
// even if the key was already released — so a tap shorter than a game tick
// still does something.
func (k *KeyState) Active(id KeyID, now time.Time) bool {
	h := k.keys[id]
	if h == nil {
		return false
	}
	tapped := h.tapped
	h.tapped = false
	return k.Held(id, now) || tapped
}

// Reset forgets every key (e.g. when the window loses focus or a screen changes).
func (k *KeyState) Reset() { k.keys = map[KeyID]*hold{} }
