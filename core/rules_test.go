package core

import "testing"

func TestShootingAHiveScores50(t *testing.T) {
	s := arena("A1", 1)
	s.addPlayer(0, 10, 10)
	s.addHive(30, 10)
	s.addHive(80, 80) // a second hive so the game doesn't end
	step(s, 1, Input{Mask: FireR})
	step(s, 12, Input{})
	if s.HivesAlive != 1 || s.Score != 50 || s.Players[0].Score != 50 {
		t.Fatalf("hives %d score %d/%d", s.HivesAlive, s.Score, s.Players[0].Score)
	}
}

func TestLastHiveWithNoSnipesWins(t *testing.T) {
	s := arena("A1", 1)
	s.addPlayer(0, 10, 10)
	s.addHive(14, 10)
	step(s, 1, Input{Mask: FireR})
	step(s, 3, Input{})
	if s.Phase != PhaseWon {
		t.Fatalf("phase %d, want won", s.Phase)
	}
}

func TestOpposingBitsCancel(t *testing.T) {
	s := arena("A1", 1)
	i := s.addPlayer(0, 10, 10)
	s.addHive(80, 80)
	step(s, 5, Input{Mask: MoveR | MoveL | MoveD | MoveU | FireR | FireL})
	if e := s.Ents[i]; e.X != 10 || e.Y != 10 || count(s, KindBullet) != 0 {
		t.Fatalf("moved to %d,%d with %d bullets", e.X, e.Y, count(s, KindBullet))
	}
	if _, ok := MoveDir(MoveR | MoveD); !ok {
		t.Fatal("diagonal not decoded")
	}
}

func TestFastMovesTwoTiles(t *testing.T) {
	s := arena("A1", 1)
	i := s.addPlayer(0, 10, 10)
	s.addHive(80, 80)
	step(s, 3, Input{Mask: MoveR, Fast: true})
	if s.Ents[i].X != 16 {
		t.Fatalf("x = %d, want 16", s.Ents[i].X)
	}
}

func TestMovementWraps(t *testing.T) {
	s := arena("A1", 1)
	i := s.addPlayer(0, 0, 0)
	s.addHive(80, 80)
	step(s, 1, Input{Mask: MoveL | MoveU})
	if e := s.Ents[i]; e.X != GridWidth-1 || e.Y != GridHeight-1 {
		t.Fatalf("at %d,%d", e.X, e.Y)
	}
}

func TestWallsBlockOrKill(t *testing.T) {
	for _, tc := range []struct {
		skill string
		dies  bool
	}{{"L1", false}, {"M1", true}} {
		s := arena(tc.skill, 1)
		i := s.addPlayer(0, 10, 10)
		s.addHive(80, 80)
		s.setWall(12, 10)
		step(s, 2, Input{Mask: MoveR})
		if dead := s.PlayerEntity(0) < 0; dead != tc.dies {
			t.Fatalf("%s: dead=%v", tc.skill, dead)
		}
		if !tc.dies && s.Ents[i].X != 10 {
			t.Fatalf("%s: walked into the wall", tc.skill)
		}
	}
}

func TestDiagonalSlidesAlongWall(t *testing.T) {
	s := arena("A1", 1)
	i := s.addPlayer(0, 10, 10)
	s.addHive(80, 80)
	for x := int32(0); x < 30; x++ {
		s.setWall(x, 12) // floor under the player
	}
	step(s, 3, Input{Mask: MoveR | MoveD})
	if e := s.Ents[i]; e.X != 13 || e.Y != 10 {
		t.Fatalf("at %d,%d, want 13,10", e.X, e.Y)
	}
}

func TestSnipeContactKillsBothAndPlayerRespawns(t *testing.T) {
	s := arena("A1", 1)
	s.addPlayer(0, 10, 10)
	s.addHive(80, 80)
	s.addSnipe(12, 10, DirW)
	step(s, 1, Input{Mask: MoveR})
	if s.PlayerEntity(0) >= 0 || s.SnipesAlive != 0 || s.Players[0].Lives != 4 {
		t.Fatalf("player alive=%v snipes=%d lives=%d", s.PlayerEntity(0) >= 0, s.SnipesAlive, s.Players[0].Lives)
	}
	step(s, RespawnDelay-1, Input{})
	if s.PlayerEntity(0) >= 0 {
		t.Fatal("respawned early")
	}
	step(s, 1, Input{})
	if s.PlayerEntity(0) < 0 {
		t.Fatal("did not respawn")
	}
}

func TestLastLifeLoses(t *testing.T) {
	s := arena("Z9", 1)
	s.Players[0].Lives = 1
	s.addPlayer(0, 10, 10)
	s.addHive(80, 80)
	s.setWall(12, 10)
	step(s, 1, Input{Mask: MoveR})
	if s.Phase != PhaseLost {
		t.Fatalf("phase %d, want lost", s.Phase)
	}
}

func TestDiagonalBulletBouncesOnBounceLetters(t *testing.T) {
	for _, tc := range []struct {
		skill   string
		bounces bool
	}{{"A1", false}, {"G1", true}} {
		s := arena(tc.skill, 1)
		s.addPlayer(0, 10, 20)
		s.addHive(80, 80)
		for y := int32(0); y < GridHeight; y++ {
			s.setWall(16, y) // wall to the east
		}
		step(s, 1, Input{Mask: FireR | FireU})
		step(s, 2, Input{})
		var bullet *Entity
		for i := range s.Ents {
			if s.Ents[i].Kind == KindBullet {
				bullet = &s.Ents[i]
			}
		}
		if (bullet != nil) != tc.bounces {
			t.Fatalf("%s: bullet alive=%v", tc.skill, bullet != nil)
		}
		if bullet != nil && bullet.Dir != DirNW {
			t.Fatalf("%s: reflected to dir %d, want NW", tc.skill, bullet.Dir)
		}
	}
}

