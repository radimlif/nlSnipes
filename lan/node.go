package lan

import (
	"bytes"
	"compress/flate"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/radimlif/nlSnipes/core"
)

// packet is one received datagram.
type packet struct {
	data []byte
	from netip.AddrPort
}

// node is a UDP socket with a background reader. Game logic never blocks:
// it drains received packets with poll.
type node struct {
	conn *net.UDPConn
	in   chan packet
	once sync.Once

	// loss drops outgoing packets for tests (0 = none); deterministic.
	loss    uint32 // out of 1000
	lossRng core.Rand
	mu      sync.Mutex
}

func newNode(addr string, loss float64, seed uint32) (*node, error) {
	ua, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return nil, err
	}
	n := &node{conn: c, in: make(chan packet, 1024), loss: uint32(loss * 1000), lossRng: core.NewRand(seed)}
	go n.readLoop()
	return n, nil
}

func (n *node) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		k, from, err := n.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			close(n.in)
			return
		}
		select {
		case n.in <- packet{data: append([]byte(nil), buf[:k]...), from: from}:
		default: // full: drop, like the network would
		}
	}
}

// poll returns every packet received so far without blocking.
func (n *node) poll() []packet {
	var out []packet
	for {
		select {
		case p, ok := <-n.in:
			if !ok {
				return out
			}
			out = append(out, p)
		default:
			return out
		}
	}
}

// wait blocks until a packet arrives or d passes, then polls.
func (n *node) wait(d time.Duration) []packet {
	select {
	case p, ok := <-n.in:
		if !ok {
			return nil
		}
		return append([]packet{p}, n.poll()...)
	case <-time.After(d):
		return nil
	}
}

func (n *node) send(to netip.AddrPort, msg any) {
	if n.loss > 0 {
		n.mu.Lock()
		drop := n.lossRng.Int(1000) < n.loss
		n.mu.Unlock()
		if drop {
			return
		}
	}
	n.conn.WriteToUDPAddrPort(Encode(msg), to)
}

func (n *node) addr() netip.AddrPort {
	return n.conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

func (n *node) close() { n.once.Do(func() { n.conn.Close() }) }

// snapshotParts compresses a state and splits it into datagram-sized parts.
func snapshotParts(epoch uint32, s *core.State) []Snapshot {
	raw, _ := s.MarshalBinary()
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestSpeed)
	w.Write(raw)
	w.Close()
	data := buf.Bytes()
	const chunk = MaxDatagram - 32
	parts := (len(data) + chunk - 1) / chunk
	hash := s.Hash()
	out := make([]Snapshot, 0, parts)
	for i := 0; i < parts; i++ {
		end := min((i+1)*chunk, len(data))
		out = append(out, Snapshot{Epoch: epoch, Tick: s.Tick, Hash: hash, Part: uint8(i), Parts: uint8(parts), Data: data[i*chunk : end]})
	}
	return out
}

// assembler collects snapshot parts until one snapshot is complete.
type assembler struct {
	epoch, tick uint32
	parts       [][]byte
	have        int
}

// add stores a part; it returns the decoded state when the last part arrives.
func (a *assembler) add(p Snapshot) (*core.State, bool) {
	if a.parts == nil || p.Epoch != a.epoch || p.Tick != a.tick || int(p.Parts) != len(a.parts) {
		a.epoch, a.tick, a.parts, a.have = p.Epoch, p.Tick, make([][]byte, p.Parts), 0
	}
	if a.parts[p.Part] != nil {
		return nil, false
	}
	a.parts[p.Part] = p.Data
	if a.have++; a.have < len(a.parts) {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(bytes.Join(a.parts, nil))), 8<<20))
	a.parts = nil
	if err != nil {
		return nil, false
	}
	var s core.State
	if s.UnmarshalBinary(raw) != nil || s.Hash() != p.Hash {
		return nil, false
	}
	return &s, true
}
