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

func TestRackRefillFromBag(t *testing.T) {
	bag := NewBag(English(), rand.New(rand.NewPCG(1, 1)))
	rack := NewRack()

	rack.Refill(bag)

	if rack.Size() != RackSize {
		t.Fatalf("expected rack size %d, got %d", RackSize, rack.Size())
	}

	if bag.Remaining() != 100-RackSize {
		t.Fatalf("expected bag %d, got %d", 100-RackSize, bag.Remaining())
	}
}

func TestRackRemoveMatchesLetter(t *testing.T) {
	rack := &Rack{tiles: []Tile{{Letter: 7}, {Letter: 14}, {Letter: 17}}}

	taken, err := rack.Remove([]Tile{{Letter: 14}})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	if len(taken) != 1 || taken[0].Letter != 14 {
		t.Errorf("unexpected taken: %+v", taken)
	}

	if rack.Size() != 2 {
		t.Errorf("expected size 2 after remove, got %d", rack.Size())
	}
}

func TestRackRemoveBlankFromBlankRequest(t *testing.T) {
	rack := &Rack{tiles: []Tile{{Letter: 0}, BlankTile, {Letter: 4}}}

	taken, err := rack.Remove([]Tile{{Letter: 7, Blank: true}})
	if err != nil {
		t.Fatalf("remove blank: %v", err)
	}

	if !taken[0].IsUnassignedBlank() {
		t.Errorf("expected to take unassigned blank, got %+v", taken[0])
	}

	if rack.Size() != 2 {
		t.Errorf("expected size 2 after remove, got %d", rack.Size())
	}
}

func TestRackRemoveMissingLetterErrors(t *testing.T) {
	rack := &Rack{tiles: []Tile{{Letter: 0}, {Letter: 1}}}

	if _, err := rack.Remove([]Tile{{Letter: 25}}); err == nil {
		t.Fatal("expected error removing absent letter")
	}

	if rack.Size() != 2 {
		t.Errorf("rack should be unchanged on error; got size %d", rack.Size())
	}
}

func TestRackExchangeRequires7InBag(t *testing.T) {
	lang := English()
	bag := NewBag(lang, rand.New(rand.NewPCG(1, 1)))
	rack := NewRack()
	rack.Refill(bag)

	_ = bag.Draw(bag.Remaining() - 6)

	if err := rack.Exchange([]int{0, 1}, bag); err == nil {
		t.Fatal("expected error when bag has fewer than 7 tiles")
	}
}

func TestRackExchangePreservesSizes(t *testing.T) {
	lang := English()
	bag := NewBag(lang, rand.New(rand.NewPCG(7, 7)))
	rack := NewRack()
	rack.Refill(bag)

	bagBefore := bag.Remaining()

	if err := rack.Exchange([]int{0, 2, 5}, bag); err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if rack.Size() != RackSize {
		t.Errorf("expected rack size %d after exchange, got %d", RackSize, rack.Size())
	}

	if bag.Remaining() != bagBefore {
		t.Errorf("bag size should be unchanged across exchange, got %d (was %d)", bag.Remaining(), bagBefore)
	}
}

func TestRackExchangeRejectsDuplicateIndex(t *testing.T) {
	lang := English()
	bag := NewBag(lang, rand.New(rand.NewPCG(7, 7)))
	rack := NewRack()
	rack.Refill(bag)

	if err := rack.Exchange([]int{0, 0}, bag); err == nil {
		t.Fatal("expected error for duplicate exchange index")
	}
}
