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

import "math/rand/v2"

// wordBank — short, drawable nouns. Kept inline so the binary stays
// self-contained; a real deployment would ship a file or a localized
// list per language.
var wordBank = []string{
	// animals
	"cat", "dog", "elephant", "giraffe", "octopus", "penguin", "whale", "rabbit",
	"butterfly", "spider", "snake", "kangaroo", "hedgehog", "owl", "dolphin",
	// food
	"pineapple", "pizza", "hamburger", "sushi", "donut", "watermelon", "carrot",
	"cupcake", "popcorn", "sandwich", "spaghetti", "taco", "icecream",
	// objects
	"umbrella", "telephone", "guitar", "camera", "bicycle", "clock", "scissors",
	"backpack", "lamp", "anchor", "binoculars", "compass", "telescope", "balloon",
	// vehicles / scenes
	"spaceship", "helicopter", "submarine", "rocket", "lighthouse", "windmill",
	"castle", "pyramid", "skyscraper", "bridge", "treehouse",
	// fantasy / fun
	"dragon", "unicorn", "robot", "mermaid", "wizard", "ghost", "pirate", "alien",
	"vampire", "snowman", "skeleton",
	// nature
	"mountain", "rainbow", "tornado", "volcano", "waterfall", "cactus", "mushroom",
	"sunflower", "iceberg", "tree",
}

// pickWords returns n distinct random words from the bank. Used by the
// RoomActor when entering the choosing phase. The bank is small enough
// (~70 words) that Fisher–Yates on a fresh slice is cheap.
func pickWords(n int) []string {
	if n <= 0 {
		return nil
	}
	if n > len(wordBank) {
		n = len(wordBank)
	}
	pool := make([]string, len(wordBank))
	copy(pool, wordBank)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return pool[:n]
}

// maskWord returns a hint string for guessers: letters → underscores,
// non-letters (e.g. spaces) preserved. Used as the StateEvent.WordMask
// so the UI can show "_ _ _ _ _ _ _ _ _" while the round is in progress.
func maskWord(w string) string {
	out := make([]byte, len(w))
	for i := 0; i < len(w); i++ {
		c := w[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			out[i] = '_'
		default:
			out[i] = c
		}
	}
	return string(out)
}
