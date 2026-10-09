## Milestone

L<n>: <name> — closes #<issue>

## Acceptance gate

<!-- Copy the gate items from the milestone issue; tick each with evidence (test name, CI job, log). -->

- [ ] ...

## Checks

- [ ] `gofmt`, `go vet`, `staticcheck` clean
- [ ] `go test -race ./...` green
- [ ] `simcheck` green (no code copied from reference/)
- [ ] Golden hashes unchanged, or the change is explained below
- [ ] ADRs added for decisions not settled by the design docs

## Notes
