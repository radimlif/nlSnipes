# 0009 — Host migration

Status: accepted · 2026-10-09

## Context
DESIGN-LIGHT.md: if the host quits, the game continues on another player's machine, which
becomes host "from its last full snapshot", lowest slot first, spectators never host, expected
freeze 2–3 s. ADR 0008 changed the protocol to unicast and per-tick input records, so clients do
not hear each other and every client already holds the full, current state.

## Decision
- **Roster carries what a successor needs:** the host's id (`Roster.HostID`) and every seat's
  address as the host sees it (`Seat.Addr`). A peer the host saw on loopback ran on the host's
  machine, so others reach it at the old host's IP.
- **Detection:** a `Bye` from the host (it quit with Esc) starts the handover at once; silence for
  `HostTimeout` = 2 s (crash, cable) starts it too.
- **Successor:** the remaining player with the lowest slot in the last roster, skipping hosts
  that already failed; spectators never host. Every client computes the same answer.
- **Promotion:** the successor turns its own client socket into the host and continues from its
  own state (current to the tick, not the last snapshot) in the same maze epoch. Everyone keeps
  their slot; spectators keep their queue order; the failed host's player leaves and its slot goes
  to the first waiting spectator. The new host also opens a discovery listener on the first free
  port of 5108–5115; its beacons carry the game port (`Beacon.Port`).
- **Followers** point at the successor, send `Join` (repeated until answered) and adopt its first
  snapshot whatever its tick. If the successor stays silent for 2 s, the next one is tried. With
  no player left to take over (or before the game started), clients show "The host left".
- **Next maze:** the host moves back to slot 0, swapping with whoever sat there.

## Measured (`TestGateHostMigration`, fake clock)
Host crashes: all clients back in sync 2.1 s later (2 s detection + one tick). Host quits with
Esc: 0.11 s. Same slots afterwards, the waiting spectator takes the old host's slot, and 600
more ticks run hash-identical.

## Found on the way
A join deferred behind a leave on the same slot could be applied on the leave's own tick and lost
(the leave wins), leaving a spectator seated but not in the game — in L3 too, whenever a player
dropped while someone waited. Deferred joins now wait for the leave; regression test
`TestSpectatorTakesLeaversSlot`.
