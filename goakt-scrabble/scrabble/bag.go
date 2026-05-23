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

import "math/rand/v2"

// Bag is the shuffled tile pool for one game. Not safe for concurrent use.
type Bag struct {
	lang  *Language
	tiles []Tile
	rng   *rand.Rand
}

// NewBag builds a full starting bag for lang and shuffles it. Pass a
// deterministic rng for reproducible tests.
func NewBag(lang *Language, rng *rand.Rand) *Bag {
	tiles := make([]Tile, 0, lang.TotalTiles())

	for id, count := range lang.Distribution {
		for range count {
			tiles = append(tiles, Tile{Letter: LetterID(id)})
		}
	}

	for range lang.Blanks {
		tiles = append(tiles, BlankTile)
	}

	bag := &Bag{lang: lang, tiles: tiles, rng: rng}
	bag.shuffle()

	return bag
}

// Remaining returns the number of tiles still in the bag.
func (b *Bag) Remaining() int {
	return len(b.tiles)
}

// Draw pops up to count tiles off the top of the bag, returning fewer if
// the bag runs out.
func (b *Bag) Draw(count int) []Tile {
	if count > len(b.tiles) {
		count = len(b.tiles)
	}

	if count == 0 {
		return nil
	}

	start := len(b.tiles) - count
	out := make([]Tile, count)
	copy(out, b.tiles[start:])
	b.tiles = b.tiles[:start]

	return out
}

// Return puts tiles back into the bag and re-shuffles. Blanks are returned
// in their unassigned form.
func (b *Bag) Return(tiles []Tile) {
	for _, tile := range tiles {
		if tile.Blank {
			tile = BlankTile
		}
		b.tiles = append(b.tiles, tile)
	}

	b.shuffle()
}

func (b *Bag) shuffle() {
	b.rng.Shuffle(len(b.tiles), func(i, j int) {
		b.tiles[i], b.tiles[j] = b.tiles[j], b.tiles[i]
	})
}
