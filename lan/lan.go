// Package lan is the UDP LAN protocol: beacon, probe, join, welcome, input,
// snapshot, delta and bye on port 5108 (docs/DESIGN-LIGHT.md, "Network
// protocol"). Named lan rather than net so it can import the standard
// library's net package without an alias (docs/decisions/0002).
//
// Implemented in milestone L3.
package lan

// Port is the single UDP port used for discovery and play.
const Port = 5108

// Magic prefixes every datagram.
const Magic = "NLS1"
