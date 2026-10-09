# 0002 — Network package is `lan`, not `net`

Status: accepted · 2026-10-09

## Context
DESIGN-LIGHT.md names the four packages `core`, `net`, `term`, `app`. A package named `net`
must import the standard library's `net` for UDP sockets, which forces an import alias inside
it and makes every caller's imports ambiguous to read.

## Decision
The UDP protocol package is `lan` (`github.com/radimlif/nlSnipes/lan`). Its contents are exactly
what the design assigns to `net`.

## Consequences
None beyond the name. Docs referring to "`net` package" mean `lan`.
