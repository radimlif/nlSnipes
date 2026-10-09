# 0003 — How the similarity check works

Status: accepted · 2026-10-09

## Context
The reference port (Davidebyzero/Snipes) is used as a behavioural spec; its copyright stays
with the original authors, so none of its code may enter this MIT repository. DESIGN.md §8
asks CI to reject any 40-token window in our code that matches the reference.

## Decision
- `reference/` is a git submodule pointing at the upstream repository, not a vendored copy.
  CI checks it out with `submodules: true`.
- `tools/simcheck` tokenises both trees into a language-neutral stream: identifiers lowercased,
  integer literals normalised to decimal (`0x7F` = `127`), string and char literals collapsed to
  one token, comments and semicolons dropped. A C++ fragment re-typed with Go syntax therefore
  still matches.
- Every 40-token window of the reference is fingerprinted (FNV-64a); every 40-token window of
  our `.go` files (outside `reference/`, `testdata/`, `dist/` and hidden directories) is
  looked up. Any hit fails the job with file and line ranges.
- Windows with fewer than 8 identifiers are skipped. The A1–Z9 skill tables are facts about the
  game and will legitimately appear as the same run of numbers in both codebases; this rule
  exempts pure data while still catching copied logic.

## Consequences
The check catches verbatim and transliterated copies, not a re-implementation of the same idea,
which is the intended line. The tests prove it catches a planted copy of the real reference and
leaves original code and data tables alone.
