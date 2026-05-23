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

// Tile is a single Scrabble tile. A placed blank carries the chosen letter
// and Blank == true, so it scores 0 but otherwise behaves as its letter.
type Tile struct {
	Letter LetterID
	Blank  bool
}

// BlankTile is the in-rack representation of an unassigned blank.
var BlankTile = Tile{Blank: true}

// IsUnassignedBlank reports whether t is a blank with no chosen letter yet.
func (t Tile) IsUnassignedBlank() bool {
	return t.Blank && t.Letter == 0
}

// Score returns the tile's score before premium-square multipliers.
func (t Tile) Score(lang *Language) int {
	if t.Blank {
		return 0
	}

	return lang.PointValue(t.Letter)
}
