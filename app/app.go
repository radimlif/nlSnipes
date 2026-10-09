// Package app ties the pieces together: the solo game loop now; LAN
// discovery, host/join and host migration in milestones L3–L4.
package app

// Version is set at build time by GoReleaser (-ldflags "-X ...app.Version=").
var Version = "dev"
