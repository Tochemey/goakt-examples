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
	"strings"
	"testing"
)

// testWords is the small wordlist used by every scoring / move / bot test.
// Kept small so the bot's move generator runs in milliseconds.
var testWords = strings.Join([]string{
	"AT", "BE", "BIT", "BITE", "DO", "GO", "HE", "HEY",
	"HI", "HO", "HORSE", "HORSES", "HOSIERY", "HOT", "IN", "IS",
	"IT", "JET", "JO", "MY", "NO", "ON", "OR", "QI", "RACE",
	"SAY", "SIR", "SIT", "SO", "TO", "WAY", "WE", "ZA", "ZIT",
}, "\n")

func newTestDAWG(t *testing.T) (*DAWG, *Language) {
	t.Helper()

	lang := English()
	dawg, err := BuildDAWG(lang, strings.NewReader(testWords))
	if err != nil {
		t.Fatalf("build dawg: %v", err)
	}

	return dawg, lang
}

func TestDAWGContains(t *testing.T) {
	dawg, _ := newTestDAWG(t)

	hits := []string{"horse", "HORSE", "HORSES", "QI", "ZA"}

	for _, w := range hits {
		if !dawg.ContainsString(w) {
			t.Errorf("expected %q to be in dawg", w)
		}
	}

	misses := []string{"HORZ", "ZZZ", "HORS"}

	for _, w := range misses {
		if dawg.ContainsString(w) {
			t.Errorf("expected %q NOT to be in dawg", w)
		}
	}
}

func TestDAWGSize(t *testing.T) {
	dawg, _ := newTestDAWG(t)

	wantSize := len(strings.Split(strings.TrimSpace(testWords), "\n"))

	if dawg.Size() != wantSize {
		t.Errorf("dawg size: got %d want %d", dawg.Size(), wantSize)
	}
}

func TestDAWGSkipsShortAndBadInput(t *testing.T) {
	lang := English()
	input := "OK\nA\nFOO!BAR\n# comment\n\nBAR"

	dawg, err := BuildDAWG(lang, strings.NewReader(input))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if !dawg.ContainsString("OK") {
		t.Error("OK should be present")
	}

	if !dawg.ContainsString("BAR") {
		t.Error("BAR should be present")
	}

	if dawg.ContainsString("A") {
		t.Error("single-letter A should be skipped")
	}

	if dawg.Size() != 2 {
		t.Errorf("expected size 2, got %d", dawg.Size())
	}
}

func TestDAWGEdgeTraversal(t *testing.T) {
	dawg, lang := newTestDAWG(t)

	hID, _ := lang.ID('H')
	oID, _ := lang.ID('O')

	hNode, ok := dawg.Root().Edge(hID)
	if !ok {
		t.Fatal("root has no H edge")
	}

	if hNode.Terminal() {
		t.Error("H alone shouldn't be terminal")
	}

	if _, ok := hNode.Edge(oID); !ok {
		t.Error("H -> O should exist (HO, HORSE, HOT)")
	}
}
