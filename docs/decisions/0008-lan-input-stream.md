# 0008 — LAN play: an input stream with hashes, unicast to each client

Status: accepted · 2026-10-09

## Context
DESIGN-LIGHT.md describes UDP on port 5108 with beacon/probe/join/welcome/input/snapshot/delta/bye,
where deltas carry tile changes and entity upserts, and world state goes to the subnet broadcast
address.

## Decision
Kept: one UDP port family from 5108, magic `NLS1`, the message set, snapshots every 2 s, inputs
carrying the last 4 frames, `--game`, `--host`, `--no-friendly-fire`, 4 players, unlimited
spectators queued for free slots, 3 s silence = gone. Changed:

1. **Deltas carry inputs, not state.** The core is deterministic, so the host sends, every tick,
   the inputs it stepped with and the resulting state hash (`Tick` message: the last 8 records,
   so up to 7 lost packets in a row cost nothing). Each client steps its own copy and compares
   the hash every tick, not only at snapshots. About 140 bytes per tick per client. A mismatch
   or a gap makes the client ask for a snapshot (`Resync`, new message type 10), every 2 ticks
   until one arrives; the host answers at most every 2 ticks per client.
2. **Unicast fan-out.** Ticks and snapshots go to each client's address. Wi-Fi sends broadcast
   at the lowest rate without retries; unicast is faster and acknowledged at the link layer.
   Broadcast is used only for discovery: probes go to 255.255.255.255 and 127.0.0.1 on ports
   5108–5115 (a host takes the first free one, so separate games can share a machine), and the
   host answers with a unicast beacon.
3. **Joins and leaves are inputs** (`core.Input.Join/Leave`), so every copy changes its roster on
   the same tick and replays stay exact. Snapshots no longer carry the 15 KB tile grid (rebuilt
   from the 320-byte maze) and are flate-compressed: a busy Z9 game fits one datagram.
4. **Seating.** The host keeps slot 0 for the whole maze. A client out of lives gives its slot to
   the longest-waiting spectator and joins the back of the queue; a leaving player's slot goes
   to the next spectator too.
5. **IDDQD in LAN** works while only one player is in the game; with more, the host turns mirror
   shots off unless it was started with `--iddqd`.

## Measured (gate tests, loopback)
1 host + 3 players + 2 spectators, 3 600 ticks, 9 mazes: zero desyncs, ~95 snapshot hash matches
per client. 20 % loss in every direction: the longest any client was out of sync was 4–14
ticks over ten runs (limit: one snapshot period, 36). Garbage, mutated and truncated packets
fired at every socket during play: no crash, no desync.
