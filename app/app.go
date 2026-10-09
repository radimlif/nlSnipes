// Package app ties the pieces together: discover a game on the subnet, then
// host or join, play, and take over as host if the host disappears.
// Implemented across milestones L2–L4.
package app

// Version is set at build time by GoReleaser (-ldflags "-X ...app.Version=").
var Version = "dev"
