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
	"errors"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/tochemey/goakt/v4/actor"
	gerrors "github.com/tochemey/goakt/v4/errors"
)

// LobbyActor is the cluster-singleton entry point. Owns the code → room
// name directory; on JoinOrCreate it either returns an existing entry
// or SpawnOns a new RoomActor on the least-loaded peer.
type LobbyActor struct {
	rooms map[string]string
}

var _ actor.Actor = (*LobbyActor)(nil)

func (*LobbyActor) PreStart(*actor.Context) error { return nil }
func (*LobbyActor) PostStop(*actor.Context) error { return nil }

func (l *LobbyActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		l.rooms = make(map[string]string)
		ctx.Logger().Infof("lobby singleton ready on %s", ctx.Self().Path().HostPort())

	case *JoinOrCreate:
		l.handleJoinOrCreate(ctx, msg)

	default:
		ctx.Unhandled()
	}
}

func (l *LobbyActor) handleJoinOrCreate(ctx *actor.ReceiveContext, msg *JoinOrCreate) {
	language := strings.ToLower(strings.TrimSpace(msg.Language))

	registry := registryFromExtension(ctx.ActorSystem())
	if registry == nil {
		ctx.Response(&JoinOrCreateResult{Err: "no language registry"})
		return
	}

	if !slices.Contains(registry.Codes(), language) {
		ctx.Response(&JoinOrCreateResult{Err: "unsupported language: " + language})
		return
	}

	code := strings.ToUpper(strings.TrimSpace(msg.Room))

	if code != "" {
		if name, ok := l.rooms[code]; ok {
			ctx.Response(&JoinOrCreateResult{RoomCode: code, RoomName: name})
			return
		}
	}

	if code == "" {
		code = l.generateCode()
	}

	// Room name encodes language so a re-spawned RoomActor on another
	// node can recover its language without consulting the lobby.
	name := RoomActorPrefix + language + "." + strings.ToLower(code)

	pid, err := ctx.ActorSystem().SpawnOn(ctx.Context(), name, new(RoomActor),
		actor.WithLongLived(),
		actor.WithPlacement(actor.LeastLoad),
		actor.WithRelocationDisabled(),
		actor.WithStashing())
	if err != nil {
		if errors.Is(err, gerrors.ErrActorAlreadyExists) {
			l.rooms[code] = name
			ctx.Response(&JoinOrCreateResult{RoomCode: code, RoomName: name})
			return
		}

		ctx.Logger().Errorf("lobby SpawnOn(%s): %v", name, err)
		ctx.Response(&JoinOrCreateResult{Err: err.Error()})
		return
	}

	l.rooms[code] = name

	placement := "local"
	if pid != nil && pid.IsRemote() {
		placement = "remote@" + pid.Path().HostPort()
	}
	ctx.Logger().Infof("lobby: spawned %s (%s) for player %s [%s]", name, placement, msg.PlayerName, language)
	ctx.Response(&JoinOrCreateResult{RoomCode: code, RoomName: name})
}

// generateCode returns a 4-letter human-friendly room code. Excludes
// visually-ambiguous characters so codes are easy to dictate over voice.
func (l *LobbyActor) generateCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	for range 16 {
		code := make([]byte, 4)
		for i := range code {
			code[i] = alphabet[rand.IntN(len(alphabet))]
		}

		s := string(code)
		if _, taken := l.rooms[s]; !taken {
			return s
		}
	}

	code := make([]byte, 6)
	for i := range code {
		code[i] = alphabet[rand.IntN(len(alphabet))]
	}

	return string(code)
}
