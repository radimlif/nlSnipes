# NLSNIPES Light — one executable, LAN auto-join, no server

Sep 30, 2026 · @Radim

## What Light is

One small executable (`nlsnipes.exe`, `nlsnipes` on macOS/Linux). Launch it on any machine on the LAN: if a game is already running on the subnet you join it; if not, you are the host and the game starts. Up to **4 players**, everyone else who launches **spectates** and is queued for the next free slot. No server, no installer, no browser, no accounts, no chat. Text mode in a terminal window, 40 × 25, exactly the original's shape.

The game rules do not change at all — the 128 × 120 toroidal maze, the A1–Z9 tables, snipe AI, scoring and lives are taken verbatim from the full design (§2 and §6 there). What changes is everything around them:

| Full design | Light |
| --- | --- |
| Browser client + terminal client + bots | Terminal client only |
| Dedicated WebSocket server, rooms, lobby codes | Host is whichever instance launched first; discovery by UDP broadcast on the subnet |
| 2–8 players, Co-op and Deathmatch modes, Daily Seed, leaderboard | 4 players, one mode: shared maze, friendly fire on (as in NSNIPES) |
| SQLite, Docker, replay viewer, themes, CRT, gamepad, touch | None. Local high-score file only |
| 7 milestones, \~2 weeks | 5 milestones, \~1 week |

