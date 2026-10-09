# 0004 — How the core reads the open points in the rules

Status: accepted · 2026-10-09

DESIGN.md §6 fixes most numbers. Where it is silent, ambiguous or (in one case) wrong, the core
does the following. All of it is pinned by `core/testdata/golden.txt`; changing any rule here
means updating the goldens and saying why in the PR.

## Corrected from the reference port's behaviour

**Snipe spears.** DESIGN.md §6.4 says a snipe fires when the player is within 20 tiles and
`rng.mask(0xFFFF >> (15 − accuracy)) == 0`. Taken literally, letter A (accuracy 2) fires on 1 in 8
moves and letter Z (accuracy 5) on 1 in 64 — difficulty runs backwards, and the HunterBot gate
failed at A1 with 88 % of deaths from spears. Reading the reference port for behaviour (not
code) shows the shift applies to distance:

- a large snipe considers firing only along its current heading, and only when the nearest
  player lies on that line: same row or column, or close to the diagonal
  (`|6·|dx| − 8·|dy|| < 8`, which allows for 8 × 6 cells);
- `shift = (|dx| + |dy|) >> accuracy`; no shot when `shift > 10`; otherwise it fires with
  chance 1 in 2^(shift+1). Closer players and higher letters both mean more spears.

It decides after moving. The 20-tile range from DESIGN.md is dropped.

## Readings where the design is silent

| Situation | Core behaviour |
| --- | --- |
| Player and snipe (large or small) touch, whoever moved | Both die; no score |
| Player walks into a spear | Player dies, spear vanishes |
| Diagonal move blocked | Slide along the free axis, horizontal first |
| Player fires into an adjacent target | Hit applies immediately |
| Bullet meets a spear or another bullet | Both vanish |
| Player bullet hits another player | Kills them when friendly fire is on (default); never scores |
| Spear meets a snipe | Spear vanishes, snipe unharmed |
| Spear hits a hive | Hive destroyed, no score — unless letters W–Z |
| Large snipe shot on ghost letters | Becomes a small snipe at its position, unless `rng.Mask(explosionMask) == 0` (explodes) |
| Large snipe blocked | Turns 45° in its fixed turn direction, no move that step |
| Small snipe | Moves every tick; 1 in 8 random new heading; random heading when blocked |
| Hive spawn spot | Next to the hive in the drawn heading; skipped this time if not free |
| Hive placement | Distinct cell centres at least 3 cells (Chebyshev, wrapped) from every player |
| Respawn | After 36 ticks, at the spawn cell or the next cell (in cell order) with a clear 6 × 6 box |
| Debris | A 1 × 1 entity that never collides and disappears after 18 ticks |
| Entities created during a tick | Act from the next tick |
| Snipe count | Large and small snipes both count toward `maxSnipes` and the win condition |
