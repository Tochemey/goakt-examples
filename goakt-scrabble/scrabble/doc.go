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

// Package scrabble is the pure-Go Scrabble engine used by goakt-scrabble.
//
// It has no actor or networking dependencies — every type here is safe
// to use from unit tests and from inside an actor's Receive without
// touching the goakt runtime. The engine covers:
//
//   - Per-language tile distributions and point values (Language)
//   - A 15x15 board with the standard premium-square layout (Board)
//   - The shuffled tile bag (Bag) and the player rack (Rack)
//   - Dictionary lookup and the DAWG used for move generation
//   - Move validation + scoring (Move)
//   - End-of-game detection and rack-penalty scoring (Endgame)
//   - A greedy Appel/Jacobson move generator used by the bot
//
// The actor layer (LobbyActor / RoomActor / BotActor / SessionActor)
// lives in the parent package and wraps these types; nothing in this
// package depends on goakt.
package scrabble
