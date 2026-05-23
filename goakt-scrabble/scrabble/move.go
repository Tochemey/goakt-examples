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
	"errors"
	"fmt"
	"slices"
)

// BingoBonus is awarded when a move places all 7 rack tiles.
const BingoBonus = 50

// Direction is the axis of a word on the board.
type Direction uint8

const (
	Horizontal Direction = iota
	Vertical
)

// Validation errors. Compare with errors.Is.
var (
	ErrEmptyMove                = errors.New("scrabble: move has no placements")
	ErrTooManyPlacements        = errors.New("scrabble: move places more than 7 tiles")
	ErrOutOfBounds              = errors.New("scrabble: placement is off the board")
	ErrDuplicatePlacement       = errors.New("scrabble: two placements on the same square")
	ErrSquareOccupied           = errors.New("scrabble: square is already filled")
	ErrUnassignedBlank          = errors.New("scrabble: a blank placement has no chosen letter")
	ErrNotOnLine                = errors.New("scrabble: placements are not on a single row or column")
	ErrNotContiguous            = errors.New("scrabble: placements leave a gap with no existing tile")
	ErrFirstMoveMustCoverCenter = errors.New("scrabble: first move must cover the center square")
	ErrMustTouchExisting        = errors.New("scrabble: move must touch an existing tile")
	ErrNoWordFormed             = errors.New("scrabble: move forms no word")
)

// InvalidWordError names a single dictionary-rejected word formed by a move.
type InvalidWordError struct {
	Word string
}

func (e *InvalidWordError) Error() string {
	return fmt.Sprintf("scrabble: word %q not in dictionary", e.Word)
}

// Placement is a single tile placed on the board by a move.
type Placement struct {
	Row  int
	Col  int
	Tile Tile
}

// Move is the set of tiles a player places on a single turn.
type Move struct {
	Placements []Placement
}

// FormedWord describes one word resulting from a move.
type FormedWord struct {
	Word      string
	Score     int
	StartRow  int
	StartCol  int
	Direction Direction
}

// MoveResult is what Validate returns on success.
type MoveResult struct {
	Words []FormedWord
	Score int
	Bingo bool
}

// Validate checks the move against the board and dictionary and computes
// the score on success. The board is not mutated; the caller calls Apply
// to commit.
func (m Move) Validate(board *Board, dict Dictionary, lang *Language) (*MoveResult, error) {
	if err := m.checkPlacements(board); err != nil {
		return nil, err
	}

	dir, err := m.lineDirection()
	if err != nil {
		return nil, err
	}

	placements := sortPlacements(m.Placements, dir)

	if err := checkConnectivity(board, placements, dir); err != nil {
		return nil, err
	}

	tmp := board.Clone()

	for _, p := range placements {
		_ = tmp.Place(p.Row, p.Col, p.Tile)
	}

	newSquares := make(map[[2]int]bool, len(placements))

	for _, p := range placements {
		newSquares[[2]int{p.Row, p.Col}] = true
	}

	words := collectWords(tmp, placements, dir)

	if len(words) == 0 {
		return nil, ErrNoWordFormed
	}

	if board.IsEmptyBoard() {
		if !coversCenter(placements) {
			return nil, ErrFirstMoveMustCoverCenter
		}
	} else if !extendsExisting(words, newSquares) {
		return nil, ErrMustTouchExisting
	}

	result := &MoveResult{Bingo: len(placements) == RackSize}

	for _, formedWord := range words {
		ids := wordLetterIDs(formedWord)
		if !dict.Contains(ids) {
			return nil, &InvalidWordError{Word: wordString(formedWord, lang)}
		}
		formed := FormedWord{
			Word:      wordString(formedWord, lang),
			Score:     scoreWord(formedWord, newSquares, lang),
			StartRow:  formedWord[0].row,
			StartCol:  formedWord[0].col,
			Direction: formedWord.direction(),
		}
		result.Words = append(result.Words, formed)
		result.Score += formed.Score
	}

	if result.Bingo {
		result.Score += BingoBonus
	}

	return result, nil
}

// Apply writes the move's placements onto the board. Call only after
// Validate has succeeded.
func (m Move) Apply(board *Board) error {
	for _, p := range m.Placements {
		if err := board.Place(p.Row, p.Col, p.Tile); err != nil {
			return err
		}
	}

	return nil
}

// wordTile is one tile in a word as collected during validation.
type wordTile struct {
	row, col int
	tile     Tile
}

// word is a contiguous run of tiles forming one word; tiles are in
// reading order (left-to-right for Horizontal, top-to-bottom for Vertical).
type word []wordTile

func (w word) direction() Direction {
	if len(w) < 2 {
		return Horizontal
	}
	if w[0].row == w[1].row {
		return Horizontal
	}

	return Vertical
}

