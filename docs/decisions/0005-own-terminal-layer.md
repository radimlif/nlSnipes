# 0005 — Own terminal layer instead of tcell, for real key releases

Status: accepted · 2026-10-09

## Context
Snipes is played by holding keys: arrows to move, W/A/S/D to fire, Space to run. Terminals
normally send a press, then (after the OS repeat delay of 250–660 ms) a stream of repeats, and
nothing on release. Guessing the release either overshoots — several tiles after a tap, fatal
next to electric walls from letter M — or stalls for the repeat delay every time you start
moving. Either way the game stops being fun, which is the owner's standing goal.

Real releases do exist: the Windows console API reports key-up for cmd.exe and Windows
Terminal, and the kitty keyboard protocol (kitty, WezTerm, Ghostty, foot, Alacritty, iTerm2)
reports press/repeat/release. tcell v2.13 receives the Windows key-ups and discards them, and
only enables the kitty protocol without event types, so it cannot deliver either.

## Decision
`term` is our own small layer, replacing tcell (DESIGN-LIGHT.md named tcell):
- Output: plain ANSI — alternate screen, 16 colours, cursor positioning — with a renderer that
  sends only changed cells and re-positions after every non-ASCII glyph so a terminal that
  draws a symbol double-width cannot shift a row. The 40 × 25 frame is centred, never scaled.
- Input on Windows: `ReadConsoleInputW` (key down, repeat, up), VT output and UTF-8 enabled
  through the console API.
- Input on Unix: raw mode via `golang.org/x/term`; query the kitty protocol at start and, if the
  terminal answers, push flags 1+2+8 for press/repeat/release on every key. Otherwise parse
  legacy VT sequences and emulate holds (`term.KeyState`): a first press holds for a 220 ms tap
  window, then a held key follows its repeats, with the repeat delay and interval learned from
  the terminal. A tap shorter than a tick still counts once.
- Tests use `term.Sim`, an in-memory terminal, in place of tcell's simulation screen; the L2
  gate "scripted-input hive kill through a simulation screen" runs through it unchanged in
  intent.

Dependencies: `golang.org/x/term` and `golang.org/x/sys` (Go project modules) replace tcell.

## Consequences
Smooth hold controls on Windows and modern Unix terminals; acceptable emulated controls on
macOS Terminal.app and xterm, where the title screen suggests a better terminal. We own about
600 lines of terminal code; the platforms that need manual checking are the four named in the
L2 gate.
