# 0001 — Build NLSNIPES Light first, in Go

Status: accepted · 2026-10-09

## Context
docs/DESIGN.md plans a TypeScript monorepo with a browser client, WebSocket server, SQLite and
Docker across seven milestones (M0–M6). docs/DESIGN-LIGHT.md cuts that to one terminal
executable with LAN auto-join over UDP, five milestones (L0–L4), and recommends Go.

## Decision
- The first release is Light. DESIGN.md stays in the repo as the long-term plan and remains
  authoritative for game rules and specs (§2, §6.1–§6.7, §6.10).
- Language: Go (module `github.com/radimlif/nlSnipes`, `go 1.23` minimum), standard library
  plus tcell. Static binaries for windows/amd64, darwin/arm64, darwin/amd64, linux/amd64.
- Milestones are named L0–L4, branches `l<n>/<slug>`, PRs `L<n>: <name>`.
- The other Light defaults are taken as written: friendly fire on, new maze after a win or
  wipe-out, UDP port 5108, `--game` for separate games on one subnet.

## Consequences
DESIGN.md's TypeScript tooling (pnpm, Biome, Vitest, Playwright) does not apply. If the owner
switches to TypeScript + Bun later, only L0's tooling and this ADR change; the design is
language-neutral.
