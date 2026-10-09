# NLSNIPES Reborn — Research, Design, Architecture & Agent Handoff

Sep 30, 2026 · @Radim

## Executive summary

Build **NLSNIPES Reborn**: a faithful, open-source (MIT) re-creation of the 1982 SuperSet text-mode maze shooter *Snipes*, playable in a browser and in a terminal, with the original A1–Z9 skill codes and a modern server-authoritative multiplayer (co-op and deathmatch, 2–8 players) replacing the NetWare shared-file hack.

The design rests on three decisions. First, one pure, deterministic TypeScript simulation core (`@nlsnipes/core`, zero dependencies, integer math, seeded PRNG) runs identically in the browser, in a Web Worker, in Node on the server and in tests; every mode is the same core behind a different transport. Second, the exact original rules are reproduced from the reverse-engineered reference port (Davidebyzero/Snipes), used as a behavioural specification only — no code is copied, because its copyright stays with the original authors. Third, the delivery is sliced into seven milestones with hard test gates so that coding agents (Claude Code / Opus 5.5) can work autonomously, one milestone per PR, with CI as the arbiter.

What the reader gets from this document: the verified facts about the original game (section 2), the modern game design (3), technology choices (4), architecture with diagrams (5), exact numeric specs and the wire protocol (6), the milestone plan (7), the test strategy (8), and a ready-to-paste `CLAUDE.md` plus master prompt for the coding agents (9). Section 10 covers scoping GitHub access to a single repository.

## Research: the original Snipes

