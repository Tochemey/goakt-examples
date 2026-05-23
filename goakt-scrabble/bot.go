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

	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-scrabble/scrabble"
)

// shutdownBot tells a BotActor to stop. Sent by RoomActor.removeBot.
type shutdownBot struct{}

// BotActor wraps the engine's greedy move generator. The room schedules
// a YourTurn to it on the bot's turn; the bot picks the best legal move
// and Tells the room a BotPlay. An empty Placements slice means pass.
//
// The actor name encodes the language so PostStart can fetch the right
// bundle from the system registry: "bot.<lang>.<roomCode>.<botID>".
type BotActor struct {
	bundle *LangBundle
}

var _ actor.Actor = (*BotActor)(nil)

func (*BotActor) PreStart(*actor.Context) error { return nil }
func (*BotActor) PostStop(*actor.Context) error { return nil }

func (b *BotActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		rest := strings.TrimPrefix(ctx.Self().Name(), BotActorPrefix)
		language, _, ok := strings.Cut(rest, ".")
		if !ok {
			ctx.Logger().Errorf("bot: malformed actor name %q", ctx.Self().Name())
			ctx.Shutdown()
			return
		}

		registry := registryFromExtension(ctx.ActorSystem())
		if registry == nil {
			ctx.Logger().Errorf("bot: no registry extension")
			ctx.Shutdown()
			return
		}

		bundle, err := registry.Get(language)
		if err != nil {
			ctx.Logger().Errorf("bot: %v", err)
			ctx.Shutdown()
			return
		}

		b.bundle = bundle

	case *YourTurn:
		b.handleTurn(ctx, msg)

	case *shutdownBot:
		ctx.Shutdown()

	default:
		ctx.Unhandled()
	}
}

func (b *BotActor) handleTurn(ctx *actor.ReceiveContext, msg *YourTurn) {
	board, err := wireToBoard(msg.Board, b.bundle.Lang)
	if err != nil {
		ctx.Logger().Errorf("bot %s: parse board: %v", msg.BotID, err)
		ctx.Tell(ctx.Self().Parent(), &BotPlay{BotID: msg.BotID})
		return
	}

	rack, err := wireToRack(msg.Rack, b.bundle.Lang)
	if err != nil {
		ctx.Logger().Errorf("bot %s: parse rack: %v", msg.BotID, err)
		ctx.Tell(ctx.Self().Parent(), &BotPlay{BotID: msg.BotID})
		return
	}

	best := scrabble.BestMove(board, rack, b.bundle.Dawg, b.bundle.Lang)

	if best == nil {
		ctx.Tell(ctx.Self().Parent(), &BotPlay{BotID: msg.BotID})
		return
	}

	wires := enginePlacementsToWire(best.Move.Placements, b.bundle.Lang)
	ctx.Tell(ctx.Self().Parent(), &BotPlay{BotID: msg.BotID, Placements: wires})
}

// wireToBoard rebuilds an engine.Board from the wire string grid the
// bot received in YourTurn.
func wireToBoard(grid [][]string, lang *scrabble.Language) (*scrabble.Board, error) {
	board := scrabble.NewBoard()

	for row, rowCells := range grid {
		for col, cell := range rowCells {
			if cell == "" {
				continue
			}
			tile, err := wireToTile(cell, lang)
			if err != nil {
				return nil, err
			}
			if err := board.Place(row, col, tile); err != nil {
				return nil, err
			}
		}
	}

	return board, nil
}

func wireToRack(tiles []string, lang *scrabble.Language) (*scrabble.Rack, error) {
	rack := scrabble.NewRack()

	parsed := make([]scrabble.Tile, 0, len(tiles))

	for _, raw := range tiles {
		tile, err := wireToTile(raw, lang)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, tile)
	}

	rack.Add(parsed)

	return rack, nil
}

func wireToTile(raw string, lang *scrabble.Language) (scrabble.Tile, error) {
	if raw == unassignedBlank {
		return scrabble.BlankTile, nil
	}

	letterPart := raw
	blank := false

	if strings.HasPrefix(raw, blankPrefix) {
		letterPart = raw[len(blankPrefix):]
		blank = true
	}

	runes := []rune(strings.ToUpper(letterPart))
	if len(runes) != 1 {
		return scrabble.Tile{}, fmt.Errorf("bot: bad wire tile %q", raw)
	}

	id, ok := lang.ID(runes[0])
	if !ok {
		return scrabble.Tile{}, fmt.Errorf("bot: bad wire tile %q", raw)
	}

	return scrabble.Tile{Letter: id, Blank: blank}, nil
}

func enginePlacementsToWire(placements []scrabble.Placement, lang *scrabble.Language) []PlacementWire {
	out := make([]PlacementWire, len(placements))

	for i, p := range placements {
		out[i] = PlacementWire{
			Row:    p.Row,
			Col:    p.Col,
			Letter: string(lang.Rune(p.Tile.Letter)),
			Blank:  p.Tile.Blank,
		}
	}

	return out
}
