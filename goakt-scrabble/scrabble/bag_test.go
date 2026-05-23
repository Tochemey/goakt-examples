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

func newTestBag(t *testing.T) *Bag {
	t.Helper()
	return NewBag(English(), rand.New(rand.NewPCG(1, 2)))
}

func TestBagStartsFull(t *testing.T) {
	bag := newTestBag(t)

	if bag.Remaining() != 100 {
		t.Fatalf("expected 100 tiles, got %d", bag.Remaining())
	}
}

func TestBagDrawShrinks(t *testing.T) {
	bag := newTestBag(t)

	tiles := bag.Draw(7)

	if len(tiles) != 7 {
		t.Errorf("expected to draw 7, got %d", len(tiles))
	}

	if bag.Remaining() != 93 {
		t.Errorf("expected 93 remaining, got %d", bag.Remaining())
	}
}

func TestBagDrawClampedToRemaining(t *testing.T) {
	bag := newTestBag(t)
	_ = bag.Draw(100)

	tiles := bag.Draw(7)

	if len(tiles) != 0 {
		t.Errorf("expected empty bag to draw 0, got %d", len(tiles))
	}
}

func TestBagReturnRestoresUnassignedBlank(t *testing.T) {
	bag := newTestBag(t)

	placed := Tile{Letter: 5, Blank: true}
	bag.Return([]Tile{placed})

	if bag.Remaining() != 101 {
		t.Fatalf("expected 101 tiles after return, got %d", bag.Remaining())
	}

	for i := 0; i < bag.Remaining(); i++ {
		tiles := bag.Draw(1)
		if tiles[0].Blank && tiles[0].Letter != 0 {
			t.Fatalf("returned blank kept its chosen letter: %+v", tiles[0])
		}
	}
}

func TestBagDeterministicWithSeed(t *testing.T) {
	a := NewBag(English(), rand.New(rand.NewPCG(42, 99)))
	b := NewBag(English(), rand.New(rand.NewPCG(42, 99)))

	for range 100 {
		ta := a.Draw(1)[0]
		tb := b.Draw(1)[0]
		if ta != tb {
			t.Fatalf("two seeded bags differ at this draw: %+v vs %+v", ta, tb)
		}
	}
}
