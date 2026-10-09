package core

// randomGame plays ticks of random inputs (new mask every 6 ticks) and
// calls check after every tick.
func randomGame(cfg Config, seed uint32, ticks int, check func(s *State)) *State {
	s := NewGame(cfg, seed)
	r := NewRand(seed ^ 0xA5A5A5A5)
	var in [MaxPlayers]Input
	for i := 0; i < ticks && s.Phase == PhaseRunning; i++ {
		if i%6 == 0 {
			for p := range in {
				in[p] = Input{Mask: uint8(r.Next()), Fast: r.Mask(3) == 0}
			}
		}
		s.Step(in)
		if check != nil {
			check(s)
		}
	}
	return s
}

// arena is an open, wall-free state with joined players and no entities,
// for scripting exact situations.
func arena(skill string, players int) *State {
	cfg, err := NewConfig(skill, players, true)
	if err != nil {
		panic(err)
	}
	s := &State{Rng: NewRand(1), Cfg: cfg, Tiles: make([]uint8, GridWidth*GridHeight), NextID: 1}
	s.rebuildOcc()
	for p := 0; p < players; p++ {
		s.Players[p] = Player{Joined: true, Lives: int32(cfg.Lives), SpawnX: 3, SpawnY: 2}
	}
	return s
}

func (s *State) addPlayer(slot int, x, y int32) int {
	return s.spawn(Entity{Kind: KindPlayer, X: x, Y: y, Owner: int8(slot)})
}

func (s *State) addHive(x, y int32) int {
	s.HivesAlive++
	return s.spawn(Entity{Kind: KindHive, X: x, Y: y, Owner: -1, Timer: 1 << 20})
}

func (s *State) addSnipe(x, y int32, dir uint8) int {
	s.SnipesAlive++
	return s.spawn(Entity{Kind: KindSnipe, X: x, Y: y, Dir: dir, Turn: 1, Owner: -1, Timer: 1 << 20})
}

func (s *State) setWall(x, y int32) { s.Tiles[wrapY(y)*GridWidth+wrapX(x)] = TileWall }

func step(s *State, n int, in0 Input) {
	for i := 0; i < n; i++ {
		s.Step([MaxPlayers]Input{in0})
	}
}

func count(s *State, k Kind) int {
	n := 0
	for _, e := range s.Ents {
		if e.Kind == k && !e.Dead {
			n++
		}
	}
	return n
}
