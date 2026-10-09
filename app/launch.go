package app

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/radimlif/nlSnipes/lan"
	"github.com/radimlif/nlSnipes/term"
)

// DiscoverTimeout is how long a new instance looks for a game before hosting.
const DiscoverTimeout = time.Second

// DefaultNick is the user's login name, or the host name, cut to 12 characters.
func DefaultNick() string {
	nick := ""
	if u, err := user.Current(); err == nil {
		nick = u.Username
		if i := strings.LastIndexAny(nick, `\/`); i >= 0 { // DOMAIN\user on Windows
			nick = nick[i+1:]
		}
	}
	if nick == "" {
		nick, _ = os.Hostname()
	}
	if nick == "" {
		nick = "player"
	}
	if len(nick) > lan.MaxNick {
		nick = nick[:lan.MaxNick]
	}
	return nick
}

// Launch finds a game on the LAN and joins it, or becomes the host.
func Launch(t term.Terminal, opt Options) (*Game, error) {
	if opt.Offline {
		return NewOffline(t, opt), nil
	}
	splash := newGame(t, opt)
	splash.composeMessage("Looking for a game on the LAN...", "")
	splash.draw()

	var idb [4]byte
	rand.Read(idb[:])
	id := binary.LittleEndian.Uint32(idb[:]) | 1
	found, ok, err := lan.Discover(lan.GameID(opt.GameName), id, opt.HostAddr, DiscoverTimeout)
	if err != nil {
		return nil, err
	}
	if ok {
		return NewClient(t, opt, found, id)
	}
	return NewHost(t, opt)
}
