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
	"testing"
)

func placeWord(t *testing.T, board *Board, lang *Language, word string, row, col int, dir Direction) {
	t.Helper()

	ids, err := lang.NormalizeWord(word)
	if err != nil {
		t.Fatalf("normalize %q: %v", word, err)
	}

	for i, id := range ids {
		r, c := stepFromBase(row, col, i, dir)
		if err := board.Place(r, c, Tile{Letter: id}); err != nil {
			t.Fatalf("place %q at (%d,%d): %v", word, r, c, err)
		}
	}
}

func placementsFor(t *testing.T, lang *Language, word string, row, col int, dir Direction) []Placement {
	t.Helper()

	ids, err := lang.NormalizeWord(word)
	if err != nil {
		t.Fatalf("normalize %q: %v", word, err)
	}

	out := make([]Placement, len(ids))

	for i, id := range ids {
		r, c := stepFromBase(row, col, i, dir)
		out[i] = Placement{Row: r, Col: c, Tile: Tile{Letter: id}}
	}

	return out
}

func stepFromBase(row, col, i int, dir Direction) (int, int) {
	if dir == Horizontal {
		return row, col + i
	}

	return row + i, col
}

func TestFirstMoveHORSEAtCenter(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	move := Move{Placements: placementsFor(t, lang, "HORSE", 7, 7, Horizontal)}

	result, err := move.Validate(board, dawg, lang)
	if err != nil {
		t.Fatalf("expected valid move, got %v", err)
	}

	if result.Score != 18 {
		t.Errorf("HORSE score: got %d want 18", result.Score)
	}

	if result.Bingo {
		t.Error("5-tile move should not be a bingo")
	}

	if len(result.Words) != 1 || result.Words[0].Word != "HORSE" {
		t.Errorf("expected one word HORSE, got %+v", result.Words)
	}
}

func TestFirstMoveMustCoverCenter(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	move := Move{Placements: placementsFor(t, lang, "HORSE", 0, 0, Horizontal)}

	_, err := move.Validate(board, dawg, lang)
	if !errors.Is(err, ErrFirstMoveMustCoverCenter) {
		t.Errorf("expected ErrFirstMoveMustCoverCenter, got %v", err)
	}
}

func TestMoveMustFormDictWord(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	move := Move{Placements: placementsFor(t, lang, "ZQXJN", 7, 7, Horizontal)}

	_, err := move.Validate(board, dawg, lang)

	var inv *InvalidWordError
	if !errors.As(err, &inv) {
		t.Errorf("expected InvalidWordError, got %v", err)
	}
}

func TestSecondMoveMustTouchExisting(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	placeWord(t, board, lang, "HORSE", 7, 7, Horizontal)

	move := Move{Placements: placementsFor(t, lang, "BIT", 0, 0, Horizontal)}

	_, err := move.Validate(board, dawg, lang)
	if !errors.Is(err, ErrMustTouchExisting) {
		t.Errorf("expected ErrMustTouchExisting, got %v", err)
	}
}

func TestSingleTileCrossWord(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	placeWord(t, board, lang, "HORSE", 7, 7, Horizontal)

	eID, _ := lang.ID('E')
	move := Move{Placements: []Placement{{Row: 8, Col: 7, Tile: Tile{Letter: eID}}}}

	result, err := move.Validate(board, dawg, lang)
	if err != nil {
		t.Fatalf("expected valid HE cross-word, got %v", err)
	}

	if result.Score != 5 {
		t.Errorf("HE score: got %d want 5", result.Score)
	}

	if len(result.Words) != 1 || result.Words[0].Word != "HE" {
		t.Errorf("expected one cross-word HE, got %+v", result.Words)
	}
}

