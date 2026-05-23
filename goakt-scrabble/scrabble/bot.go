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

// maxAlphabetForBitset is the largest alphabet size that fits in the uint32
// cross-check bitsets used by the move generator.
const maxAlphabetForBitset = 32

// ScoredMove pairs a legal Move with its MoveResult.
type ScoredMove struct {
	Move   Move
	Result *MoveResult
}

// GenerateMoves returns every legal move available to the rack on the board,
// using the Appel/Jacobson 1988 algorithm. The list is empty if no legal
// placement exists.
func GenerateMoves(board *Board, rack *Rack, dawg *DAWG, lang *Language) []ScoredMove {
	gen := newGenState(board, rack, dawg, lang)

	gen.crossH = computeCrossChecks(board, dawg, lang, Vertical)
	gen.crossV = computeCrossChecks(board, dawg, lang, Horizontal)

	if board.IsEmptyBoard() {
		gen.runFromAnchor(CenterRow, CenterCol, Horizontal)
		gen.runFromAnchor(CenterRow, CenterCol, Vertical)
		return gen.scored()
	}

	gen.anchors = buildAnchorGrid(board)

	for row := range BoardSize {
		for col := range BoardSize {
			if !gen.anchors[row][col] {
				continue
			}
			gen.runFromAnchor(row, col, Horizontal)
			gen.runFromAnchor(row, col, Vertical)
		}
	}

	return gen.scored()
}

// BestMove returns the highest-scoring legal move for the rack, or nil
// if no legal move exists.
func BestMove(board *Board, rack *Rack, dawg *DAWG, lang *Language) *ScoredMove {
	moves := GenerateMoves(board, rack, dawg, lang)

	if len(moves) == 0 {
		return nil
	}

	best := &moves[0]

	for i := 1; i < len(moves); i++ {
		if moves[i].Result.Score > best.Result.Score {
			best = &moves[i]
		}
	}

	return best
}

// crossChecks holds, for each board position, the bitset of letters that
// would form a legal perpendicular cross-word at that square. A bit at
// index LetterID i means "letter i is valid here".
type crossChecks struct {
	valid [BoardSize][BoardSize]uint32
}

func (c *crossChecks) allows(row, col int, letter LetterID) bool {
	return c.valid[row][col]&(1<<letter) != 0
}

// genState bundles inputs and scratch state for one GenerateMoves call.
type genState struct {
	board     *Board
	rackCount []int        // index = LetterID, value = how many of that tile remain in rack
	blanks    int          // blank tiles remaining in rack
	dawg      *DAWG
	lang      *Language
	crossH    *crossChecks // cross-checks for HORIZONTAL moves (cross-words vertical)
	crossV    *crossChecks // cross-checks for VERTICAL moves (cross-words horizontal)
	anchors   [BoardSize][BoardSize]bool
	emitted   []Move
}

func newGenState(board *Board, rack *Rack, dawg *DAWG, lang *Language) *genState {
	state := &genState{
		board:     board,
		dawg:      dawg,
		lang:      lang,
		rackCount: make([]int, lang.AlphabetSize()),
	}

	for _, tile := range rack.tiles {
		if tile.Blank {
			state.blanks++
		} else {
			state.rackCount[tile.Letter]++
		}
	}

	return state
}

func (g *genState) scored() []ScoredMove {
	out := make([]ScoredMove, 0, len(g.emitted))

	for _, move := range g.emitted {
		result, err := move.Validate(g.board, g.dawg, g.lang)
		if err != nil {
			continue
		}
		out = append(out, ScoredMove{Move: move, Result: result})
	}

	return out
}

func (g *genState) runFromAnchor(row, col int, dir Direction) {
	leftLimit := g.computeLeftLimit(row, col, dir)

	// When the immediate-left square is filled, the word generated from
	// this anchor must include the existing left run as a DAWG prefix.
	if leftLimit == 0 {
		node, ok := g.walkLeftExisting(row, col, dir)
		if !ok {
			return
		}
		g.extendRight(node, nil, row, col, dir, false)
		return
	}

	g.extendLeft(g.dawg.Root(), nil, row, col, dir, leftLimit)
}

