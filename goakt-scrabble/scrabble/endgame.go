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

// ScorelessTurnsToEnd is the official Scrabble cutoff for ending a game
// where no one is playing tiles: six consecutive scoreless turns (passes
// or zero-score exchanges).
const ScorelessTurnsToEnd = 6

// Value returns the sum of the rack's tile point values.
func (r *Rack) Value(lang *Language) int {
	total := 0

	for _, tile := range r.tiles {
		total += tile.Score(lang)
	}

	return total
}

// IsGameOver reports whether the game has ended after the most recent turn.
// Two conditions end a game:
//   - The bag is empty AND some player has emptied their rack.
//   - ScorelessTurnsToEnd consecutive scoreless turns have elapsed.
func IsGameOver(bag *Bag, racks []*Rack, scorelessTurns int) bool {
	if scorelessTurns >= ScorelessTurnsToEnd {
		return true
	}

	return bag.Remaining() == 0 && FirstEmptyRack(racks) >= 0
}

// FirstEmptyRack returns the index of the first rack with zero tiles, or
// -1 if every rack still holds tiles.
func FirstEmptyRack(racks []*Rack) int {
	for i, rack := range racks {
		if rack.Size() == 0 {
			return i
		}
	}

	return -1
}

// FinalScores applies end-of-game adjustments to current scores:
//   - Every player loses the sum of their rack's remaining tile values.
//   - If exactly one player went out (empty rack), they gain the sum of
//     every other player's remaining rack values.
//   - If no one went out (six-passes ending), only the rack penalty applies.
//
// currentScores and racks are parallel: index i in both is player i.
func FinalScores(currentScores []int, racks []*Rack, lang *Language) []int {
	out := make([]int, len(currentScores))
	copy(out, currentScores)

	rackValues := make([]int, len(racks))

	for i, rack := range racks {
		rackValues[i] = rack.Value(lang)
	}

	for i := range out {
		out[i] -= rackValues[i]
	}

	outIdx := FirstEmptyRack(racks)

	if outIdx >= 0 {
		for i, value := range rackValues {
			if i != outIdx {
				out[outIdx] += value
			}
		}
	}

	return out
}
