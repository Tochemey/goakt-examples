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

package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/tochemey/goakt-examples/v2/goakt-scrabble/scrabble"
)

// blankPrefix marks a blank tile in the wire string form. "A" is a regular
// A; "*A" is a blank that has been assigned to A. An unassigned rack blank
// is "?".
const (
	blankPrefix     = "*"
	unassignedBlank = "?"
)

// boardToWire converts the engine board to the 15x15 string grid the
// browser renders. Empty squares are "".
func boardToWire(board *scrabble.Board, lang *scrabble.Language) [][]string {
	out := make([][]string, scrabble.BoardSize)

	for row := range scrabble.BoardSize {
		out[row] = make([]string, scrabble.BoardSize)
		for col := range scrabble.BoardSize {
			sq := board.At(row, col)
			if !sq.Filled {
				continue
			}
			out[row][col] = tileToWire(sq.Tile, lang)
		}
	}

	return out
}

func emptyBoardWire() [][]string {
	out := make([][]string, scrabble.BoardSize)
	for row := range scrabble.BoardSize {
		out[row] = make([]string, scrabble.BoardSize)
	}

	return out
}

// rackToWire converts a rack to the per-player tile-string array sent in
// StateEvent.PerRack. Unassigned blanks render as "?".
func rackToWire(rack *scrabble.Rack, lang *scrabble.Language) []string {
	tiles := rack.Tiles()
	out := make([]string, len(tiles))

	for i, tile := range tiles {
		out[i] = tileToWire(tile, lang)
	}

	return out
}

func tileToWire(tile scrabble.Tile, lang *scrabble.Language) string {
	if tile.IsUnassignedBlank() {
		return unassignedBlank
	}

	letter := string(lang.Rune(tile.Letter))

	if tile.Blank {
		return blankPrefix + letter
	}

	return letter
}

// toEnginePlacements converts wire placements (with letter strings) into
// engine Placements. Returns an error naming the first malformed entry.
func toEnginePlacements(wires []PlacementWire, lang *scrabble.Language) ([]scrabble.Placement, error) {
	out := make([]scrabble.Placement, 0, len(wires))

	for i, wire := range wires {
		if wire.Letter == "" {
			return nil, fmt.Errorf("placement %d has empty letter", i)
		}

		runes := []rune(strings.ToUpper(wire.Letter))
		if len(runes) != 1 {
			return nil, fmt.Errorf("placement %d letter %q must be a single character", i, wire.Letter)
		}

		id, ok := lang.ID(unicode.ToUpper(runes[0]))
		if !ok {
			return nil, fmt.Errorf("placement %d letter %q not in alphabet", i, wire.Letter)
		}

		out = append(out, scrabble.Placement{
			Row:  wire.Row,
			Col:  wire.Col,
			Tile: scrabble.Tile{Letter: id, Blank: wire.Blank},
		})
	}

	return out, nil
}

// tilesUsed returns the rack tiles consumed by a list of placements: a
// regular placement consumes its letter; a blank placement consumes an
// unassigned blank (Letter 0).
func tilesUsed(placements []scrabble.Placement) []scrabble.Tile {
	out := make([]scrabble.Tile, len(placements))

	for i, p := range placements {
		if p.Tile.Blank {
			out[i] = scrabble.BlankTile
		} else {
			out[i] = scrabble.Tile{Letter: p.Tile.Letter}
		}
	}

	return out
}

func formedWords(result *scrabble.MoveResult) []FormedWordWire {
	out := make([]FormedWordWire, len(result.Words))

	for i, formed := range result.Words {
		out[i] = FormedWordWire{Word: formed.Word, Score: formed.Score}
	}

	return out
}