func (g *genState) walkLeftExisting(row, col int, dir Direction) (*DAWGNode, bool) {
	dRow, dCol := stepBack(dir)
	r, c := row+dRow, col+dCol

	if !InBounds(r, c) || !g.board.At(r, c).Filled {
		return g.dawg.Root(), true
	}

	for {
		prev := [2]int{r + dRow, c + dCol}
		if !InBounds(prev[0], prev[1]) || !g.board.At(prev[0], prev[1]).Filled {
			break
		}
		r, c = prev[0], prev[1]
	}

	node := g.dawg.Root()
	fwdR, fwdC := step(dir)

	for {
		next, ok := node.Edge(g.board.At(r, c).Tile.Letter)
		if !ok {
			return nil, false
		}
		node = next
		nr, nc := r+fwdR, c+fwdC
		if nr == row && nc == col {
			return node, true
		}
		r, c = nr, nc
	}
}

func (g *genState) computeLeftLimit(row, col int, dir Direction) int {
	dRow, dCol := stepBack(dir)
	limit := 0

	r := row + dRow
	c := col + dCol

	for InBounds(r, c) && !g.board.At(r, c).Filled && !g.anchors[r][c] {
		limit++
		r += dRow
		c += dCol
	}

	return limit
}

// partLetter is one letter of the left part. Positions are not stored
// because they depend on the final left-part length, only known at
// extendRight time.
type partLetter struct {
	Letter LetterID
	Blank  bool
}

// extendLeft enumerates left parts of length 0..leftLimit and triggers
// extendRight at every depth.
func (g *genState) extendLeft(node *DAWGNode, leftPart []partLetter, anchorRow, anchorCol int, dir Direction, leftLimit int) {
	leftPlacements := buildLeftPlacements(leftPart, anchorRow, anchorCol, dir)
	g.extendRight(node, leftPlacements, anchorRow, anchorCol, dir, false)

	if leftLimit == 0 {
		return
	}

	node.Each(func(letter LetterID, next *DAWGNode) bool {
		if g.rackCount[letter] > 0 {
			g.rackCount[letter]--
			extended := growLeftPart(leftPart, partLetter{Letter: letter})
			g.extendLeft(next, extended, anchorRow, anchorCol, dir, leftLimit-1)
			g.rackCount[letter]++
		}
		if g.blanks > 0 {
			g.blanks--
			extended := growLeftPart(leftPart, partLetter{Letter: letter, Blank: true})
			g.extendLeft(next, extended, anchorRow, anchorCol, dir, leftLimit-1)
			g.blanks++
		}
		return true
	})
}

func growLeftPart(prev []partLetter, next partLetter) []partLetter {
	out := make([]partLetter, len(prev)+1)
	copy(out, prev)
	out[len(prev)] = next

	return out
}

func buildLeftPlacements(leftPart []partLetter, anchorRow, anchorCol int, dir Direction) []Placement {
	if len(leftPart) == 0 {
		return nil
	}

	dRow, dCol := stepBack(dir)
	out := make([]Placement, len(leftPart))

	for i, p := range leftPart {
		offset := len(leftPart) - i
		out[i] = Placement{
			Row:  anchorRow + dRow*offset,
			Col:  anchorCol + dCol*offset,
			Tile: Tile{Letter: p.Letter, Blank: p.Blank},
		}
	}

	return out
}