Kept from the modern design because they cost almost nothing and make the build testable: deterministic core with a state hash, seeded games, bots for CI, and host migration (if the host quits, the game continues on another player's machine — the original simply died).

## Architecture

Every copy of the program is the same binary and contains the whole game; roles (host, player, spectator) are decided at runtime by who is already on the subnet.

&#91;embedded content: lifecycle of one instance · launch, discover, host or join, play, migrate\]

The host is the only instance that runs the simulation; everyone else renders what the host broadcasts and sends key presses back. Because full snapshots go out every 2 s, any player has a recent copy of the world and can take over if the host disappears.

Internally one Go module with four packages: `core` (deterministic simulation, identical spec to the full design), `net` (UDP beacon, join, inputs, snapshots), `term` (tcell renderer, 40 × 25, CP437 glyphs via Unicode), `app` (state machine above: discover → host/join → play → migrate). Bots and tests sit beside them.

## Network protocol (UDP on the local subnet)

Everything is UDP on one port, **5108**, with a 4-byte magic `NLS1` and a 1-byte message type. Discovery and world state go to the subnet broadcast address (255.255.255.255, falling back to the interface's directed broadcast); inputs and joins go unicast to the host. No TCP, no handshakes beyond `join`/`welcome`, no encryption — it is a LAN toy, like the original.

| Type | From → to | Payload | When |
| --- | --- | --- | --- |
| `beacon` | host → broadcast | game id (u32), skill code, tick, player count, protocol version | every 500 ms while hosting |
| `probe` | newcomer → broadcast | protocol version | 3 × at 300 ms on launch; a `beacon` reply within 1 s means "join", silence means "host" |
| `join` | client → host | nick (≤ 12 chars), client id (random u32) | once, retried every 300 ms until `welcome` |
| `welcome` | host → client | slot (1–4) or `spectator`, seed, config, current tick | reply to `join` |
| `input` | player → host | tick, 8-bit mask, fast flag — the **last 4 frames**, so a lost packet costs nothing | every tick (18/s) |
| `snapshot` | host → broadcast | full state: tick, hash, RLE tiles (\~2 KB), all entities, players, HUD; may span 2–3 datagrams of ≤ 1 200 B with a part index | every 36 ticks (2 s) and immediately after a `welcome` |
| `delta` | host → broadcast | tick, hash, tile changes, entity upserts/removals, players, HUD, events | every other tick |
| `bye` | anyone → broadcast | client id | on quit (best effort) |

### Loss and ordering

- Deltas are applied only if their tick is exactly `last + 1`; otherwise the client waits for the next full snapshot (at most 2 s of frozen screen on a bad LAN, no divergence). Snapshots are always applied if newer.
- Inputs carry the last 4 frames; the host applies any it has not seen yet, in tick order, and ignores frames older than 8 ticks.
- Clients render the latest applied state. There is no prediction; at 18 Hz on a LAN the round-trip is well under one tick.

### Host migration

If no `snapshot`/`delta` arrives for 2 s, every client checks the roster from its last snapshot: the player with the **lowest slot number that is still alive on the network** (heard from via `input` broadcast-echo or a `probe`) becomes host from its last full snapshot, re-assigns the same slots, and starts beaconing with the same game id. Spectators never host. Ties cannot occur because slots are unique. Expected freeze: 2–3 s, then the game continues; the departed host's smiley is removed.

### Coexistence

Two games on one subnet cannot exist by design (a newcomer always joins the beaconing host). To play separately, launch with `--game <name>`; the name is hashed into the game id and beacons with a different id are ignored. `--host <ip>` skips discovery for VPN or routed networks.

### Firewall note

On first launch Windows asks to allow the program on private networks; that must be accepted for UDP 5108 in and out. The README will say so in one sentence.

## Gameplay decisions for Light

- **Players:** 4 maximum, each with its own smiley colour (white, yellow, cyan, magenta), own lives and own score shown in the HUD roster. Slots are handed out in launch order.
- **Spectators:** unlimited. They see the world through a free camera that follows the lowest-slot living player (Tab cycles players). When a slot frees — a player quits or loses all lives — the longest-waiting spectator gets it and spawns at a safe cell on the next tick.
- **Skill code:** the host's argument (or prompt if none) decides for everyone: `nlsnipes M5`. Joiners' arguments are ignored and the HUD shows the host's code. Default when omitted: `A1`.
- **One mode**, as the original network version: shared maze, shared hives, friendly fire on. Killing another player scores nothing (tribute rule; toggle `--no-friendly-fire` for the host).
- **Game end:** when all hives and snipes are dead the host shows the roster for 10 s, then generates a new maze with the same skill code and everyone respawns. When all players are out of lives the same happens. There is no menu to return to; the original had none either. `Esc` quits.
- **Controls:** arrows move, W/A/S/D fire, Space fast, Tab (spectator) cycle camera, `F1` legend, `Esc` quit. No remapping.
- **Display:** 40 × 25 characters in the terminal, HUD 3 rows, viewport 40 × 22 centred on you with toroidal wrap; 16 ANSI colours mapped to the original CGA indices; CP437 glyphs drawn with their Unicode equivalents (☺ ☻ ← ↑ → ↓ ═ ║ ╔ ╗ ╚ ╝ ▓). If the terminal is larger, the 40 × 25 window is centred; it never scales.
- **Sound:** terminal bell on player death only. Nothing else.
- **High scores:** a single JSON file next to the executable, best score per skill code, local only.
- **Not included, deliberately:** chat, lobby, spectator limits, Deathmatch scoring, Daily Seed, replays as a user feature (the core still records seed + inputs for debug dumps), themes, gamepad, mouse.

## Technology

**Go 1.23 + tcell**, one static binary per platform. The full design chose TypeScript because a browser was a target; Light has no browser, and "double-click the .exe" is the whole point, so the language that produces a 6 MB dependency-free executable with `GOOS=windows go build` wins.

| Decision | Choice | Why |
| --- | --- | --- |
| Language | Go 1.23, standard library only except tcell | Single static binary, trivial cross-compile (Windows, macOS Intel/ARM, Linux), integer math by default, `go test` with fuzzing built in, very agent-friendly |
| Terminal | `github.com/gdamore/tcell/v2` | Handles Windows console and ANSI terminals alike, raw key input with key-up emulation via repeat timing, 16/256/true colour |
| Networking | `net` package: `ListenUDP`, broadcast writes; hand-rolled binary encoding with `encoding/binary` | No dependency; the protocol above is \~10 message structs |
| Serialization | fixed-layout little-endian structs; RLE for tiles | Deterministic byte output, tiny, no reflection |
| PRNG | own xorshift32/sfc32 in `core` (not `math/rand`) | Seedable, serialisable state, identical everywhere |
| Hash | FNV-1a 32 from `hash/fnv` over a canonical byte dump | Same contract as the full design |
| Tests | `go test`, table tests, `testing/quick` or `rapid` for properties, `go test -fuzz` on the packet decoder, bot harness in `cmd/simbot` | One tool |
| Lint | `go vet`, `staticcheck`, `gofmt -l` | One command |
| CI | GitHub Actions: vet, lint, test with `-race`, sim, cross-compile matrix, similarity check vs `reference/`, upload artefacts | Binaries for every platform on every push |
| Release | GoReleaser: tagged release with `nlsnipes-windows-amd64.exe`, `-darwin-arm64`, `-darwin-amd64`, `-linux-amd64` | Users download one file |
| Licence | MIT; NOTICE as in the full design | — |

**Alternative if you prefer to stay in TypeScript:** `bun build --compile` also yields a single executable (60–90 MB) and Node's `dgram` does UDP broadcast; everything in this document is language-neutral. I recommend Go for the size and the Windows console story, but say the word and the handoff files switch.

Things that do not exist in Light and therefore need no decision: web framework, WebSocket library, database, Docker, HTML/CSS, fonts (the terminal's own font renders the glyphs).

## Milestones

Five milestones, one PR each, same working agreement as the full design (checklist in the PR, CI is the arbiter, `blocked` issues instead of lowered gates). Sequential; L2 and L3 could overlap but the whole thing is small enough not to bother.

| # | Milestone | Deliverables | Acceptance gate |
| --- | --- | --- | --- |
| L0 | Scaffold | Go module `nlsnipes`, packages `core`, `net`, `term`, `app`, `cmd/nlsnipes`, `cmd/simbot`; CI (vet, staticcheck, test -race, cross-compile matrix, similarity check vs `reference/` submodule); GoReleaser config; `CLAUDE.md`/`AGENTS.md`, `LICENSE`, `NOTICE`, templates | CI green; binaries for 4 platforms appear as artefacts; similarity job passes and its test catches a planted copy |
| L1 | Core | PRNG, skill tables, maze generator, state, hash, entities, combat, snipe AI, spawning, scoring, lives, win/lose, replay record/playback, ASCII dump; `simbot` with Random and Hunter bots | 1 000-seed connectivity; all 234 skill codes; 10 000-tick bot games at A1/M5/Z9 with 1 and 4 players replay to the same hash; `HunterBot` wins A1 in ≤ 5 000 ticks on ≥ 90 % of 100 seeds; `step` p99 < 1 ms at Z9 with 4 bots |
| L2 | Terminal solo | tcell renderer (40 × 25, HUD, toroidal viewport, CGA colours), key input → mask (with key-release emulation), game loop at 18 Hz, title/skill prompt, roster overlay, F1 legend, high-score file | Runs on Windows Terminal, cmd.exe, macOS Terminal, xterm without garbage; a scripted-input test drives a hive kill (score 50) headlessly through a tcell simulation screen; frame render < 2 ms |
| L3 | LAN play | `net` package: beacon/probe/join/welcome/input/snapshot/delta/bye; host loop; client loop; spectators and slot queue; `--game`, `--host`, `--no-friendly-fire` | Integration test on loopback: 1 host + 3 players + 2 spectators for 3 600 ticks, every client's hash equals the host's at each full snapshot; packet fuzzer never crashes host or client; 20 % simulated loss still converges within one snapshot period |
| L4 | Migration and release | Host migration; new-maze-after-win cycle; README with a GIF and the firewall sentence; `v1.0.0` release via GoReleaser | Migration test: kill the host mid-game, remaining clients resume within 3 s with the same roster and continue to identical hashes; manual 20-minute LAN session on real machines signed off by the owner; release page has 4 binaries |

Effort for an autonomous agent: L0 ½ day, L1 2 days, L2 1 day, L3 2 days, L4 1 day — about one week, versus two for the full design. L1 is unchanged in scope from the full design's M1 + M2 because the rules are the same; that is where the game actually is.

## What carries over from the full design

Unchanged and still authoritative, read from [NLSNIPES Reborn — Research, Design, Architecture & Agent Handoff](https://claude.ai/code/artifact/19bdc84c-9564-4b8d-a1ae-46d64a2a7f87):

- §2 Research: history, verified mechanics, the A1–Z9 tables, the reference port and its licence status.
- §6.1–§6.7 Specifications: grid and wrap, maze generation, PRNG, entity rules, `GameConfig`, `GameState`, `InputFrame` mask. Translate the TypeScript shapes to Go structs one-to-one.
- §6.10 Rendering spec: glyph codes and CGA colour indices.
- §7.1 Working agreement and §8 testing philosophy (determinism as the oracle, bots as QA, replay per bug).
- The handoff mechanics: `CLAUDE.md` = `AGENTS.md`, milestone files as issue bodies, master prompt, one PR per milestone.

Dropped: §3.2 modernised shell, §3.4 multiplayer modes, §5 web/server architecture, §6.8–§6.9 WebSocket protocol and `.nlsr` format (replaced by the UDP protocol above and an internal debug dump), M3–M6.

### Decisions to confirm (defaults in bold)

- [ ] Language: **Go** (alternative: TypeScript compiled with Bun).
- [ ] Friendly fire: **on by default**, as in NSNIPES.
- [ ] After a win or wipe-out: **new maze, same skill code, everyone respawns**; no menu.
- [ ] Port **5108**; game name via `--game` for separate games on one subnet.
- [ ] Repository: **the same `nlsnipes` repo**, with the full design kept in `docs/DESIGN.md` as the long-term plan and this document as `docs/DESIGN-LIGHT.md` driving the first release.
