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

import "testing"

func TestEnglishLanguage(t *testing.T) {
	lang := English()

	if lang.AlphabetSize() != 26 {
		t.Fatalf("expected 26 letters, got %d", lang.AlphabetSize())
	}

	if lang.TotalTiles() != 100 {
		t.Fatalf("expected 100 tiles in English bag, got %d", lang.TotalTiles())
	}

	if lang.Blanks != 2 {
		t.Fatalf("expected 2 blanks, got %d", lang.Blanks)
	}

	checks := map[rune]struct {
		points int
		count  int
	}{
		'A': {1, 9},
		'E': {1, 12},
		'Q': {10, 1},
		'Z': {10, 1},
		'D': {2, 4},
		'K': {5, 1},
	}

	for r, want := range checks {
		id, ok := lang.ID(r)
		if !ok {
			t.Errorf("rune %q not in English alphabet", r)
			continue
		}
		if lang.PointValue(id) != want.points {
			t.Errorf("%q: points = %d, want %d", r, lang.PointValue(id), want.points)
		}
		if lang.Distribution[id] != want.count {
			t.Errorf("%q: count = %d, want %d", r, lang.Distribution[id], want.count)
		}
	}
}

func TestNormalizeWord(t *testing.T) {
	lang := English()

	ids, err := lang.NormalizeWord("Horse")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	want := []rune("HORSE")
	if len(ids) != len(want) {
		t.Fatalf("length mismatch: got %d want %d", len(ids), len(want))
	}

	for i, r := range want {
		if lang.Rune(ids[i]) != r {
			t.Errorf("position %d: got %q want %q", i, lang.Rune(ids[i]), r)
		}
	}

	if _, err := lang.NormalizeWord("hello!"); err == nil {
		t.Errorf("expected error for punctuation, got nil")
	}
}
