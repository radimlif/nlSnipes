#!/bin/sh
# Regenerates docs/demo.gif: a HunterBot game recorded through the real
# renderer (tools/castgen), drawn by agg (brew install agg) in CGA colours.
set -e
cd "$(dirname "$0")/.."
CGA="000000,aaaaaa,000000,aa0000,00aa00,aa5500,0000aa,aa00aa,00aaaa,aaaaaa,555555,ff5555,55ff55,ffff55,5555ff,ff55ff,55ffff,ffffff"
go run ./tools/castgen -out docs/demo.cast "$@"
agg --theme "$CGA" --font-family "Menlo,DejaVu Sans Mono,Consolas" --font-size 16 \
    --idle-time-limit 2 --fps-cap 18 docs/demo.cast docs/demo.gif
rm docs/demo.cast
