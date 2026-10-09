# NLSNIPES Light — agent instructions

You are implementing NLSNIPES Light: a faithful, MIT-licensed re-creation of the 1982 text-mode
game Snipes as one small Go executable. Launch it on any LAN machine: if a game is beaconing on
the subnet you join it, otherwise you host. Up to 4 players, everyone else spectates.

Read before writing code:
- docs/DESIGN-LIGHT.md — what we are building now (scope, UDP protocol, milestones L0–L4).
- docs/DESIGN.md §2 and §6.1–§6.7, §6.10 — game rules, skill tables, specs. Still authoritative
  for the rules; translate its TypeScript shapes to Go structs one-to-one. Its web/server/Docker
  parts (§3.2, §3.4, §5, §6.8–§6.9, M3–M6) are NOT in scope.
- docs/decisions/ — choices already made.

The current milestone is the open GitHub issue labelled `milestone`.

## Hard rules
1. `core` is pure: standard library only, and not even `math/rand`, `time`, `os` or `net`.
   Integer math only — no floats. Every random draw goes through the state's own PRNG.
2. Never copy code from `reference/` (git submodule of Davidebyzero/Snipes, copyright retained by
   the original authors) or any other Snipes clone (lsnipes is GPL). Read for behaviour, then write
   your own. CI's `similarity` job fails on any shared 40-token window; if it fails, rewrite.
3. Determinism is sacred. Same seed + same inputs ⇒ same state hash on every OS and arch. If a
   change alters golden hashes it is either a deliberate rule change (update goldens, say why in
   the PR) or a bug.
4. One milestone per PR; fill in the acceptance-gate checklist. Don't start milestone n+1 until
   n is merged.
5. Never lower or skip a gate. If blocked, open an issue labelled `blocked` with evidence and stop.
6. Record any choice the design docs don't settle as an ADR in docs/decisions/NNNN-slug.md.
7. Dependencies: standard library plus `github.com/gdamore/tcell/v2` (in `term` only). Anything
   else needs an ADR.

## Layout
- `core/`   deterministic simulation (L1)
- `lan/`    UDP protocol: beacon/probe/join/welcome/input/snapshot/delta/bye, port 5108 (L3)
- `term/`   tcell renderer 40×25 + key input → mask (L2)
- `app/`    discover → host/join → play → migrate state machine (L2–L4)
- `cmd/nlsnipes/` the game binary · `cmd/simbot/` headless bot games for CI
- `tools/simcheck/` the similarity checker · `reference/` read-only submodule, never edit

## Commands
- `git submodule update --init`             fetch reference/ (needed by tests and simcheck)
- `go build ./...`                          build everything
- `go test ./...`                           unit + property + golden tests (fast)
- `go test -race ./...`                     what CI runs
- `go run ./cmd/simbot`                     bot games
- `go run ./tools/simcheck/cmd/simcheck`    similarity check against reference/
- `gofmt -l . && go vet ./...`              lint
- `GOTOOLCHAIN=go1.23.12 go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...`
                                            staticcheck as CI runs it (it can't read newer Go's export data)
- `go run ./cmd/nlsnipes M5`                play

## Conventions
- Branch `l<n>/<slug>`; PR title `L<n>: <milestone>`; conventional commits (`feat:`, `fix:`,
  `test:`, `chore:`, `docs:`); small commits after each green run.
- Table-driven tests; every test touching `core` uses an explicit seed and prints it on failure.
- Fixed-layout little-endian encoding (`encoding/binary`); no reflection-based serialisation in
  `core` or `lan`.
- Skill codes are uppercase letter + digit ("M5"); default "A1".
- 18 ticks per second: use `core.TicksPerSecond`, never a literal 55 or 56 ms.

## Numbers you will need (authoritative copies in docs/DESIGN.md §2 and §6)
- Grid 128×120 toroidal; 16×20 cells of 8×6. Viewport 40×22 + 3 HUD rows.
- Digit 1–9: maxSnipes 10 20 30 40 60 80 100 120 150; hives 3 3 4 4 5 5 6 8 10;
  lives 5 5 5 5 5 4 4 3 2.
- Letter tables (accuracy, smallSnipes, bounce, explosionMask): DESIGN.md §2.
  Electric walls for letters ≥ M; hives resist spears for letters ≥ W.
- Score: +1 snipe, +50 hive. Killing another player scores nothing (friendly fire on by default).
- 4 players max, slot colours white, yellow, cyan, magenta; spectators unlimited.
