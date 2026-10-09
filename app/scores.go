package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ScoreFileName is the high-score file kept next to the executable.
const ScoreFileName = "nlsnipes-highscores.json"

// Scores is the best score per skill code.
type Scores map[string]int32

// DefaultScorePath is next to the executable, or the working directory if
// that cannot be found.
func DefaultScorePath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), ScoreFileName)
	}
	return ScoreFileName
}

// LoadScores reads the file; a missing or broken file is an empty table.
func LoadScores(path string) Scores {
	s := Scores{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Save writes the table; errors are returned but never fatal to the game.
func (s Scores) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
