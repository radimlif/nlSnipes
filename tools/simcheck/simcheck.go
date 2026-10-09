// Package simcheck detects code copied from the reference port.
//
// Sources are reduced to a language-neutral token stream (identifiers
// lowercased, numbers normalised to decimal, strings collapsed, comments and
// semicolons dropped). Every window of K consecutive tokens in the reference
// is fingerprinted; any window of ours with the same fingerprint is a match.
// Windows with fewer than MinIdents identifiers are ignored, so numeric data
// tables (facts about the game, not expression) never count as copies.
package simcheck

import (
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Token is one lexical unit with the 1-based line it starts on.
type Token struct {
	Text  string
	Line  int
	Ident bool
}

// Options controls the comparison.
type Options struct {
	K         int // window length in tokens
	MinIdents int // windows with fewer identifiers are skipped
}

// DefaultOptions matches the design: no 40-token window may match.
var DefaultOptions = Options{K: 40, MinIdents: 8}

// Match is one run of consecutive matching windows.
type Match struct {
	File      string
	StartLine int
	EndLine   int
	RefFile   string
	RefLine   int
}

// Tokenize splits source text into normalised tokens.
func Tokenize(src string) []Token {
	var toks []Token
	line := 1
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v' || c == ';':
			i++
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				end = len(src) - i - 2
			}
			line += strings.Count(src[i:i+2+end], "\n")
			i += end + 4
		case c == '"' || c == '\'' || c == '`':
			start := line
			i++
			for i < len(src) && src[i] != c {
				if src[i] == '\\' && c != '`' {
					i++
				}
				if i < len(src) && src[i] == '\n' {
					line++
				}
				i++
			}
			i++
			toks = append(toks, Token{Text: "STR", Line: start})
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, Token{Text: strings.ToLower(src[i:j]), Line: line, Ident: true})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (isIdentPart(src[j]) || src[j] == '.') {
				j++
			}
			toks = append(toks, Token{Text: normNumber(src[i:j]), Line: line})
			i = j
		default:
			_, size := utf8.DecodeRuneInString(src[i:])
			toks = append(toks, Token{Text: src[i : i+size], Line: line})
			i += size
		}
	}
	return toks
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// normNumber renders integer literals in decimal so 0x7F and 127 compare equal.
func normNumber(s string) string {
	t := strings.TrimRight(strings.ToLower(s), "ul")
	if v, err := strconv.ParseInt(strings.ReplaceAll(t, "_", ""), 0, 64); err == nil {
		return strconv.FormatInt(v, 10)
	}
	return strings.ToLower(s)
}

func windowHash(toks []Token) uint64 {
	h := fnv.New64a()
	for _, t := range toks {
		h.Write([]byte(t.Text))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func countIdents(toks []Token) int {
	n := 0
	for _, t := range toks {
		if t.Ident {
			n++
		}
	}
	return n
}

type refLoc struct {
	file string
	line int
}

// Index holds the fingerprints of a reference tree.
type Index struct {
	opt  Options
	seen map[uint64]refLoc
}

// NewIndex creates an empty index.
func NewIndex(opt Options) *Index {
	return &Index{opt: opt, seen: map[uint64]refLoc{}}
}

// Len reports how many distinct fingerprints the index holds.
func (ix *Index) Len() int { return len(ix.seen) }

// Add fingerprints one reference file.
func (ix *Index) Add(name, src string) {
	toks := Tokenize(src)
	for i := 0; i+ix.opt.K <= len(toks); i++ {
		w := toks[i : i+ix.opt.K]
		if countIdents(w) < ix.opt.MinIdents {
			continue
		}
		h := windowHash(w)
		if _, ok := ix.seen[h]; !ok {
			ix.seen[h] = refLoc{name, w[0].Line}
		}
	}
}

// Check returns the runs of windows in src that also occur in the index.
func (ix *Index) Check(name, src string) []Match {
	toks := Tokenize(src)
	var out []Match
	var cur *Match
	last := -2
	for i := 0; i+ix.opt.K <= len(toks); i++ {
		w := toks[i : i+ix.opt.K]
		if countIdents(w) < ix.opt.MinIdents {
			continue
		}
		loc, ok := ix.seen[windowHash(w)]
		if !ok {
			continue
		}
		end := w[len(w)-1].Line
		if cur != nil && i == last+1 {
			cur.EndLine = end
		} else {
			out = append(out, Match{File: name, StartLine: w[0].Line, EndLine: end, RefFile: loc.file, RefLine: loc.line})
			cur = &out[len(out)-1]
		}
		last = i
	}
	return out
}

// IndexDir fingerprints every text file under dir, skipping .git.
func IndexDir(dir string, opt Options) (*Index, error) {
	ix := NewIndex(opt)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".git" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !utf8.Valid(b) || len(b) > 4<<20 {
			return nil
		}
		ix.Add(p, string(b))
		return nil
	})
	return ix, err
}

// CheckTree scans every .go file under root, skipping the reference tree,
// hidden directories, dist/ and testdata/.
func CheckTree(root, refDir string, ix *Index) ([]Match, error) {
	refAbs, _ := filepath.Abs(refDir)
	var out []Match
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			abs, _ := filepath.Abs(p)
			name := d.Name()
			if abs == refAbs || name == "testdata" || name == "dist" || (p != root && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".go" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, ix.Check(p, string(b))...)
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].StartLine < out[j].StartLine
	})
	return out, err
}
