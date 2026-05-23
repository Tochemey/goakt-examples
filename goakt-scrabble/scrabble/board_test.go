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

import "testing"

// standardBoardASCII is the canonical Scrabble premium-square layout.
// T = TripleWord, D = DoubleWord, t = TripleLetter, d = DoubleLetter, . = none.
const standardBoardASCII = "" +
	"T..d...T...d..T" +
	".D...t...t...D." +
	"..D...d.d...D.." +
	"d..D...d...D..d" +
	"....D.....D...." +
	".t...t...t...t." +
	"..d...d.d...d.." +
	"T..d...D...d..T" +
	"..d...d.d...d.." +
	".t...t...t...t." +
	"....D.....D...." +
	"d..D...d...D..d" +
	"..D...d.d...D.." +
	".D...t...t...D." +
	"T..d...T...d..T"

func TestStandardPremiumLayout(t *testing.T) {
	board := NewBoard()

	for row := range BoardSize {
		for col := range BoardSize {
			want := premiumFromChar(standardBoardASCII[row*BoardSize+col])
			got := board.At(row, col).Premium
			if got != want {
				t.Errorf("(%d,%d): got %v want %v", row, col, got, want)
			}
		}
	}
}

func TestPremiumCounts(t *testing.T) {
	board := NewBoard()

	counts := map[Premium]int{}

	for row := range BoardSize {
		for col := range BoardSize {
			counts[board.At(row, col).Premium]++
		}
	}

	want := map[Premium]int{
		PremiumTripleWord:   8,
		PremiumDoubleWord:   17,
		PremiumTripleLetter: 12,
		PremiumDoubleLetter: 24,
		PremiumNone:         BoardSize*BoardSize - 8 - 17 - 12 - 24,
	}

	for p, n := range want {
		if counts[p] != n {
			t.Errorf("premium %v: got %d want %d", p, counts[p], n)
		}
	}
}

func TestBoardEmptyAtStart(t *testing.T) {
	board := NewBoard()

	if !board.IsEmptyBoard() {
		t.Fatal("new board should report empty")
	}

	if err := board.Place(7, 7, Tile{Letter: 0}); err != nil {
		t.Fatalf("place failed: %v", err)
	}

	if board.IsEmptyBoard() {
		t.Fatal("board with one tile should not report empty")
	}
}

func TestBoardPlaceTwiceFails(t *testing.T) {
	board := NewBoard()

	if err := board.Place(0, 0, Tile{Letter: 0}); err != nil {
		t.Fatalf("first place failed: %v", err)
	}

	if err := board.Place(0, 0, Tile{Letter: 1}); err == nil {
		t.Fatal("expected error placing on filled square")
	}
}

func premiumFromChar(c byte) Premium {
	switch c {
	case 'T':
		return PremiumTripleWord
	case 'D':
		return PremiumDoubleWord
	case 't':
		return PremiumTripleLetter
	case 'd':
		return PremiumDoubleLetter
	}

	return PremiumNone
}
