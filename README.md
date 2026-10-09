# nlSnipes

NLSNIPES Light: the 1982 text-mode maze shooter *Snipes* as one small executable.
Launch it on any machine on your LAN. If a game is already running on the subnet you join it;
otherwise you host and the game starts. Up to 4 players, everyone else spectates.

**Status:** scaffold (milestone L0). Not playable yet.

## Build

```sh
git clone --recurse-submodules https://github.com/radimlif/nlSnipes.git
cd nlSnipes
go test ./...
go run ./cmd/nlsnipes
```

Requires Go 1.23 or newer.

## Documents

- [docs/DESIGN-LIGHT.md](docs/DESIGN-LIGHT.md): what is being built now
- [docs/DESIGN.md](docs/DESIGN.md): full research and long-term design; authoritative for game rules
- [CLAUDE.md](CLAUDE.md): instructions for coding agents · [docs/PROMPT.md](docs/PROMPT.md): master prompt
- [docs/decisions/](docs/decisions/): architecture decision records

## Licence

MIT, see [LICENSE](LICENSE) and [NOTICE](NOTICE). Snipes is by SuperSet Software (1982); this is an
independent re-creation containing none of its code.
