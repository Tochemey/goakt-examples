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
	"testing"
)

func TestBotPicksBestFirstMove(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	rack := rackFromWord(t, lang, "HORSE")

	best := BestMove(board, rack, dawg, lang)
	if best == nil {
		t.Fatal("expected a best move on empty board with HORSE rack")
	}

	if best.Result.Score != 24 {
		t.Errorf("best first-move score: got %d want 24", best.Result.Score)
	}

	if len(best.Move.Placements) != 5 {
		t.Errorf("expected 5 placements for HORSE, got %d", len(best.Move.Placements))
	}
}

func TestBotFindsExtension(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	placeWord(t, board, lang, "HORSE", 7, 7, Horizontal)

	rack := rackFromWord(t, lang, "S")

	best := BestMove(board, rack, dawg, lang)
	if best == nil {
		t.Fatal("expected a best move extending HORSE → HORSES")
	}

	if len(best.Move.Placements) != 1 {
		t.Fatalf("expected 1 placement for HORSES extension, got %d", len(best.Move.Placements))
	}

	p := best.Move.Placements[0]
	if p.Row != 7 || p.Col != 12 {
		t.Errorf("expected placement at (7,12), got (%d,%d)", p.Row, p.Col)
	}

	if best.Result.Score != 9 {
		t.Errorf("HORSES extension score: got %d want 9", best.Result.Score)
	}
}

func TestBotEmptyRack(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	rack := NewRack()

	if best := BestMove(board, rack, dawg, lang); best != nil {
		t.Errorf("expected nil best with empty rack, got %+v", best)
	}
}

func TestBotUsesBlankToFormWord(t *testing.T) {
	dawg, lang := newTestDAWG(t)
	board := NewBoard()
	placeWord(t, board, lang, "HORSE", 7, 7, Horizontal)

	rack := NewRack()
	rack.tiles = []Tile{BlankTile}

	best := BestMove(board, rack, dawg, lang)
	if best == nil {
		t.Fatal("expected blank to form a 1-letter extension or cross-word")
	}

	if !best.Move.Placements[0].Tile.Blank {
		t.Errorf("expected blank tile in placement, got %+v", best.Move.Placements[0].Tile)
	}
}
