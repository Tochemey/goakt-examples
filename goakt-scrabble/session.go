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
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
	"github.com/tochemey/goakt/v4/actor"
)

const writeBudget = 80 * time.Millisecond

// closed is sent by the gateway reader when the WS ends.
type closed struct{}

// PlayerSessionActor bridges one WebSocket connection to a RoomActor.
// It owns the *websocket.Conn directly; writes are serialized through
// Receive so no second goroutine is needed for outbound frames.
type PlayerSessionActor struct {
	room      *actor.PID
	conn      *websocket.Conn
	playerID  string
	name      string
	roomCode  string
	topicName string
}

var _ actor.Actor = (*PlayerSessionActor)(nil)

func (*PlayerSessionActor) PreStart(*actor.Context) error { return nil }
func (*PlayerSessionActor) PostStop(*actor.Context) error { return nil }

func (p *PlayerSessionActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		topic := ctx.ActorSystem().TopicActor()
		if topic != nil {
			ctx.Tell(topic, actor.NewSubscribe(p.topicName))
		}
		ctx.Tell(p.room, &PlayerHello{
			PlayerID:    p.playerID,
			Name:        p.name,
			SessionName: ctx.Self().Name(),
		})

	case *actor.SubscribeAck, *actor.UnsubscribeAck:

	case *PlayerInput:
		ctx.Tell(p.room, msg)

	case *closed:
		ctx.Shutdown()

	default:
		p.encodeWS(ctx)
	}
}

func (p *PlayerSessionActor) encodeWS(ctx *actor.ReceiveContext) {
	var (
		target  string
		payload any
	)

	switch event := ctx.Message().(type) {
	case *JoinedEvent:
		target = event.For
		payload = map[string]any{
			"type":        OutTypeJoined,
			"room":        event.Room,
			"language":    event.Language,
			"playerID":    event.PlayerID,
			"owner":       event.Owner,
			"profile":     event.Profile,
			"leaderboard": event.Leaderboard,
		}
	case *StateEvent:
		target = event.For
		yours := event.PerRack[p.playerID]
		if yours == nil {
			yours = []string{}
		}
		payload = map[string]any{
			"type":         OutTypeState,
			"phase":        event.Phase,
			"board":        event.Board,
			"yourRack":     yours,
			"players":      event.Players,
			"currentID":    event.CurrentID,
			"ownerID":      event.OwnerID,
			"bagRemaining": event.BagRemaining,
			"timerMs":      event.TimerMs,
		}
	case *MoveEvent:
		target = event.For
		payload = map[string]any{
			"type":       OutTypeMove,
			"playerID":   event.PlayerID,
			"name":       event.Name,
			"placements": event.Placements,
			"words":      event.Words,
			"score":      event.Score,
			"newTotal":   event.NewTotal,
			"bingo":      event.Bingo,
		}
	case *ChatEvent:
		target = event.For
		payload = map[string]any{
			"type": OutTypeChat,
			"from": event.From,
			"text": event.Text,
		}
	case *ErrorEvent:
		target = event.For
		payload = map[string]any{
			"type":    OutTypeError,
			"message": event.Message,
		}
	case *GameOverEvent:
		target = event.For
		payload = map[string]any{
			"type":        OutTypeGameOver,
			"winnerID":    event.WinnerID,
			"winnerName":  event.WinnerName,
			"scores":      event.Scores,
			"leaderboard": event.Leaderboard,
		}
	default:
		ctx.Unhandled()
		return
	}

	if target != "" && target != p.playerID {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		ctx.Err(err)
		return
	}

	wctx, cancel := context.WithTimeout(ctx.Context(), writeBudget)
	err = p.conn.Write(wctx, websocket.MessageText, data)
	cancel()
	if err != nil {
		ctx.Logger().Infof("ws write failed for %s: %v", ctx.Self().Name(), err)
		ctx.Shutdown()
	}
}
