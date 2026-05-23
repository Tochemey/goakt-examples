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
	"math/rand/v2"
	"testing"
)

func rackFromWord(t *testing.T, lang *Language, word string) *Rack {
	t.Helper()

	ids, err := lang.NormalizeWord(word)
	if err != nil {
		t.Fatalf("normalize %q: %v", word, err)
	}

	r := NewRack()

	for _, id := range ids {
		r.tiles = append(r.tiles, Tile{Letter: id})
	}

	return r
}

func TestIsGameOverNormalState(t *testing.T) {
	bag := NewBag(English(), rand.New(rand.NewPCG(1, 1)))
	rack := NewRack()
	rack.Refill(bag)

	if IsGameOver(bag, []*Rack{rack}, 0) {
		t.Error("game should not be over with full bag and rack")
	}
}

func TestIsGameOverBagEmptyAndPlayerOut(t *testing.T) {
	bag := NewBag(English(), rand.New(rand.NewPCG(1, 1)))
	_ = bag.Draw(bag.Remaining())

	empty := NewRack()
	other, _ := NewBag(English(), rand.New(rand.NewPCG(2, 2))), NewRack()
	_ = other

	full := NewRack()
	full.tiles = []Tile{{Letter: 0}, {Letter: 1}}

	if !IsGameOver(bag, []*Rack{empty, full}, 0) {
		t.Error("game should be over when bag empty and a rack empty")
	}
}

func TestIsGameOverSixScorelessTurns(t *testing.T) {
	bag := NewBag(English(), rand.New(rand.NewPCG(1, 1)))
	rack := NewRack()
	rack.Refill(bag)

	if !IsGameOver(bag, []*Rack{rack}, ScorelessTurnsToEnd) {
		t.Error("game should be over after six scoreless turns")
	}
}

func TestFinalScoresRackPenaltyOnly(t *testing.T) {
	lang := English()

	rackA := rackFromWord(t, lang, "BC")
	rackB := rackFromWord(t, lang, "DE")

	got := FinalScores([]int{50, 30}, []*Rack{rackA, rackB}, lang)

	want := []int{50 - (3 + 3), 30 - (2 + 1)}

	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("final scores: got %v want %v", got, want)
	}
}

func TestFinalScoresOutBonus(t *testing.T) {
	lang := English()

	rackOut := NewRack()
	rackOther := rackFromWord(t, lang, "BC")

	got := FinalScores([]int{50, 30}, []*Rack{rackOut, rackOther}, lang)

	want := []int{50 + (3 + 3), 30 - (3 + 3)}

	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("final scores with out-bonus: got %v want %v", got, want)
	}
}
