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
	// RackSize is the maximum number of tiles a player holds.
	RackSize = 7

	// sentinelLetter is a LetterID that cannot occur on a real tile; used
	// to mark a slot consumed during in-place rack matching.
	sentinelLetter LetterID = 255
)

// Rack is a player's hand, ordered as the player sees it.
type Rack struct {
	tiles []Tile
}

// NewRack returns an empty rack.
func NewRack() *Rack {
	return &Rack{tiles: make([]Tile, 0, RackSize)}
}

// Tiles returns a copy of the rack's tiles, in order.
func (r *Rack) Tiles() []Tile {
	out := make([]Tile, len(r.tiles))
	copy(out, r.tiles)

	return out
}

// Size returns the current number of tiles in the rack.
func (r *Rack) Size() int {
	return len(r.tiles)
}

// Add appends tiles to the rack. Used to restore tiles consumed by a
// failed placement attempt.
func (r *Rack) Add(tiles []Tile) {
	r.tiles = append(r.tiles, tiles...)
}

// Refill draws from bag until the rack is full or the bag is empty.
func (r *Rack) Refill(bag *Bag) {
	need := RackSize - len(r.tiles)

	if need <= 0 {
		return
	}

	r.tiles = append(r.tiles, bag.Draw(need)...)
}

// Remove takes the listed tiles out of the rack. A non-blank request
// matches a same-letter non-blank tile; a blank request matches an
// unassigned blank. On any miss the rack is unchanged and an error returned.
func (r *Rack) Remove(needed []Tile) ([]Tile, error) {
	working := make([]Tile, len(r.tiles))
	copy(working, r.tiles)

	taken := make([]Tile, 0, len(needed))
	indices := make([]int, 0, len(needed))

	for _, want := range needed {
		idx := matchRack(working, want)
		if idx < 0 {
			return nil, fmt.Errorf("scrabble: rack does not contain %+v", want)
		}
		taken = append(taken, working[idx])
		indices = append(indices, idx)
		working[idx] = Tile{Letter: sentinelLetter}
	}

	r.tiles = compactExcluding(r.tiles, indices)

	return taken, nil
}

// Exchange swaps the tiles at the given rack indices with fresh tiles from
// the bag. The bag must hold at least 7 tiles (official Scrabble rule);
// the swapped-out tiles go back into the bag.
func (r *Rack) Exchange(indices []int, bag *Bag) error {
	if bag.Remaining() < RackSize {
		return fmt.Errorf("scrabble: bag has only %d tiles, exchange requires at least %d", bag.Remaining(), RackSize)
	}

	seen := make(map[int]bool, len(indices))

	for _, idx := range indices {
		if idx < 0 || idx >= len(r.tiles) {
			return fmt.Errorf("scrabble: exchange index %d out of range", idx)
		}
		if seen[idx] {
			return fmt.Errorf("scrabble: exchange index %d duplicated", idx)
		}
		seen[idx] = true
	}

	out := make([]Tile, 0, len(indices))

	for _, idx := range indices {
		out = append(out, r.tiles[idx])
	}

	r.tiles = compactExcluding(r.tiles, indices)
	r.tiles = append(r.tiles, bag.Draw(len(indices))...)
	bag.Return(out)

	return nil
}

func matchRack(working []Tile, want Tile) int {
	if want.Blank {
		for i, tile := range working {
			if tile.IsUnassignedBlank() {
				return i
			}
		}
		return -1
	}

	for i, tile := range working {
		if !tile.Blank && tile.Letter == want.Letter {
			return i
		}
	}

	return -1
}

func compactExcluding(src []Tile, indices []int) []Tile {
	skip := make(map[int]bool, len(indices))

	for _, idx := range indices {
		skip[idx] = true
	}

	out := make([]Tile, 0, len(src)-len(indices))

	for i, tile := range src {
		if !skip[i] {
			out = append(out, tile)
		}
	}

	return out
}
