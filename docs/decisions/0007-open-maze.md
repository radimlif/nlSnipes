# 0007 — An open maze: straight-run walker, 64 knock-outs, braided dead ends

Status: accepted · 2026-10-09 · from the owner's playtest ("I just can't find a way to reach my opponents")

## Context
DESIGN.md §6 specified a spanning tree plus "3 + rng(3)" extra openings — almost a perfect maze:
one route between any two cells. Measured over 200 seeds: walking routes 2.2× the straight
distance, 35 % of cells dead ends, the farthest cell 50 cells of walking away. Players could not
find their way to hives or to each other. The 1982 original (studied for behaviour in the
reference port) instead grows its tree from straight random runs of 1–4 cells and then knocks
out 64 random walls.

## Decision
`core.GenerateMaze` (own implementation):
1. **Walker tree**: join a random cell to the tree, then walk straight runs of 1–4 cells in
   random directions through untouched cells, carving, until blocked; repeat. Long corridors.
2. **64 knock-out attempts** on random walls, as the original — loops.
3. **Braid**: each remaining dead end gets a second exit with probability 12/16.

| Measure (200 seeds) | Before | Original-style (1+2) | **Now (1+2+3)** |
| --- | --- | --- | --- |
| Detour (route ÷ straight distance) | ×2.18 | ×1.89 | **×1.41** |
| Dead-end cells | 35 % | 22 % | **4 %** |
| Farthest cell | 50 | 53 | **32** |

`TestMazeIsEasyToGetAround` pins detour ≤ 1.5, dead ends ≤ 8 %, farthest ≤ 40. Connectivity for
1 000 seeds is still tested. HunterBot still wins A1 100/100 (mean 217 ticks, was 380); at
digit 5 the bot wins 30–96 % (was 20–73 %); digit 9 stays unbeaten. Goldens regenerated.