func (m Move) checkPlacements(board *Board) error {
	if len(m.Placements) == 0 {
		return ErrEmptyMove
	}
	if len(m.Placements) > RackSize {
		return ErrTooManyPlacements
	}

	seen := make(map[[2]int]bool, len(m.Placements))

	for _, p := range m.Placements {
		if !InBounds(p.Row, p.Col) {
			return ErrOutOfBounds
		}
		if p.Tile.IsUnassignedBlank() {
			return ErrUnassignedBlank
		}
		key := [2]int{p.Row, p.Col}
		if seen[key] {
			return ErrDuplicatePlacement
		}
		if board.At(p.Row, p.Col).Filled {
			return ErrSquareOccupied
		}
		seen[key] = true
	}

	return nil
}

func (m Move) lineDirection() (Direction, error) {
	if len(m.Placements) == 1 {
		return Horizontal, nil
	}

	row0 := m.Placements[0].Row
	col0 := m.Placements[0].Col

	sameRow := true
	sameCol := true

	for _, p := range m.Placements[1:] {
		if p.Row != row0 {
			sameRow = false
		}
		if p.Col != col0 {
			sameCol = false
		}
	}

	switch {
	case sameRow:
		return Horizontal, nil
	case sameCol:
		return Vertical, nil
	}

	return 0, ErrNotOnLine
}

func sortPlacements(placements []Placement, dir Direction) []Placement {
	out := make([]Placement, len(placements))
	copy(out, placements)

	slices.SortFunc(out, func(a, b Placement) int {
		if dir == Horizontal {
			if a.Col != b.Col {
				return a.Col - b.Col
			}
			return a.Row - b.Row
		}
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})

	return out
}

func checkConnectivity(board *Board, placements []Placement, dir Direction) error {
	if len(placements) <= 1 {
		return nil
	}

	first := placements[0]
	last := placements[len(placements)-1]

	if dir == Horizontal {
		for col := first.Col + 1; col < last.Col; col++ {
			row := first.Row
			if !board.At(row, col).Filled && !hasPlacement(placements, row, col) {
				return ErrNotContiguous
			}
		}
		return nil
	}

	for row := first.Row + 1; row < last.Row; row++ {
		col := first.Col
		if !board.At(row, col).Filled && !hasPlacement(placements, row, col) {
			return ErrNotContiguous
		}
	}

	return nil
}

func hasPlacement(placements []Placement, row, col int) bool {
	for _, p := range placements {
		if p.Row == row && p.Col == col {
			return true
		}
	}

	return false
}

func coversCenter(placements []Placement) bool {
	for _, p := range placements {
		if p.Row == CenterRow && p.Col == CenterCol {
			return true
		}
	}

	return false
}

// collectWords returns the main word (along dir) plus every length-2+
// cross-word formed perpendicular to dir at a placement square.
func collectWords(tmp *Board, placements []Placement, dir Direction) []word {
	var words []word

	main := extractWord(tmp, placements[0].Row, placements[0].Col, dir)

	if len(main) >= 2 {
		words = append(words, main)
	}

	for _, p := range placements {
		cross := extractWord(tmp, p.Row, p.Col, perpendicular(dir))
		if len(cross) >= 2 {
			words = append(words, cross)
		}
	}

	return words
}

func extractWord(tmp *Board, row, col int, dir Direction) word {
	dRow, dCol := step(dir)

	startRow, startCol := row, col

	for {
		prevRow := startRow - dRow
		prevCol := startCol - dCol

		if !InBounds(prevRow, prevCol) || !tmp.At(prevRow, prevCol).Filled {
			break
		}

		startRow, startCol = prevRow, prevCol
	}

	var out word

	curRow, curCol := startRow, startCol

	for InBounds(curRow, curCol) && tmp.At(curRow, curCol).Filled {
		out = append(out, wordTile{row: curRow, col: curCol, tile: tmp.At(curRow, curCol).Tile})
		curRow += dRow
		curCol += dCol
	}

	return out
}

func perpendicular(dir Direction) Direction {
	if dir == Horizontal {
		return Vertical
	}

	return Horizontal
}

func extendsExisting(words []word, newSquares map[[2]int]bool) bool {
	for _, formed := range words {
		for _, tile := range formed {
			if !newSquares[[2]int{tile.row, tile.col}] {
				return true
			}
		}
	}

	return false
}

func wordLetterIDs(w word) []LetterID {
	out := make([]LetterID, len(w))

	for i, tile := range w {
		out[i] = tile.tile.Letter
	}

	return out
}

func wordString(w word, lang *Language) string {
	runes := make([]rune, len(w))

	for i, tile := range w {
		runes[i] = lang.Rune(tile.tile.Letter)
	}

	return string(runes)
}

func scoreWord(w word, newSquares map[[2]int]bool, lang *Language) int {
	letterTotal := 0
	wordMultiplier := 1

	for _, tile := range w {
		base := tile.tile.Score(lang)
		key := [2]int{tile.row, tile.col}

		if newSquares[key] {
			switch standardPremium(tile.row, tile.col) {
			case PremiumDoubleLetter:
				base *= 2
			case PremiumTripleLetter:
				base *= 3
			case PremiumDoubleWord:
				wordMultiplier *= 2
			case PremiumTripleWord:
				wordMultiplier *= 3
			}
		}

		letterTotal += base
	}

	return letterTotal * wordMultiplier
}