func TestSpearsKillHivesUnlessResistant(t *testing.T) {
	for _, tc := range []struct {
		skill string
		dies  bool
	}{{"V1", true}, {"W1", false}} {
		s := arena(tc.skill, 1)
		s.addPlayer(0, 100, 100)
		s.addHive(80, 80)
		s.addHive(20, 10)
		s.spawn(Entity{Kind: KindSpear, X: 14, Y: 10, Dir: DirE, Owner: -1})
		step(s, 4, Input{})
		if got := s.HivesAlive == 1; got != tc.dies {
			t.Fatalf("%s: hives %d", tc.skill, s.HivesAlive)
		}
		if s.Score != 0 {
			t.Fatalf("%s: spear kill scored %d", tc.skill, s.Score)
		}
	}
}

func TestFriendlyFire(t *testing.T) {
	for _, ff := range []bool{true, false} {
		s := arena("A1", 2)
		s.Cfg.FriendlyFire = ff
		s.addPlayer(0, 10, 10)
		s.addPlayer(1, 20, 10)
		s.addHive(80, 80)
		step(s, 1, Input{Mask: FireR})
		step(s, 6, Input{})
		if hit := s.PlayerEntity(1) < 0; hit != ff {
			t.Fatalf("friendlyFire=%v: player 2 hit=%v", ff, hit)
		}
		if s.Players[0].Score != 0 {
			t.Fatal("player kill scored")
		}
	}
}

func TestShotSnipeOnGhostLettersSplitsOrExplodes(t *testing.T) {
	splits, explodes := 0, 0
	for seed := uint32(0); seed < 400; seed++ {
		s := arena("Z1", 1) // smallSnipes on, explosion mask 0x0F (1 in 16 explode)
		s.Rng = NewRand(seed)
		s.addPlayer(0, 10, 10)
		s.addHive(80, 80)
		s.addSnipe(20, 10, DirE)
		step(s, 1, Input{Mask: FireR})
		step(s, 6, Input{})
		if s.Score != 1 {
			t.Fatalf("seed %d: score %d", seed, s.Score)
		}
		switch count(s, KindGhost) {
		case 1:
			splits++
		case 0:
			explodes++
		}
		if int(s.SnipesAlive) != count(s, KindSnipe)+count(s, KindGhost) {
			t.Fatalf("seed %d: snipe count drift", seed)
		}
	}
	if explodes == 0 || splits < 300 {
		t.Fatalf("splits %d explodes %d", splits, explodes)
	}
	s := arena("A1", 1) // no small snipes on A
	s.addPlayer(0, 10, 10)
	s.addHive(80, 80)
	s.addSnipe(20, 10, DirE)
	step(s, 1, Input{Mask: FireR})
	step(s, 6, Input{})
	if count(s, KindGhost) != 0 || s.SnipesAlive != 0 {
		t.Fatal("ghost on letter A")
	}
}

func TestHivesSpawnUpToMaxSnipes(t *testing.T) {
	cfg, _ := NewConfig("A1", 1, true)
	s := NewGame(cfg, 4)
	for i := range s.Ents {
		if s.Ents[i].Kind == KindPlayer {
			s.remove(i) // nobody to shoot or be shot
		}
	}
	s.Players[0].Respawn = 0
	s.Players[0].Lives = 99
	peak := int32(0)
	for i := 0; i < 20000; i++ {
		s.Step([MaxPlayers]Input{})
		peak = max(peak, s.SnipesAlive)
		if s.SnipesAlive > int32(cfg.MaxSnipes) {
			t.Fatalf("tick %d: %d snipes > max %d", s.Tick, s.SnipesAlive, cfg.MaxSnipes)
		}
	}
	if peak != int32(cfg.MaxSnipes) {
		t.Fatalf("peak %d snipes, want max %d", peak, cfg.MaxSnipes)
	}
}

func TestInvariantsHoldInRandomGames(t *testing.T) {
	for _, skill := range []string{"A1", "D3", "M5", "W8", "Z9"} {
		for seed := uint32(0); seed < 20; seed++ {
			cfg, _ := NewConfig(skill, 4, true)
			prevLives := [MaxPlayers]int32{}
			first := true
			randomGame(cfg, seed, 2000, func(s *State) {
				if int(s.HivesAlive) != count(s, KindHive) || int(s.SnipesAlive) != count(s, KindSnipe)+count(s, KindGhost) {
					t.Fatalf("%s seed %d tick %d: counters %d/%d disagree with entities", skill, seed, s.Tick, s.HivesAlive, s.SnipesAlive)
				}
				if s.SnipesAlive > int32(cfg.MaxSnipes) {
					t.Fatalf("%s seed %d: %d snipes over max", skill, seed, s.SnipesAlive)
				}
				for p := range s.Players {
					if !first && s.Players[p].Lives > prevLives[p] {
						t.Fatalf("%s seed %d: lives went up", skill, seed)
					}
					prevLives[p] = s.Players[p].Lives
				}
				first = false
				occ := append([]int32(nil), s.occ...)
				s.rebuildOcc()
				for i := range occ {
					if occ[i] != s.occ[i] {
						t.Fatalf("%s seed %d tick %d: occupancy index stale", skill, seed, s.Tick)
					}
				}
				for _, e := range s.Ents {
					w, h := e.Kind.Size()
					for dy := int32(0); dy < h; dy++ {
						for dx := int32(0); dx < w; dx++ {
							if s.Wall(e.X+dx, e.Y+dy) {
								t.Fatalf("%s seed %d: %v inside a wall", skill, seed, e)
							}
						}
					}
				}
			})
		}
	}
}
