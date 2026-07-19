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

// writeBudget caps how long one outbound JSON frame may take to land on
// the socket. A slow client trips it, the actor logs and self-stops —
// that's our backpressure. Same posture as goakt-tetris.
const writeBudget = 80 * time.Millisecond

// closed is sent by the gateway reader goroutine when the WS ends.
type closed struct{}

// PlayerSessionActor bridges one WebSocket connection to a RoomActor.
// It owns the *websocket.Conn directly; Receive serializes all I/O so
// writes happen inline (no second goroutine for sends). The reader
// side, which has to block in conn.Read, lives in the gateway.
//
// On start it subscribes itself to the room's pub/sub topic so any
// event the room publishes is delivered into its mailbox — including
// events targeted at OTHER players. Filtering by Target field happens
// in encodeWS below.
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
		// Subscribe before any room messages can route back to us.
		topic := ctx.ActorSystem().TopicActor()
		if topic != nil {
			ctx.Tell(topic, actor.NewSubscribe(p.topicName))
		}
		// Now announce ourselves to the room. The room will record
		// ctx.Sender() (this actor) and watch it.
		ctx.Tell(p.room, &PlayerHello{
			PlayerID:    p.playerID,
			Name:        p.name,
			SessionName: ctx.Self().Name(),
		})

	case *actor.SubscribeAck, *actor.UnsubscribeAck:
		// expected, nothing to do

	case *PlayerInput:
		// Browser → session (forwarded by gateway). Tag with our playerID
		// and ship to the room.
		ctx.Tell(p.room, msg)

	case *closed:
		ctx.Shutdown()

	default:
		// Every other message we expect is an outbound game event from
		// the room/topic. Route through the encoder.
		p.encodeWS(ctx)
	}
}

// encodeWS performs the type-switch over outbound event types, filters
// by Target, builds the wire JSON, and writes it to the socket. Keeping
// the switch here (rather than in main/room) lets the session also
// inject per-recipient bits like YouAreDrawer that the room can't
// know about in a broadcast topic.
func (p *PlayerSessionActor) encodeWS(ctx *actor.ReceiveContext) {
	msg := ctx.Message()
	var (
		target  string
		payload any
	)
	switch event := msg.(type) {
	case *JoinedEvent:
		target = event.For
		payload = map[string]any{
			"type":        OutTypeJoined,
			"room":        event.Room,
			"playerID":    event.PlayerID,
			"profile":     event.Profile,
			"leaderboard": event.Leaderboard,
		}
	case *StateEvent:
		target = event.For
		payload = map[string]any{
			"type":         OutTypeState,
			"phase":        event.Phase,
			"round":        event.Round,
			"maxRounds":    event.MaxRounds,
			"timeLeft":     event.TimeLeft,
			"players":      event.Players,
			"drawerID":     event.DrawerID,
			"wordMask":     event.WordMask,
			"youAreDrawer": event.DrawerID == p.playerID,
		}
	case *StrokeEvent:
		target = event.For
		payload = map[string]any{
			"type":   OutTypeStroke,
			"points": event.Points,
			"color":  event.Color,
			"width":  event.Width,
		}
	case *ClearEvent:
		target = event.For
		payload = map[string]any{"type": OutTypeClear}
	case *ChatEvent:
		target = event.For
		payload = map[string]any{
			"type": OutTypeChat,
			"from": event.From,
			"text": event.Text,
		}
	case *GuessedEvent:
		target = event.For
		payload = map[string]any{
			"type":     OutTypeGuessed,
			"playerID": event.PlayerID,
			"name":     event.Name,
		}
	case *ScoreEvent:
		target = event.For
		payload = map[string]any{
			"type":     OutTypeScore,
			"playerID": event.PlayerID,
			"delta":    event.Delta,
			"total":    event.Total,
		}
	case *WordChoicesEvent:
		target = event.For
		payload = map[string]any{
			"type":    OutTypeWordChoices,
			"choices": event.Choices,
		}
	case *SecretWordEvent:
		target = event.For
		payload = map[string]any{
			"type": OutTypeSecretWord,
			"word": event.Word,
		}
	case *RoundOverEvent:
		target = event.For
		payload = map[string]any{
			"type":   OutTypeRoundOver,
			"word":   event.Word,
			"scores": event.Scores,
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
	case *ErrorEvent:
		target = event.For
		payload = map[string]any{
			"type":    OutTypeError,
			"message": event.Message,
		}
	default:
		ctx.Unhandled()
		return
	}

	// Target filter — empty means broadcast (everyone in the topic
	// receives, everyone writes); non-empty means only that player's
	// session forwards to the WS.
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
