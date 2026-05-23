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
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	scannerInitialBuffer = 64 * 1024
	scannerMaxBuffer     = 1024 * 1024
)

// DAWG is a trie-shaped lookup structure over a language's words. It
// implements Dictionary and exposes edge traversal for the move generator.
type DAWG struct {
	lang *Language
	root *DAWGNode
	size int
}

// DAWGNode is one node in the DAWG. Edges are kept sorted by letter so
// callers can iterate alphabetically without re-sorting.
type DAWGNode struct {
	edges    []dawgEdge
	terminal bool
}

type dawgEdge struct {
	letter LetterID
	target *DAWGNode
}

// BuildDAWG reads one word per line from r. Comments (#), blank lines,
// words shorter than MinWordLength, and words with characters not in the
// alphabet are skipped silently.
func BuildDAWG(lang *Language, r io.Reader) (*DAWG, error) {
	dawg := &DAWG{lang: lang, root: &DAWGNode{}}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, scannerInitialBuffer), scannerMaxBuffer)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		ids, err := lang.NormalizeWord(line)
		if err != nil {
			continue
		}
		if len(ids) < MinWordLength {
			continue
		}

		if dawg.insert(ids) {
			dawg.size++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scrabble: reading wordlist: %w", err)
	}

	return dawg, nil
}

// Size returns the number of words inserted into the DAWG.
func (d *DAWG) Size() int {
	return d.size
}

// Root returns the entry point for move-generator traversal.
func (d *DAWG) Root() *DAWGNode {
	return d.root
}

// Contains reports whether word (already normalized to LetterIDs) is in
// the dictionary.
func (d *DAWG) Contains(word []LetterID) bool {
	if len(word) < MinWordLength {
		return false
	}

	node := d.root

	for _, id := range word {
		next, ok := node.Edge(id)
		if !ok {
			return false
		}
		node = next
	}

	return node.terminal
}

// ContainsString is a convenience wrapper that normalizes the word first.
func (d *DAWG) ContainsString(word string) bool {
	ids, err := d.lang.NormalizeWord(word)
	if err != nil {
		return false
	}

	return d.Contains(ids)
}

// Terminal reports whether the node represents the end of a valid word.
func (n *DAWGNode) Terminal() bool {
	return n.terminal
}

// Edge returns the child reached by following letter, or (nil, false) if
// no such edge exists.
func (n *DAWGNode) Edge(letter LetterID) (*DAWGNode, bool) {
	idx := sort.Search(len(n.edges), func(i int) bool {
		return n.edges[i].letter >= letter
	})

	if idx >= len(n.edges) || n.edges[idx].letter != letter {
		return nil, false
	}

	return n.edges[idx].target, true
}

// Each calls fn for every outgoing edge in sorted-letter order. Iteration
// stops early if fn returns false.
func (n *DAWGNode) Each(fn func(letter LetterID, next *DAWGNode) bool) {
	for _, edge := range n.edges {
		if !fn(edge.letter, edge.target) {
			return
		}
	}
}

func (d *DAWG) insert(word []LetterID) bool {
	node := d.root

	for _, id := range word {
		next, ok := node.Edge(id)
		if !ok {
			next = &DAWGNode{}
			node.addEdge(id, next)
		}
		node = next
	}

	if node.terminal {
		return false
	}

	node.terminal = true

	return true
}

func (n *DAWGNode) addEdge(letter LetterID, target *DAWGNode) {
	idx := sort.Search(len(n.edges), func(i int) bool {
		return n.edges[i].letter >= letter
	})

	n.edges = append(n.edges, dawgEdge{})
	copy(n.edges[idx+1:], n.edges[idx:])
	n.edges[idx] = dawgEdge{letter: letter, target: target}
}
