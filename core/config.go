package core

import "fmt"

// MaxPlayers is the number of player slots in NLSNIPES Light.
const MaxPlayers = 4

// Skill tables from DESIGN.md §2, indexed by letter (A=0) or digit (1=0).
var (
	accuracyTable      = [26]uint8{2, 3, 4, 3, 4, 4, 3, 4, 3, 4, 4, 5, 3, 4, 3, 4, 3, 4, 3, 4, 4, 5, 4, 4, 5, 5}
	smallSnipesTable   = [26]uint8{0, 0, 0, 1, 1, 1, 0, 0, 1, 1, 1, 1, 0, 0, 1, 1, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1}
	bounceTable        = [26]uint8{0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	explosionMaskTable = [26]uint8{
		0x7F, 0x7F, 0x7F, 0x3F, 0x3F, 0x1F, 0x7F, 0x7F, 0x3F, 0x3F, 0x1F, 0x1F, 0x7F,
		0x7F, 0x3F, 0x3F, 0x7F, 0x7F, 0x3F, 0x3F, 0x1F, 0x1F, 0x3F, 0x1F, 0x1F, 0x0F,
	}
	maxSnipesTable = [9]uint16{10, 20, 30, 40, 60, 80, 100, 120, 150}
	hivesTable     = [9]uint8{3, 3, 4, 4, 5, 5, 6, 8, 10}
	livesTable     = [9]uint8{5, 5, 5, 5, 5, 4, 4, 3, 2}
)

// Config is everything a game's rules depend on. Letter, Digit, Players and
// FriendlyFire are the inputs; the rest is derived from the skill tables.
type Config struct {
	Letter       uint8 // 0..25 for A..Z
	Digit        uint8 // 1..9
	Players      uint8 // 1..MaxPlayers
	FriendlyFire bool

	Accuracy          uint8
	SmallSnipes       bool
	Bounce            bool
	ExplosionMask     uint8
	ElectricWalls     bool
	HivesResistSpears bool
	MaxSnipes         uint16
	Hives             uint8
	Lives             uint8
}

// NewConfig builds a config from a skill code such as "M5".
func NewConfig(skill string, players int, friendlyFire bool) (Config, error) {
	if len(skill) != 2 || skill[0] < 'A' || skill[0] > 'Z' || skill[1] < '1' || skill[1] > '9' {
		return Config{}, fmt.Errorf("skill code %q: want an uppercase letter and a digit 1-9, e.g. A1 or M5", skill)
	}
	if players < 1 || players > MaxPlayers {
		return Config{}, fmt.Errorf("players %d: want 1-%d", players, MaxPlayers)
	}
	return configFor(skill[0]-'A', skill[1]-'0', uint8(players), friendlyFire), nil
}

func configFor(letter, digit, players uint8, friendlyFire bool) Config {
	return Config{
		Letter:            letter,
		Digit:             digit,
		Players:           players,
		FriendlyFire:      friendlyFire,
		Accuracy:          accuracyTable[letter],
		SmallSnipes:       smallSnipesTable[letter] == 1,
		Bounce:            bounceTable[letter] == 1,
		ExplosionMask:     explosionMaskTable[letter],
		ElectricWalls:     letter >= 'M'-'A',
		HivesResistSpears: letter >= 'W'-'A',
		MaxSnipes:         maxSnipesTable[digit-1],
		Hives:             hivesTable[digit-1],
		Lives:             livesTable[digit-1],
	}
}

// Skill returns the skill code, e.g. "M5".
func (c Config) Skill() string { return string([]byte{'A' + c.Letter, '0' + c.Digit}) }