func TestExtendingExistingWord(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	placeWord(t, board, lang, "HORSE", 7, 7, Horizontal)

	sID, _ := lang.ID('S')
	move := Move{Placements: []Placement{{Row: 7, Col: 12, Tile: Tile{Letter: sID}}}}

	result, err := move.Validate(board, dawg, lang)
	if err != nil {
		t.Fatalf("expected valid HORSES extension, got %v", err)
	}

	if result.Score != 9 {
		t.Errorf("HORSES score: got %d want 9", result.Score)
	}

	if result.Words[0].Word != "HORSES" {
		t.Errorf("expected word HORSES, got %q", result.Words[0].Word)
	}
}

func TestGapInPlacementsRejected(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	hID, _ := lang.ID('H')
	eID, _ := lang.ID('E')

	move := Move{Placements: []Placement{
		{Row: 7, Col: 7, Tile: Tile{Letter: hID}},
		{Row: 7, Col: 9, Tile: Tile{Letter: eID}},
	}}

	_, err := move.Validate(board, dawg, lang)
	if !errors.Is(err, ErrNotContiguous) {
		t.Errorf("expected ErrNotContiguous, got %v", err)
	}
}

func TestPlacementsNotOnLineRejected(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	hID, _ := lang.ID('H')
	eID, _ := lang.ID('E')

	move := Move{Placements: []Placement{
		{Row: 7, Col: 7, Tile: Tile{Letter: hID}},
		{Row: 8, Col: 8, Tile: Tile{Letter: eID}},
	}}

	_, err := move.Validate(board, dawg, lang)
	if !errors.Is(err, ErrNotOnLine) {
		t.Errorf("expected ErrNotOnLine, got %v", err)
	}
}

func TestBingoBonus(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	move := Move{Placements: placementsFor(t, lang, "HOSIERY", 7, 1, Horizontal)}

	result, err := move.Validate(board, dawg, lang)
	if err != nil {
		t.Fatalf("expected valid HOSIERY bingo, got %v", err)
	}

	if !result.Bingo {
		t.Error("7-tile move should be a bingo")
	}

	if result.Score != 78 {
		t.Errorf("HOSIERY bingo score: got %d want 78", result.Score)
	}
}

func TestBlankTileScoresZero(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	hID, _ := lang.ID('H')
	oID, _ := lang.ID('O')
	rID, _ := lang.ID('R')
	sID, _ := lang.ID('S')
	eID, _ := lang.ID('E')

	move := Move{Placements: []Placement{
		{Row: 7, Col: 7, Tile: Tile{Letter: hID, Blank: true}},
		{Row: 7, Col: 8, Tile: Tile{Letter: oID}},
		{Row: 7, Col: 9, Tile: Tile{Letter: rID}},
		{Row: 7, Col: 10, Tile: Tile{Letter: sID}},
		{Row: 7, Col: 11, Tile: Tile{Letter: eID}},
	}}

	result, err := move.Validate(board, dawg, lang)
	if err != nil {
		t.Fatalf("expected valid blank-H HORSE, got %v", err)
	}

	if result.Score != 10 {
		t.Errorf("blank-H HORSE score: got %d want 10", result.Score)
	}
}

func TestUnassignedBlankRejected(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	move := Move{Placements: []Placement{
		{Row: 7, Col: 7, Tile: BlankTile},
	}}

	_, err := move.Validate(board, dawg, lang)
	if !errors.Is(err, ErrUnassignedBlank) {
		t.Errorf("expected ErrUnassignedBlank, got %v", err)
	}
}

func TestEmptyMoveRejected(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()

	_, err := Move{}.Validate(board, dawg, lang)
	if !errors.Is(err, ErrEmptyMove) {
		t.Errorf("expected ErrEmptyMove, got %v", err)
	}
}

func TestPlacementOnFilledSquareRejected(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	placeWord(t, board, lang, "HORSE", 7, 7, Horizontal)

	hID, _ := lang.ID('H')
	move := Move{Placements: []Placement{{Row: 7, Col: 7, Tile: Tile{Letter: hID}}}}

	_, err := move.Validate(board, dawg, lang)
	if !errors.Is(err, ErrSquareOccupied) {
		t.Errorf("expected ErrSquareOccupied, got %v", err)
	}
}