// extendRight walks rightward from the current square. A move is recorded
// only when the DAWG node is terminal AND we have placed at or past the anchor.
func (g *genState) extendRight(node *DAWGNode, placements []Placement, row, col int, dir Direction, placedAtAnchor bool) {
	if !InBounds(row, col) {
		if node.Terminal() && placedAtAnchor {
			g.record(placements)
		}
		return
	}

	sq := g.board.At(row, col)
	dRow, dCol := step(dir)

	if !sq.Filled {
		if node.Terminal() && placedAtAnchor {
			g.record(placements)
		}

		cross := g.crossFor(dir)

		node.Each(func(letter LetterID, next *DAWGNode) bool {
			if !cross.allows(row, col, letter) {
				return true
			}

			if g.rackCount[letter] > 0 {
				g.rackCount[letter]--
				grown := appendPlacement(placements, Placement{Row: row, Col: col, Tile: Tile{Letter: letter}})
				g.extendRight(next, grown, row+dRow, col+dCol, dir, true)
				g.rackCount[letter]++
			}

			if g.blanks > 0 {
				g.blanks--
				grown := appendPlacement(placements, Placement{Row: row, Col: col, Tile: Tile{Letter: letter, Blank: true}})
				g.extendRight(next, grown, row+dRow, col+dCol, dir, true)
				g.blanks++
			}

			return true
		})

		return
	}

	if next, ok := node.Edge(sq.Tile.Letter); ok {
		g.extendRight(next, placements, row+dRow, col+dCol, dir, placedAtAnchor)
	}
}

func (g *genState) crossFor(dir Direction) *crossChecks {
	if dir == Horizontal {
		return g.crossH
	}

	return g.crossV
}

func (g *genState) record(placements []Placement) {
	if len(placements) == 0 {
		return
	}

	copied := make([]Placement, len(placements))
	copy(copied, placements)
	g.emitted = append(g.emitted, Move{Placements: copied})
}

// computeCrossChecks builds the per-square cross-check bitset. perpDir is
// the direction of the cross-word formed by a placement.
func computeCrossChecks(board *Board, dawg *DAWG, lang *Language, perpDir Direction) *crossChecks {
	cs := &crossChecks{}
	all := allLettersMask(lang)
	dRow, dCol := step(perpDir)

	for row := range BoardSize {
		for col := range BoardSize {
			if board.At(row, col).Filled {
				continue
			}

			prefix := walkExisting(board, row-dRow, col-dCol, -dRow, -dCol)
			reverseInPlace(prefix)
			suffix := walkExisting(board, row+dRow, col+dCol, dRow, dCol)

			if len(prefix) == 0 && len(suffix) == 0 {
				cs.valid[row][col] = all
				continue
			}

			var bits uint32

			for id := range lang.AlphabetSize() {
				candidate := make([]LetterID, 0, len(prefix)+1+len(suffix))
				candidate = append(candidate, prefix...)
				candidate = append(candidate, LetterID(id))
				candidate = append(candidate, suffix...)
				if dawg.Contains(candidate) {
					bits |= 1 << id
				}
			}

			cs.valid[row][col] = bits
		}
	}

	return cs
}

func walkExisting(board *Board, row, col, dRow, dCol int) []LetterID {
	var out []LetterID

	for InBounds(row, col) && board.At(row, col).Filled {
		out = append(out, board.At(row, col).Tile.Letter)
		row += dRow
		col += dCol
	}

	return out
}

func reverseInPlace(s []LetterID) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func allLettersMask(lang *Language) uint32 {
	if lang.AlphabetSize() >= maxAlphabetForBitset {
		return ^uint32(0)
	}

	return (uint32(1) << lang.AlphabetSize()) - 1
}

func buildAnchorGrid(board *Board) [BoardSize][BoardSize]bool {
	var grid [BoardSize][BoardSize]bool

	for row := range BoardSize {
		for col := range BoardSize {
			if board.At(row, col).Filled {
				continue
			}
			if hasFilledNeighbor(board, row, col) {
				grid[row][col] = true
			}
		}
	}

	return grid
}

func hasFilledNeighbor(board *Board, row, col int) bool {
	neighbors := [4][2]int{{row - 1, col}, {row + 1, col}, {row, col - 1}, {row, col + 1}}

	for _, n := range neighbors {
		if InBounds(n[0], n[1]) && board.At(n[0], n[1]).Filled {
			return true
		}
	}

	return false
}

func step(dir Direction) (int, int) {
	if dir == Horizontal {
		return 0, 1
	}

	return 1, 0
}

func stepBack(dir Direction) (int, int) {
	r, c := step(dir)
	return -r, -c
}

func appendPlacement(head []Placement, tail Placement) []Placement {
	out := make([]Placement, 0, len(head)+1)
	out = append(out, head...)
	out = append(out, tail)

	return out
}
