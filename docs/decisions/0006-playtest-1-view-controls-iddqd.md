# 0006 — First playtest: full view, diagonal controls, emulated holds, IDDQD

Status: accepted · 2026-10-09 · from the owner's first hand playtest of L2

| Feedback | Decision |
| --- | --- |
| "Movement is fluid at the start, then stutters, then fluid again" | In terminals without key releases, a press now counts as held right through the learned OS repeat delay instead of a 220 ms tap window, so holds never stall. While a hold is only a guess (no repeat seen yet), the client drops any move that would put the player against a wall — a tap can still overshoot along a corridor but never into an electric wall. |
| "The maze is quite small; the PC has more space" | The default **full view** fills the terminal, up to 120 × 113 (larger would show the toroidal maze twice). **Classic view** is the original 40 × 25: `V` toggles in game, `-classic` starts in it; kept for small screens and a mobile client later. This supersedes DESIGN-LIGHT.md's "never scales" for the viewport size; glyphs are still never scaled. |
| "It's hard to shoot diagonally" | `Q E Z C` fire diagonally (the keys around `W A S D`). Moving diagonally also has single keys: numpad 7 9 1 3 (with 8 2 4 6 for straight moves) and Home / PgUp / End / PgDn. Two-key chords still work where the terminal reports holds. |
| "Typing IDDQD should give bouncing shots, mirror-like, for an edge against the hordes" | **Mirror shots**: `core.Player.Mirror`, toggled by `Input.ToggleMirror`. Every shot of that player gets `MirrorBounces` = 8 wall bounces in any direction, reflecting like light — a flat hit comes straight back, a diagonal flips the component that hit. It is part of the deterministic state (in the hash and replays). The terminal client toggles it when IDDQD is typed during play and keeps it on across mazes. In LAN play (L3) the host decides whether to honour it; by default only a solo game does. |

Core changes: `Player.Mirror`, `Input.ToggleMirror`, `EvMirror`, state codec version 2, replay
flags byte (bit 0 fast, bit 1 toggle mirror — old replays still decode). Goldens regenerated.