Snipes is a 1982 text-mode maze shooter by SuperSet Software (Drew Major, Kyle Powell, Dale Neibaur, Mark Hurst), written in PL/M-86 because no C compiler existed for the IBM PC yet; its networked version, demoed at the National Computer Conference in June 1982, was the first network-aware PC program and later shipped with NetWare 2.x as NSNIPES/NCSNIPES and NLSNIPES ([Network World interview with Drew Major](https://www.networkworld.com/article/2297960/novell-and-the-computer-game-that-changed-networking.html), [Wikipedia](<https://en.wikipedia.org/wiki/Snipes_(video_game)>)). It ran at 18 frames per second and was based on a Convergent Technologies game called *Rats*. Novell replaced it with NetWars in 1993.

### Verified mechanics (from the reference port source)

The numbers below were read directly from [Davidebyzero/Snipes](https://github.com/Davidebyzero/Snipes), a 2016 C++ port reverse-engineered from the 1982 colour executable with 100% identical logic, released with the original authors' permission but with copyright retained by them. Treat it as a behavioural spec, never as code to copy.

| Aspect | Original value |
| --- | --- |
| Maze | 16 × 20 cells, each 8 × 6 characters → 128 × 120 character grid; the maze **wraps toroidally** in both axes |
| Maze generation | Randomised spanning tree over the cell grid (all cells connected), then a few random extra wall removals; walls drawn with CP437 double-line box characters |
| Viewport | 40 × 25 text mode: 3 HUD rows (Skill, Time, Men Left, Score) + 40 × 22 scrolling window centred on the player |
| Tick rate | 18.2 Hz (PC timer); movement is per-tick, no sub-cell positions |
| Player | 2 × 2 sprite (smiley face + legs); 8-direction movement (arrow keys, combinations for diagonals); fires with W/A/S/D, combinable for diagonal shots; holding Space doubles movement speed; fire period = 2 frame-pairs |
| Hives ("snipe portals") | 2 × 2 box sprites; spawn snipes on a timer (base 5 frames, slower with distance from the player); destroyed by one player bullet |
| Large snipes | 2 × 1 sprite (smiley + arrow showing heading); wander with 3/4 chance to keep direction, 1/4 to turn, 1/16 fully random; fire "spears" at the player with a per-letter accuracy |
| Small snipes ("ghosts") | 1 × 1 sprite; appear when a large snipe is shot on letters that enable them; chance to explode instead of splitting is per letter |
| Scoring | +1 per snipe hit, +50 per hive destroyed |
| Win / lose | Win when all hives are destroyed and no snipes remain; lose when lives ("Men Left") reach 0 |
| Replay | Reference port records seed + skill + input stream; playback is bit-exact (proof that the sim is deterministic) |

### Skill code A1–Z9 — exact tables

The **letter** sets rule flags; the **digit** sets quantity and lives. Total 26 × 9 = 234 combinations.

Digit tables (index 1..9):

| Digit | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Max snipes alive | 10 | 20 | 30 | 40 | 60 | 80 | 100 | 120 | 150 |
| Hives at start | 3 | 3 | 4 | 4 | 5 | 5 | 6 | 8 | 10 |
| Lives | 5 | 5 | 5 | 5 | 5 | 4 | 4 | 3 | 2 |

Letter tables (A..Z, 26 entries each):

| Flag | Values A→Z |
| --- | --- |
| Snipe shooting accuracy (higher = more accurate) | 2 3 4 3 4 4 3 4 3 4 4 5 3 4 3 4 3 4 3 4 4 5 4 4 5 5 |
| Small snipes (ghosts) enabled | 0 0 0 1 1 1 0 0 1 1 1 1 0 0 1 1 0 0 1 1 1 1 1 1 1 1 |
| Diagonal bullets bounce off walls | 0 0 0 0 0 0 1 1 1 1 1 1 0 0 0 0 1 1 1 1 1 1 1 1 1 1 |
| Small-snipe explosion mask (lower = explodes more often) | 7F 7F 7F 3F 3F 1F 7F 7F 3F 3F 1F 1F 7F 7F 3F 3F 7F 7F 3F 3F 1F 1F 3F 1F 1F 0F |
| Electric (deadly) walls | letters M–Z |
| Hives resist snipe spears | letters W–Z |

A bouncing bullet gets 1–8 bounces (random). The Linux clone *lsnipes* documents the same feature ladder in words ([man page](https://mankier.com/6/snipes)), which matches the tables above.

### Network play in the original

NSNIPES required a shared network drive with read/write access; clients synchronised through a shared file rather than IPX packets, which is why it also worked on Windows XP file shares. Players shared one maze and could shoot each other. This is the part we replace wholesale with a real-time authoritative server.

### Existing ports and clones (useful references, none to copy)

| Project | What it is | Licence note |
| --- | --- | --- |
| [Davidebyzero/Snipes](https://github.com/Davidebyzero/Snipes) | SDL2/Win console C++ port, bit-exact, replay files | Copyright retained by original authors; permission to publish only |
| lsnipes (Debian/Fedora package) | 1990s Linux reimplementation, ncurses/X, partial difficulty support | GPL — do not import code into an MIT project |
| Snipes Reloaded (LÖVE/Lua) | Graphical fan remake | Unknown; ignore |
| Snapcraft "snipes" | Packaging of the Davidebyzero port | n/a |

## Game design: the modern take

The rule: **the simulation stays faithful, the shell gets modern.** Anyone who played NSNIPES in 1990 must recognise the maze, the smileys, the skill code and the chunky 18 Hz movement within seconds; everything around it (input, display, networking, persistence, accessibility) is 2026-grade.

### Faithful (Classic ruleset)

- 128 × 120 toroidal maze, 16 × 20 cells of 8 × 6, spanning-tree generation, double-line box walls.
- All 234 skill codes with the exact tables from section 2; same entity sizes, snipe AI odds, spawn cadence, scoring (+1 / +50), lives.
- 18 Hz simulation tick; per-cell movement; Space = fast; W/A/S/D fire + arrows move, diagonals by combination.
- 40 × 22 viewport by default ("Classic view"), CP437 glyphs, CGA 16-colour palette.
- Win when hives and snipes are gone; lose at 0 lives.

### Modernised (shell and options)

| Area | Original | Reborn |
| --- | --- | --- |
| Platforms | DOS text mode | Browser (desktop + mobile touch), Node terminal client (ANSI, true-colour) |
| Multiplayer | Shared file on a NetWare drive | Authoritative WebSocket server, lobbies with room codes, 2–8 players, spectators |
| Modes | Solo; network free-for-all | Solo Classic, Co-op (shared maze, shared hives, shared lives pool option), Deathmatch (players can shoot each other, respawns, frag limit), Daily Seed (same seed for everyone for 24 h, leaderboard) |
| Input | Keyboard only | Keyboard (remappable), gamepad (two sticks: move / fire), touch (two virtual sticks) |
| View | 40 × 22 window | Classic 40 × 22, Wide (fits window, integer-scaled glyphs), optional minimap showing hives, optional CRT shader |
| Themes | CGA colours, CP437 | CGA default; Amber and Green mono; high-contrast accessible; theme = palette + glyph map, swappable at runtime |
| Sound | PC speaker beeps | Web Audio square-wave synth reproducing the beep timbre, volume control, mute |
| Persistence | None | Local best scores per skill code; server leaderboard for Daily Seed; replay files (seed + inputs) downloadable and shareable by URL |
| Onboarding | No title screen, no docs | Title screen, 20-second interactive tutorial, in-game legend, help overlay (?) |
| Determinism | Implicit | Explicit: replays, seeded games (`?seed=`), sim hash shown in debug overlay |

### Entities and rules (shared by all modes)

| Entity | Size | Behaviour | Killed by | Score |
| --- | --- | --- | --- | --- |
| Player | 2 × 2 | 8-dir movement, 8-dir fire, fast with Space; dies on spear, on snipe contact, on electric wall (letters M+) | Spear, snipe, wall (M+), other player (Deathmatch) | — |
| Hive | 2 × 2 | Spawns large snipes on cadence while alive snipes < max; resists spears on W+ | Player bullet | 50 |
| Large snipe | 2 × 1 | Wanders (3/4 keep, 1/4 turn, 1/16 random); fires spears toward player with letter accuracy | Player bullet; may split into a small snipe on ghost letters | 1 |
| Small snipe | 1 × 1 | Faster erratic wander, no shooting; may explode instead of spawning (mask per letter) | Player bullet | 1 |
| Bullet | 1 × 1 | Player projectile; diagonal bullets bounce 1–8 times on bounce letters | Wall, target | — |
| Spear | 1 × 1 | Snipe projectile, kills players, destroys hives unless W+ | Wall, target | — |

### Multiplayer rules

- Co-op: all players share one maze, one hive set and one score; each player has own lives; game ends on win or when all players are out. Friendly fire off by default (toggle).
- Deathmatch: hives spawn snipes as environmental hazard; players score +10 per player kill, −5 per death; respawn after 3 s at a random cell far from others; first to frag limit (default 20) or time limit (default 10 min) wins.
- Late join: allowed in lobby and in-game for Co-op (spawns at a safe cell); spectators receive the same stream without an input slot.
- Host migration is not needed: the server owns the game.

### Out of scope for v1

Accounts and OAuth login, ranked matchmaking, mod support, mobile app stores, level editor. All are compatible with the architecture and listed in section 11 as follow-ups.

## Technology decisions

TypeScript everywhere, one deterministic core, boring infrastructure. These choices minimise the number of languages and toolchains an autonomous coding agent must juggle, and every piece has first-class Node/browser test tooling.

| Decision | Choice | Why | Rejected alternative |
| --- | --- | --- | --- |
| Language | TypeScript 5.x, `strict`, ES2022 target | Same code in browser, worker, server and tests; agents are strongest in TS; type-checked protocol | Rust + WASM (better perf, but two toolchains and slower agent iteration); C++ (no browser story) |
| Repo layout | pnpm workspaces monorepo, Turborepo optional | One lockfile, one CI, package boundaries enforce the pure-core rule | Multi-repo (needless coordination) |
| Simulation core | `@nlsnipes/core`: pure functions, integer math only, zero runtime dependencies, no `Date`/`Math.random` | Determinism across engines; replays; property tests | Physics/ECS framework (overkill for a 128 × 120 tile game) |
| PRNG | 32-bit xorshift-family (e.g. mulberry32/sfc32) with explicit state in `GameState` | Cheap, seedable, serialisable; original used a 16-bit LCG — bit-exact replay of 1982 games is a stretch goal, not v1 | `Math.random` (non-seedable) |
| Tick | 18 Hz fixed step (`TICK_MS = 1000/18`); render loop decoupled at display refresh | Preserves the original feel; small tick count keeps bandwidth tiny | 60 Hz sim (alters game feel, 3× traffic) |
| Web client | Vite + vanilla TS; Canvas 2D glyph renderer drawing from a pre-rendered CP437 atlas; optional WebGL CRT post-pass | No framework to fight; a 40 × 22 grid needs \~900 draw calls per frame at most; trivially testable | React/Preact (adds state-sync complexity for a game loop); PixiJS (heavy for glyphs) |
| Font | "Ultimate Oldschool PC Font Pack" Px437 IBM VGA 8 × 16 (CC BY-SA 4.0, attribution in About) rendered to atlas; fallback to any monospace | Authentic CP437 glyphs, permissive licence | Bundling the original ROM font (unclear rights) |
| Terminal client | Node 22, raw-mode stdin, ANSI true-colour output, own diff renderer (no ncurses dep) | Honours the DOS heritage; useful for headless smoke tests | blessed/ink (unmaintained or React-flavoured) |
| Server | Node 22, `ws` library, one process, rooms in memory | Fits 8-player rooms × hundreds of rooms on one core; simplest to operate | uWebSockets.js (faster, but native binary); Go (second language) |
| Wire format | Binary via `msgpackr` for game traffic; JSON for lobby/control; schemas in `@nlsnipes/protocol` with `zod` validation on both ends | Compact snapshots, validated inputs | Protobuf (codegen step), raw JSON (3–5× bigger) |
| Persistence | SQLite via `better-sqlite3` for leaderboard and replays; file-backed; no ORM | One file, zero ops, ACID, trivial backup | Postgres (a second service to run) |
| Tests | Vitest (unit, property via `fast-check`), Playwright (e2e browser), `tsx` scripts for bot simulations | Fast, standard, agent-friendly | Jest (slower ESM story) |
| Lint/format | Biome (lint + format in one tool) | One config, one command, fast | ESLint + Prettier (two tools, more config) |
| CI | GitHub Actions: typecheck, lint, unit, property, e2e (Chromium), determinism cross-check (Node vs browser hash) | Gates every PR; agents get objective feedback | — |
| Packaging | Single Docker image: server serves the built web client and the WS endpoint; `docker compose` for local run | One artefact to deploy on any VM | Separate static host + API |
| Licence | MIT for all original code; NOTICE file crediting SuperSet Software and the reverse-engineering work | Clean, permissive; reference code never enters the tree | — |

### Non-negotiable engineering rules

1. `@nlsnipes/core` imports nothing from `node:*`, `window`, or any package. It exports `createGame(config, seed)`, `step(state, inputs) → state`, `hash(state)`, `serialize/deserialize`.
2. All positions, timers and counters are integers. No floats in `core`.
3. Every random draw goes through the state's PRNG. `Math.random` is banned by a lint rule in `core` and `protocol`.
4. The server never trusts the client: inputs are validated against the schema and clamped; snapshots are the only truth.
5. Reference sources (`Davidebyzero/Snipes`, `lsnipes`) may be read for behaviour, never copied. Any file in the repo must be original work; CI runs a similarity check against a vendored copy of the reference to enforce this.

## Architecture

One deterministic simulation core, three thin clients, one authoritative server. Solo play is "multiplayer with a local transport": the web client hosts the core in a Web Worker and talks to it through the same message types the server uses, so there is exactly one game loop implementation to test.

&#91;embedded content: runtime architecture · 3 clients, 1 protocol, 1 server, 1 shared core\]

Clients only render and send inputs; the two accented boxes are the only places `@nlsnipes/core` executes, and they run the same package.

### Packages (pnpm workspace)

| Package | Runs in | Depends on | Responsibility |
| --- | --- | --- | --- |
| `@nlsnipes/core` | everywhere | nothing | `GameConfig` from skill code; `GameState`; `step(state, inputsByPlayer)`; maze generator; entity logic; `hash(state)`; snapshot/delta production; replay record/playback |
| `@nlsnipes/protocol` | everywhere | `zod`, `msgpackr` | Message schemas (lobby JSON, game binary), codec, protocol version constant |
| `@nlsnipes/server` | Node 22 | core, protocol, `ws`, `better-sqlite3` | Rooms, tick scheduler, input queue, snapshot broadcast, leaderboard, replay storage, static hosting of the web build |
| `@nlsnipes/web` | browser | core (worker), protocol | Vite app: title/lobby UI, `LocalTransport` (worker) and `WsTransport`, Canvas glyph renderer, input mapper, audio synth, themes, replay viewer |
| `@nlsnipes/tui` | Node 22 | protocol | Terminal client: connects to a server (or spawns a local one in-process for solo), ANSI diff renderer |
| `@nlsnipes/bots` | Node 22 | core, protocol | Scripted and random players for tests, load and balance runs |

### Runtime data flow

1. Client samples input every animation frame and coalesces it into one `InputFrame` per sim tick: an 8-bit mask (move U/D/L/R, fire U/D/L/R) plus `fast` flag and the client's tick number.
2. Transport delivers `InputFrame`s to the authority (worker or server). The authority applies inputs for tick *t* when it runs tick *t*; inputs arriving late are applied on the next tick (no rewind — at 18 Hz a 55 ms window is generous on LAN/regional play).
3. Authority runs `step()` once per 55.5 ms using a drift-corrected scheduler (accumulate wall-clock, run 0..N steps, cap N at 4 to avoid spiral of death).
4. After each tick the authority emits a `Snapshot`: full state every 36 ticks (2 s) or on join, otherwise a `Delta` (changed tiles as run-length list, changed/created/removed entities, HUD counters). Snapshots carry the tick and a 32-bit state hash.
5. Clients render the latest snapshot; the renderer never simulates. A client compares the authority's hash against its own only in *debug determinism mode* (client runs a shadow core) — used in CI, off in production.
6. Replays are `seed + config + ordered InputFrames`. Playback re-runs the core; the recorded final hash must match.

### Server internals

- **Lobby (HTTP + JSON over WS):** create room (returns 5-letter code), join, list players, set mode/skill, start. Rate-limited per IP. No accounts; nickname only.
- **Room:** owns one `GameState`, a per-player input ring buffer, the scheduler, and connected sockets (players + spectators). Ends on game over or 60 s empty. Hard caps: 8 players, 16 spectators, 500 rooms per process.
- **Broadcast:** one encoded frame per tick per room, fanned out to all sockets; per-socket backpressure check drops spectators first.
- **Persistence:** SQLite tables `scores(skill, mode, nick, score, ticks, seed, created_at)` and `replays(id, seed, config, inputs BLOB, final_hash)`. Daily Seed derives from `SHA-256(date + server secret)`.
- **Health:** `/healthz`, Prometheus-style `/metrics` (rooms, ticks/s, p99 step time, bytes out).

### Client internals (web)

- **Renderer:** glyph atlas (256 CP437 glyphs × 16 colours pre-tinted lazily), dirty-cell rendering into an offscreen canvas at 1 : 1 glyph scale, then integer-scaled blit; optional WebGL CRT pass (scanlines, curvature) on the final blit.
- **Viewport:** Classic = 40 × 22 cells centred on the local player with toroidal wrap; Wide = as many cells as fit. Camera logic is in the client; the core is unaware of viewports.
- **Input mapper:** key bindings → 8-bit mask; gamepad polling; touch: two virtual sticks (left move, right fire), 8-way quantised.
- **Audio:** Web Audio `OscillatorNode` square wave, envelope per event (shot, hit, hive, death, spawn), master volume, resumes on first user gesture.
- **UI shell:** title, skill picker (A–Z × 1–9 grid with a tooltip describing flags), lobby, HUD overlay, pause, results, settings, replay viewer. Plain DOM + CSS; no framework.

### Determinism contract

` hash(step(s, i))  ` is identical on V8 (Node), V8 (Chromium), SpiderMonkey and JavaScriptCore for the same `(s, i)`. Guaranteed by: integer-only math (`| 0`, `>>> 0`), no object-key-order dependence (arrays, not maps, for iteration), no `Date`, no `Math.random`, no floating-point accumulation. CI runs the same replay in Node and headless Chromium and compares hashes.

## Detailed specifications

These are the numbers and shapes the coding agents implement verbatim. Where the original is ambiguous, the choice made here is marked *(decision)*.

### Coordinate system and grid

- Tile grid `W = 128`, `H = 120` characters; `(x, y)` with `x` to the right, `y` down; **all arithmetic modulo W/H** (toroidal). Helper `wrap(x, y)`.
- Cell grid `CW = 16`, `CH = 20`, cell size `8 × 6`. Walls live on tile rows/columns that are multiples of 6 / 8; the cell interior is 7 × 5 walkable tiles plus the corridor openings.
- Tile kinds: `Empty`, `Wall(variant)`, `Debris` (destroyed hive/snipe remnant, walkable, decorative, fades after 18 ticks *(decision)*). Entities are not stored in tiles; occupancy is computed from an entity spatial index per tick.

### Maze generation (deterministic)

```text
function generateMaze(rng):
  cells[CW*CH] = all four walls set (bitmask N=1,E=2,S=4,W=8)
  visited = {cell 0}
  // randomized spanning tree by random walk with random restarts (matches original's spirit)
  while |visited| < CW*CH:
    c = random unvisited-adjacent frontier cell chosen via rng
    d = rng.pick(directions where neighbour(c,d) is visited)   // neighbour wraps toroidally
    remove wall between c and neighbour(c,d); visited.add(c)
  // extra openings: remove K = 3 + rng.int(3) random walls that still exist  (decision: original removes a small random number)
  rasterize: for each cell draw remaining walls with double-line box glyphs; corner glyph chosen from the 4-neighbour wall mask
```

Invariants tested by property tests: every cell reachable from every other (BFS on cell graph); wall count after rasterization equals expected; generation for a given seed is byte-identical across runs.

### PRNG

`sfc32` or `mulberry32` seeded from a 32-bit seed; state stored in `GameState.rng`. API: `nextU32()`, `int(nMax)` (unbiased via rejection), `mask(m)` (`nextU32() & m`, used where the original used a bitmask draw). All entity decisions consume RNG in a fixed order: hives → large snipes → small snipes → weapons, by ascending entity id.

### Entities

| Entity | Size | Speed | Notes |
| --- | --- | --- | --- |
| Player | 2 × 2 | 1 tile per tick; 2 per tick while `fast` | Fire cooldown 4 ticks *(original: 2 frame-pairs)*; bullet spawns at the sprite edge facing the fire direction; dies on collision with snipe, spear, other player's bullet (Deathmatch), or wall when `electricWalls`; respawn after 36 ticks at the original spawn cell if lives remain |
| Hive | 2 × 2 | static | `spawnTimer` starts 1, reloads to `5 + (orthoDistToNearestPlayer >> (tick/256 + 1))` ticks; on expiry, if `snipesAlive < maxSnipes` and `rng.mask(0xF >> (hivesAtStart − hivesAlive)) == 0`, spawn a large snipe adjacent to the hive with random heading |
| Large snipe | 2 × 1 | 1 tile every 2 ticks | Heading in 8 directions; each move: 3/4 keep heading, else turn ±45° in a per-snipe fixed turn direction, and with a further 1/4 pick a random heading; blocked → turn. Fires a spear when the player is within 20 tiles orthogonally and `rng.mask(0xFFFF >> (15 − accuracy))` is 0 *(interpretation of the original shift)*; spear direction = 8-way direction toward player |
| Small snipe | 1 × 1 | 1 tile per tick | Created when a large snipe is shot and `smallSnipes` flag is on: with probability `1/(explosionMask+1)` it explodes instead. Random heading, re-randomised on block |
| Bullet | 1 × 1 | 2 tiles per tick | Destroys first thing it enters: hive (+50), snipe (+1), player (Deathmatch), spear (both vanish). Diagonal bullet on `bounce` letters reflects off walls `1 + rng.mask(7)` times using the reflection table (orthogonal component of the wall normal flips) |
| Spear | 1 × 1 | 2 tiles per tick | Kills players; destroys hives unless `hivesResistSpears`; removed on wall |

Movement resolution order per tick: players (in id order), bullets, spears, large snipes, small snipes, hives. Collision is checked on each single-tile sub-step so 2-tile movers cannot tunnel.

### GameConfig from skill code

```ts
interface GameConfig {
  letter: 0..25; digit: 1..9;
  accuracy: number;           // table A..Z in section 2
  smallSnipes: boolean;       // table
  bounce: boolean;            // table
  explosionMask: number;      // table (0x7F / 0x3F / 0x1F / 0x0F)
  electricWalls: boolean;     // letter >= 'M'
  hivesResistSpears: boolean; // letter >= 'W'
  maxSnipes: number; hives: number; lives: number; // digit tables
  mode: 'classic' | 'coop' | 'deathmatch'; players: 1..8;
  friendlyFire: boolean; fragLimit: number; timeLimitTicks: number;
}
```

### GameState (authoritative)

`{ tick, rng, config, tiles: Uint8Array(W*H), tileVariant: Uint8Array, entities: Entity[], nextEntityId, players: PlayerState[], score, hivesAlive, snipesAlive, phase: 'lobby'|'running'|'won'|'lost'|'ended', events: Event[] }` — `events` is cleared each tick and drives client audio/FX (`shot`, `hit`, `hiveDown`, `playerDied`, `spawn`, `bounce`).

`hash(state)` = FNV-1a 32 over `tick, rng, tiles, entities (id, kind, x, y, dir, timers), players (x, y, lives, score), score, counters`.

### Input

`InputFrame = { tick: u32, mask: u8, fast: bool }` with mask bits `0 moveR, 1 moveL, 2 moveD, 3 moveU, 4 fireR, 5 fireL, 6 fireD, 7 fireU`. Opposing bits cancel. Direction derived from the resulting 2-axis vector (8-way). Default keyboard: arrows move, WASD fire, Space fast, P pause, ? help; all remappable. Gamepad: left stick move, right stick fire, RT fast.

### Wire protocol (`@nlsnipes/protocol`)

Control channel = JSON text frames; game channel = msgpack binary frames. All frames carry `v: 1` (protocol version); mismatched versions are rejected at handshake.

| Direction | Message | Fields |
| --- | --- | --- |
| C→S | `hello` | \`v, nick, clientKind: 'web' |
| S→C | `welcome` | `playerId, serverTick, tickMs` |
| C→S | `createRoom` / `joinRoom` | `mode, skill: 'A1'..'Z9'` / `code` |
| S→C | `room` | `code, players[{id,nick,ready}], mode, skill, hostId` |
| C→S | `setReady`, `setSkill`, `start` | — |
| S→C | `start` | `seed, config, startTick` |
| C→S | `input` (binary) | `InputFrame[]` (batched, ≤ 4 per frame) |
| S→C | `snapshot` (binary) | `tick, hash, full: boolean, tiles?: RLE, entities: Entity[], players, hud` |
| S→C | `delta` (binary) | `tick, hash, tileChanges: [idx, kind, variant][], upsert: Entity[], remove: id[], players, hud, events` |
| S→C | `gameOver` | `result, scores[], replayId` |
| both | `ping`/`pong` | `t` |
| S→C | `error` | `code, message` |

Size budget: full snapshot ≤ 4 KB (RLE tiles \~2 KB + ≤ 200 entities × 6 B); delta typically < 300 B; at 18 Hz and 8 players a room emits < 50 KB/s.

### Replay file (`.nlsr`)

msgpack: `{ v: 1, seed, config, players: [{nick}], inputs: [[tick, playerIdx, mask, fast]...], finalTick, finalHash }`. Playback validates `finalHash`; mismatch = determinism bug, fail loudly.

### Rendering spec

- Glyphs: player `☻` over `╨`-like legs (CP437 0x01/0x02 and 0x11/0x10 in the original), large snipe `☺` + arrow (0x18–0x1B), small snipe `☻`, hive box corners (0xDA 0xBF 0xC0 0xD9), bullet `○`/`·` (0x09/0x0F), spear arrows/slashes (0x18–0x1B, 0x2F, 0x5C), walls double-line set (0xB9–0xCE), debris `▓` (0xB2).
- CGA palette indices as in the original: walls 9 (light blue), player 15, large snipe 2, bullet 14, spear 10, hive per-hive colour. Themes remap indices → RGB.
- HUD (3 rows): `Skill A1 · Time mm:ss · Men Left n · Score nnnnn` plus multiplayer roster in Wide view.

## Implementation plan

Seven milestones, one pull request each, each gated by CI; an agent may not start milestone *n + 1* until *n* is merged. Milestones M1 and M2 are pure core work and produce no UI, which keeps the highest-risk logic testable before any pixel is drawn.

&#91;embedded content: milestone roadmap · 7 phases, 7 gates, one parallel branch\]

Each diamond is a CI gate from the table above; nothing below a diamond starts until it is green.

| # | Milestone | Deliverables | Acceptance gate (all automated unless marked) |
| --- | --- | --- | --- |
| M0 | Scaffold and CI | pnpm workspace with the six packages (empty), Biome, Vitest, Playwright, GitHub Actions (typecheck · lint · unit · e2e · determinism · similarity-check jobs), `CLAUDE.md`, `CONTRIBUTING.md`, MIT `LICENSE`, `NOTICE`, issue templates, PR template, Dependabot | CI green on an empty test; `pnpm -r build` succeeds; similarity job runs against `reference/` and passes |
| M1 | Core: world | PRNG, `GameConfig` from skill code with all tables, maze generator + rasterizer, `GameState`, `hash`, `serialize/deserialize`, ASCII dump helper for tests | Property tests: connectivity for 1 000 seeds, wrap correctness, byte-identical regeneration; table tests for all 234 skill codes; `hash` stable across 100 serialize round-trips; 100 % branch coverage on tables |
| M2 | Core: play | Entities, movement, collisions, weapons incl. bounce, hives and spawning, snipe AI, small snipes, electric walls, scoring, lives, win/lose, Co-op and Deathmatch rules, events, replay record/playback | 10 000-tick random-bot games at A1, M5, Z9 finish without exceptions; replay of each replays to the same `finalHash`; `step()` p99 < 1 ms on CI runner at Z9 with 8 bots; golden ASCII snapshots for 5 seeds at ticks 0/100/1000 |
| M3 | Web solo | Vite app, glyph atlas renderer, viewport with wrap, input mapper (keyboard), audio synth, worker `LocalTransport`, title + skill picker + HUD + results, local best scores | Playwright: start A1, move, shoot, destroy a hive (score 50) using a scripted seed; render 60 fps with < 4 ms frame time in headless Chromium trace; Node-vs-Chromium hash equality on a 5 000-tick replay |
| M4 | Protocol and server | `@nlsnipes/protocol` schemas + codec, `ws` server, rooms and codes, tick scheduler, snapshot/delta encoding, Co-op and Deathmatch, spectators, SQLite scores and replays, `/healthz`, `/metrics`, Docker image | Bot integration test: 8 bots in one room for 3 600 ticks with zero desync (client shadow hash == server hash every 36 ticks); 50 rooms × 8 bots keep tick jitter p99 < 10 ms; fuzzed malformed frames never crash the process; image builds and serves `/` |
| M5 | Multiplayer UI, terminal client, replays | Lobby screens, in-game roster, chat-less emotes (optional), reconnection with resume, replay viewer with scrub, Daily Seed leaderboard page, `@nlsnipes/tui` client incl. solo-in-process mode | Playwright: two browser contexts create/join a room and both see the same hive destroyed; TUI smoke test under `node-pty` renders a frame containing `Skill`; reconnect test resumes within 2 s |
| M6 | Polish and release 1.0 | Themes (CGA, Amber, Green, High-contrast), optional CRT pass, gamepad and touch input, tutorial, help overlay, settings persistence, accessibility pass (focus order, reduced motion, colour-blind safe theme), README with GIFs, `docker compose`, tagged release | Lighthouse accessibility ≥ 95; all previous suites green; manual 30-minute play session log signed off by the owner *(manual)*; `v1.0.0` tag + GitHub Release with image |

### Working agreement for agents

- Branch `m<n>/<slug>`; PR title `M<n>: <milestone>`; PR body must fill the acceptance-gate checklist.
- Commit after every green test run; small commits; conventional commit prefixes (`feat:`, `fix:`, `test:`, `chore:`).
- If a gate cannot be met, open an issue labelled `blocked` explaining why and stop; do not lower the gate.
- Never edit `reference/` (vendored read-only copy of the reference port used by the similarity job); never copy from it.
- Ask the owner only through GitHub issues labelled `question`; otherwise decide, record the decision in `docs/decisions/NNNN-<slug>.md`, and continue.

### Effort estimate

Sized for an autonomous Opus-class agent with CI feedback: M0 ½ day, M1 1 day, M2 2–3 days, M3 2 days, M4 2–3 days, M5 2 days, M6 2 days — roughly two weeks of agent time, parallelisable after M2 (M3 and M4 are independent).

## Testing strategy

Determinism is the test oracle: because the same seed and inputs must always produce the same state, most of the suite is "run it, hash it, compare". Agents get objective pass/fail from CI on every push, and every bug becomes a replay file that reproduces exactly.

| Layer | Tool | What it proves | Runs |
| --- | --- | --- | --- |
| Unit | Vitest | Tables, PRNG sequences, wrap math, direction/mask conversion, bounce reflection table, codec round-trips | every push, < 30 s |
| Property | Vitest + fast-check | Maze connectivity and wall counts for arbitrary seeds; `deserialize(serialize(s)) == s`; delta applied to previous snapshot == next full snapshot; input mask normalisation is idempotent | every push |
| Golden | Vitest snapshot files | ASCII dumps of maze and entities at fixed ticks for 5 seeds × 3 skill codes; changes require an explicit `--update` with a justification in the PR | every push |
| Simulation | `tsx` scripts in `@nlsnipes/bots` | Random and scripted bots play 10 000 ticks at A1, M5, Z9; asserts no exceptions, invariants (snipes ≤ maxSnipes, hives never negative, lives monotone), and `finalHash` reproducibility | every push (3 min) and nightly with 1 000 seeds |
| Cross-engine | Node + Playwright (Chromium, Firefox, WebKit) | The same replay yields the same hash in all four engines | every push (Chromium), nightly (all) |
| Integration | Vitest + in-process server + bot WS clients | Rooms, join/leave, late join, spectator, reconnect, 8 bots with shadow-core hash check every 36 ticks, malformed-frame fuzzing | every push |
| E2E | Playwright | Solo: start A1, kill a hive on a fixed seed, see score 50; Multi: two contexts share a room; Replay viewer loads a `.nlsr`; Settings persist | every push (Chromium) |
| Performance | Vitest bench + Playwright trace | `step()` p99 < 1 ms at Z9 with 8 players; render frame < 4 ms; 50 rooms tick jitter p99 < 10 ms | nightly, regression threshold 20 % |
| Accessibility | Playwright + axe-core, Lighthouse CI | No critical axe violations; Lighthouse a11y ≥ 95 | M6 and nightly |
| Licence hygiene | Custom script (`scripts/similarity-check.ts`, winnowing fingerprints) | No 40-token window in `packages/**` matches `reference/**` | every push |

### Rules that make the suite cheap

- Every test that touches the core takes an explicit seed; failing tests print the seed and a replay path.
- Bots are the primary QA player: `RandomBot` (uniform mask changes every 6 ticks), `HunterBot` (BFS to nearest hive, fires when aligned), `CowardBot` (maximise distance from snipes). `HunterBot` must win A1 within 5 000 ticks on ≥ 90 % of seeds — this doubles as a balance check.
- Visual regression is limited to the ASCII golden dumps; pixel screenshots are not compared (font rendering differs per OS).
- CI matrix: Ubuntu only for every push; Windows and macOS on nightly to catch path/line-ending issues in the TUI.

### Definition of done for a bug

A failing replay committed under `tests/replays/regressions/<issue>.nlsr`, a fix, and the replay now passing. No bug is closed without its replay.

## Agent handoff package

Everything a coding agent needs lives in the repository, not in this document: this section is the content of those files. Once the repo exists, export this doc as Markdown to `docs/DESIGN.md` and paste the two blocks below into `CLAUDE.md` and `docs/PROMPT.md`.

### Repository layout

```text
nlsnipes/
  CLAUDE.md                 agent instructions (below) — Claude Code reads this automatically
  README.md                 what it is, how to play, how to run
  LICENSE                   MIT
  NOTICE                    credits: SuperSet Software (1982), Davidebyzero reverse-engineering, font licence
  package.json / pnpm-workspace.yaml / biome.json / tsconfig.base.json / turbo.json
  .github/workflows/ci.yml  typecheck · lint · unit · property · sim · e2e · cross-engine · similarity
  .github/ISSUE_TEMPLATE/   bug (requires replay), question, blocked
  docs/
    DESIGN.md               this document (exported Markdown)
    PROMPT.md               master prompt for a fresh agent session
    decisions/              ADRs written by agents: 0001-prng-choice.md ...
    protocol.md             generated from zod schemas (pnpm docs:protocol)
  reference/                READ-ONLY vendored copy of Davidebyzero/Snipes for behaviour lookup and the similarity job
  packages/
    core/      src/{rng,config,tables,maze,state,entities,step,hash,serialize,replay}.ts  tests/
    protocol/  src/{messages,codec,version}.ts
    server/    src/{index,lobby,room,scheduler,snapshot,db,metrics}.ts  Dockerfile
    web/       index.html src/{main,transport,worker,render,input,audio,ui,themes,replay}.ts  e2e/
    tui/       src/{index,render,input}.ts
    bots/      src/{random,hunter,coward,harness}.ts
  tests/replays/            golden and regression .nlsr files
  scripts/                  similarity-check.ts, gen-protocol-docs.ts, bench.ts
  docker-compose.yml
```

### `CLAUDE.md` (paste verbatim)

```markdown
# NLSNIPES Reborn — agent instructions

You are implementing a faithful, MIT-licensed re-creation of the 1982 text-mode game Snipes
with modern multiplayer. The full design is in docs/DESIGN.md; read sections 3, 5, 6 and 7
before writing code. The current milestone is tracked in the GitHub issue labelled `milestone`.

## Hard rules
1. packages/core is pure: no imports from node:*, DOM, or any npm package. Integer math only.
   No Math.random, no Date. Every random draw goes through GameState.rng.
2. Never copy code from reference/ or any Snipes clone. Read it to understand behaviour, then
   write your own implementation. CI runs a similarity check; if it fails, rewrite.
3. Determinism is sacred. If a change alters hashes of golden replays, either the change is a
   deliberate rule change (update goldens, explain in PR) or it is a bug (fix it).
4. One milestone per PR. Fill the acceptance-gate checklist in the PR body. Do not start the next
   milestone until the current PR is merged.
5. Do not lower or skip a gate. If blocked, open an issue labelled `blocked` and stop.
6. Write an ADR in docs/decisions/ for any choice not settled by DESIGN.md.

## Commands
- pnpm i                        install
- pnpm -r build                 build all packages
- pnpm test                     unit + property + golden (fast)
- pnpm test:sim                 bot simulation games
- pnpm test:e2e                 Playwright
- pnpm test:all                 everything CI runs
- pnpm lint / pnpm format       Biome
- pnpm dev                      web client + local server with hot reload
- pnpm replay <file.nlsr>       verify a replay in Node

## Conventions
- TypeScript strict; no `any`; prefer readonly types; arrays over Maps for iteration order.
- Tests next to code in tests/; one seed per test, printed on failure.
- Conventional commits; branch m<n>/<slug>; small commits after each green run.
- Skill code strings are always uppercase letter + digit, e.g. "M5".
- Tick = 1000/18 ms. Never hardcode 55 or 56; import TICK_MS from core.

## Numbers you will need (authoritative copies live in docs/DESIGN.md §2 and §6)
- Grid 128×120 toroidal; cells 16×20 of 8×6. Viewport 40×22 + 3 HUD rows.
- Digit tables: maxSnipes 10 20 30 40 60 80 100 120 150; hives 3 3 4 4 5 5 6 8 10; lives 5 5 5 5 5 4 4 3 2.
- Letter tables: see DESIGN.md §2 (accuracy, smallSnipes, bounce, explosionMask); electricWalls ≥ M;
  hivesResistSpears ≥ W.
- Score: +1 snipe, +50 hive. Deathmatch: +10 frag, −5 death.
```

### Master prompt (`docs/PROMPT.md`, paste into a fresh Claude Code / Opus 5.5 session)

```markdown
You are the lead engineer on NLSNIPES Reborn. Repository: <owner>/nlsnipes (you have write access).

Context: read CLAUDE.md, then docs/DESIGN.md. Then read the open issue labelled `milestone` — that is
your task. If no such issue exists, create issues M0–M6 from DESIGN.md §7 (title "M<n>: <name>",
body = deliverables + acceptance-gate checklist), label M0 `milestone`, and start on it.

Working loop for the current milestone:
1. Plan: list the files you will create/modify and the tests that prove each gate item. Post the plan
   as a comment on the milestone issue.
2. Implement in small steps. After each step run `pnpm test`; commit when green.
3. When every gate item passes locally, run `pnpm test:all`, push branch m<n>/<slug>, open a PR titled
   "M<n>: <name>" with the checklist filled in, and wait for CI.
4. If CI fails, fix and push. If a gate is impossible, open a `blocked` issue with evidence and stop.
5. When the PR is merged, move the `milestone` label to the next issue and repeat.

Quality bar: code a senior engineer would merge without comments; tests that fail for the right reason;
no TODOs left in merged code; ADRs for judgement calls.

Safety and scope: only touch this repository; never commit secrets; never run destructive git commands
(force-push, history rewrite) on main; do not add dependencies beyond those named in DESIGN.md §4
without an ADR.

Begin by confirming, in one short comment on the milestone issue, which milestone you are starting and
your plan.
```

### Parallelising with several agents

After M2 is merged, run two sessions: one on M3 (branch `m3/web-solo`) and one on M4 (`m4/server`). They share `core` and `protocol` read-only; only M4 may modify `protocol`. If M3 needs a protocol change it opens an issue labelled `protocol` for the M4 agent. M5 starts only when both are merged.

### Handing this document over

The least-capable path: give the sibling model the repo URL and the sentence "Read CLAUDE.md and follow docs/PROMPT.md." Everything else is discoverable from there. For a model without repository access, attach the exported `DESIGN.md` and the two blocks above.

## GitHub setup and secure access for Claude

Create the empty repository yourself, then grant a token that can see only that one repository, with a short expiry. GitHub fine-grained tokens can be restricted to named repositories, but only to repositories that already exist — which is why the repo comes first and the token second. There is no GitHub connector in this Claude directory, so a token is the only route for this chat; Claude Code sessions later use your own `gh` login on your machine and need nothing from this step.

### Steps (10 minutes)

1. GitHub → New repository → name `nlsnipes`, Private (switch to Public at release), tick "Add a README" so `main` exists. Do not add a licence yet; the agent adds MIT + NOTICE in M0.
2. Settings (your profile) → Developer settings → Personal access tokens → **Fine-grained tokens** → Generate new token:
   - Token name `claude-nlsnipes`, Expiration **7 days** (renew if needed).
   - Resource owner: your user.
   - Repository access: **Only select repositories → nlsnipes**.
   - Repository permissions: Contents **Read and write**, Pull requests **Read and write**, Issues **Read and write**, Workflows **Read and write** (needed to push `.github/workflows/ci.yml`), Metadata Read (auto). Everything else "No access". Account permissions: none.
3. Repository → Settings → Branches → add a ruleset for `main`: require a pull request, require status checks to pass (add the CI job names after M0 lands), block force pushes. This protects `main` from any agent, including me.
4. Paste the token into this chat only when you want me to push. I will use it as `https://x-access-token:<token>@github.com/<you>/nlsnipes.git` for the session, never write it to a file or commit, and I have no memory of it after the conversation. Revoke it in the same settings page when the scaffold is in.
5. Optional, stricter: instead of a token, add a repository **Deploy key** (Settings → Deploy keys → Allow write access) with an SSH keypair generated for the purpose; give me the private key the same way. Deploy keys cannot touch issues or PRs, so use it only for the initial push.

### What the token cannot do, by construction

- See or touch any other repository, your organisation (LIF) repositories, or account settings.
- Push to `main` once the ruleset is in place (it must open a PR).
- Outlive 7 days.

### What I will do with it, once you paste it

1. Clone `nlsnipes`, create branch `m0/scaffold`.
2. Commit the M0 skeleton: workspace, package stubs, Biome/Vitest/Playwright config, CI workflow, `CLAUDE.md`, `docs/PROMPT.md`, `docs/DESIGN.md` (this document exported), `LICENSE`, `NOTICE`, issue and PR templates, and `reference/` as a documented Git submodule pointer rather than a copy (keeps the copyright status unambiguous).
3. Open PR "M0: Scaffold and CI" and create issues M0–M6 with the acceptance checklists.
4. Report the PR link here; you merge it after CI is green, then revoke the token.

## Open questions and references

### Decisions I would like you to confirm (defaults in bold)

- [ ] Bit-exact compatibility with 1982 replays: **out of scope for v1** (would require reproducing the original 16-bit LCG and PL/M quirks); faithful rules only.
- [ ] Public or private repo at start: **private**, public at v1.0.
- [ ] Hosting target for the Docker image: **a VM you run** (fits your Proxmox/VMware estate); no cloud account needed.
- [ ] Default language of the UI: **English**, with Czech as the second locale in M6 (string table from day one).
- [ ] Project name shown to players: **"NLSNIPES"** with subtitle "Reborn"; the trademark position of Novell/OpenText on the name is unknown — a neutral fallback ("Hivebreaker") is reserved if it ever matters.

### Follow-ups after v1 (not planned, compatible with the architecture)

Accounts and friends lists, ranked matchmaking, custom mazes and a level editor, Steam/itch packaging via Electron or Tauri, mobile store builds, mod hooks for entity behaviour.

### Sources (opened while writing)

- [Davidebyzero/Snipes on GitHub](https://github.com/Davidebyzero/Snipes) — reverse-engineered port; tables and mechanics in §2 and §6 were read from `Snipes.cpp` and `Snipes.h` on 2026-09-30
- [VOGONS thread by the port's author](https://www.vogons.org/viewtopic.php?p=515052) — history of the mono/colour/NLSNIPES variants, 40 × 25 text mode, spacebar speed
- [Network World: Drew Major on Snipes](https://www.networkworld.com/article/2297960/novell-and-the-computer-game-that-changed-networking.html) — origin, PL/M, 18 fps, first networked PC program
- [Wikipedia: Snipes (video game)](<https://en.wikipedia.org/wiki/Snipes_(video_game)>) — controls, level scheme, shared-drive networking, successor NetWars
- [lsnipes man page](https://mankier.com/6/snipes) — feature ladder per letter in a Linux clone, cross-check of the tables
- [Home of the Underdogs: Snipes](https://homeoftheunderdogs.net/game.php?id=3265) — A1 vs Z9 description
