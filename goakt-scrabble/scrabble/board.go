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

import "fmt"

const (
	BoardSize = 15
	CenterRow = 7
	CenterCol = 7
)

// Premium classifies a square's bonus. The bonus only applies to a tile
// placed on the square this turn — once a tile is on the square, it is spent.
type Premium uint8

const (
	PremiumNone Premium = iota
	PremiumDoubleLetter
	PremiumTripleLetter
	PremiumDoubleWord
	PremiumTripleWord
)

// Square is one cell of the board. Premium is fixed at construction; Tile
// is the zero value while Filled is false.
type Square struct {
	Premium Premium
	Tile    Tile
	Filled  bool
}

// Board is a 15x15 array of Squares (row-major). The premium-square layout
// is the canonical Hasbro Scrabble layout, identical across EN/FR/ES.
type Board struct {
	squares [BoardSize][BoardSize]Square
	tiles   int
}

// NewBoard returns an empty board with premium squares laid out.
func NewBoard() *Board {
	b := &Board{}

	for row := range BoardSize {
		for col := range BoardSize {
			b.squares[row][col].Premium = standardPremium(row, col)
		}
	}

	return b
}

// At returns the square at (row, col).
func (b *Board) At(row, col int) *Square {
	return &b.squares[row][col]
}

// IsEmptyBoard reports whether the board has no tiles at all.
func (b *Board) IsEmptyBoard() bool {
	return b.tiles == 0
}

// Place puts a tile on an empty square.
func (b *Board) Place(row, col int, t Tile) error {
	sq := &b.squares[row][col]

	if sq.Filled {
		return fmt.Errorf("scrabble: square (%d,%d) is already filled", row, col)
	}

	sq.Tile = t
	sq.Filled = true
	b.tiles++

	return nil
}

// Clone returns a deep copy of the board.
func (b *Board) Clone() *Board {
	clone := &Board{squares: b.squares, tiles: b.tiles}

	return clone
}

// InBounds reports whether (row, col) is a valid coordinate.
func InBounds(row, col int) bool {
	return row >= 0 && row < BoardSize && col >= 0 && col < BoardSize
}

// standardPremium returns the premium type at (row, col) using the D4
// symmetry of the standard Scrabble board: normalize (row, col) into the
// 0 <= r <= c <= 7 octant and enumerate only that octant.
func standardPremium(row, col int) Premium {
	r, c := row, col

	if r > 7 {
		r = 14 - r
	}

	if c > 7 {
		c = 14 - c
	}

	if r > c {
		r, c = c, r
	}

	switch [2]int{r, c} {
	case [2]int{0, 0}, [2]int{0, 7}:
		return PremiumTripleWord
	case [2]int{1, 1}, [2]int{2, 2}, [2]int{3, 3}, [2]int{4, 4}, [2]int{7, 7}:
		return PremiumDoubleWord
	case [2]int{1, 5}, [2]int{5, 5}:
		return PremiumTripleLetter
	case [2]int{0, 3}, [2]int{2, 6}, [2]int{3, 7}, [2]int{6, 6}:
		return PremiumDoubleLetter
	}

	return PremiumNone
}
