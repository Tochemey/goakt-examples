// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package scrabble

import (
	"fmt"
	"strings"
	"unicode"
)

// LetterID is the per-language index of a letter in [0, len(Language.Letters)).
type LetterID uint8

// Language describes one playable Scrabble language. Letters, PointValues and
// Distribution are parallel slices indexed by LetterID.
type Language struct {
	Name         string
	Code         string
	Letters      []rune
	PointValues  []int
	Distribution []int
	Blanks       int

	rune2id map[rune]LetterID
}

// English is the canonical 26-letter / 2-blank Scrabble distribution.
func English() *Language {
	letters := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")

	points := []int{
		1, 3, 3, 2, 1, 4, 2, 4, 1, 8, 5, 1, 3,
		1, 1, 3, 10, 1, 1, 1, 1, 4, 4, 8, 4, 10,
	}

	dist := []int{
		9, 2, 2, 4, 12, 2, 3, 2, 9, 1, 1, 4, 2,
		6, 8, 2, 1, 6, 4, 6, 4, 2, 2, 1, 2, 1,
	}

	return newLanguage("English", "en", letters, points, dist, 2)
}

// AlphabetSize returns the number of distinct (non-blank) letters.
func (l *Language) AlphabetSize() int {
	return len(l.Letters)
}

// TotalTiles returns the bag size at the start of a game.
func (l *Language) TotalTiles() int {
	total := l.Blanks

	for _, count := range l.Distribution {
		total += count
	}

	return total
}

// Rune returns the rune for a LetterID.
func (l *Language) Rune(id LetterID) rune {
	return l.Letters[id]
}

// ID returns the LetterID for a rune (case-insensitive).
func (l *Language) ID(r rune) (LetterID, bool) {
	id, ok := l.rune2id[unicode.ToUpper(r)]

	return id, ok
}

// PointValue returns the point value of a letter (ignores the blank rule).
func (l *Language) PointValue(id LetterID) int {
	return l.PointValues[id]
}

// NormalizeWord uppercases s and converts it into LetterIDs. Returns an
// error naming the first rune not in this language's alphabet.
func (l *Language) NormalizeWord(s string) ([]LetterID, error) {
	upper := strings.ToUpper(s)
	out := make([]LetterID, 0, len(upper))

	for _, letter := range upper {
		id, ok := l.rune2id[letter]
		if !ok {
			return nil, fmt.Errorf("scrabble: rune %q not in %q alphabet", letter, l.Code)
		}
		out = append(out, id)
	}

	return out, nil
}

func newLanguage(name, code string, letters []rune, points, dist []int, blanks int) *Language {
	if len(letters) != len(points) || len(letters) != len(dist) {
		panic(fmt.Sprintf("scrabble: language %q parallel-slice mismatch", code))
	}

	lookup := make(map[rune]LetterID, len(letters))

	for i, r := range letters {
		lookup[r] = LetterID(i)
	}

	return &Language{
		Name:         name,
		Code:         code,
		Letters:      letters,
		PointValues:  points,
		Distribution: dist,
		Blanks:       blanks,
		rune2id:      lookup,
	}
}
