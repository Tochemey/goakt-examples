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
	"strings"

	"github.com/tochemey/goakt/v4/actor"
	gerrors "github.com/tochemey/goakt/v4/errors"
)

// LobbyActor is the cluster-singleton entry point. It owns a tiny
// directory mapping room codes → actor names. When a gateway asks for
// a room, the lobby either returns an existing entry or mints a new
// code and spawns the RoomActor on whichever cluster node has the
// lightest load.
//
// On singleton failover the rebuilt LobbyActor starts with an empty
// directory — it does NOT enumerate the cluster's existing rooms. The
// directory recovers lazily: a player presenting an existing room
// code (via URL ?room=) triggers a SpawnOn that returns
// ErrActorAlreadyExists, which handleJoinOrCreate treats as success
// and re-registers the code → name mapping. Codes that nobody is
// holding (no URL share, no active player) are effectively lost.
type LobbyActor struct {
	// rooms is a local map of code → cluster-stable RoomActor name.
	// The PIDs themselves aren't cached — `ActorOf` resolves them
	// fresh per request so cross-node placement is transparent.
	rooms map[string]string
}

var _ actor.Actor = (*LobbyActor)(nil)

func (*LobbyActor) PreStart(*actor.Context) error { return nil }
func (*LobbyActor) PostStop(*actor.Context) error { return nil }

func (l *LobbyActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		// Initialize state here — SpawnSingleton uses `new(LobbyActor)`
		// so a constructor that allocated maps would be bypassed.
		l.rooms = make(map[string]string)
		ctx.Logger().Infof("lobby singleton ready on %s", ctx.Self().Path().HostPort())

	case *JoinOrCreate:
		l.handleJoinOrCreate(ctx, msg)

	default:
		ctx.Unhandled()
	}
}

func (l *LobbyActor) handleJoinOrCreate(ctx *actor.ReceiveContext, msg *JoinOrCreate) {
	code := strings.ToUpper(strings.TrimSpace(msg.Room))

	// Existing room? Just hand back the actor name; gateway resolves
	// the PID via ActorOf so cross-node placement stays transparent.
	if code != "" {
		if name, ok := l.rooms[code]; ok {
			ctx.Response(&JoinOrCreateResult{RoomCode: code, RoomName: name})
			return
		}
		// Caller explicitly asked for a code we don't know about — pretend
		// they wanted a new one with that code. Lets URL-shared codes
		// remain stable across server restarts during the demo.
	}

	if code == "" {
		code = l.generateCode()
	}

	name := RoomActorPrefix + strings.ToLower(code)

	// LeastLoad places new rooms on the least-loaded peer. In single-
	// node mode this trivially picks the local node. WithRelocationDisabled
	// keeps a room pinned to its origin — mid-game relocation would just
	// drop the live state on the floor, so we prefer to let a room die
	// with its host. Same rationale as goakt-tetris's MatchFactory.
	// new(RoomActor) is intentional — SpawnOn re-instantiates on the
	// chosen node via the kind registry anyway. The room derives its
	// code from its actor name and reads the leaderboard from system
	// extensions; see RoomActor PostStart.
	pid, err := ctx.ActorSystem().SpawnOn(ctx.Context(), name, new(RoomActor),
		actor.WithLongLived(),
		actor.WithPlacement(actor.LeastLoad),
		actor.WithRelocationDisabled(),
		actor.WithStashing()) // needed so the choosing-phase guess stash works
	if err != nil {
		// A room with this name already exists in the cluster registry.
		// This happens after a singleton failover (the rebuilt lobby's
		// local directory is empty but the room actors persist) or when
		// two JoinOrCreate calls for the same URL-supplied code race.
		// Either way the right thing is to point the gateway at the
		// already-running room.
		if errors.Is(err, gerrors.ErrActorAlreadyExists) {
			l.rooms[code] = name
			ctx.Logger().Infof("lobby: reusing existing %s for player %s", name, msg.PlayerName)
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
	ctx.Logger().Infof("lobby: spawned %s (%s) for player %s", name, placement, msg.PlayerName)
	ctx.Response(&JoinOrCreateResult{RoomCode: code, RoomName: name})
}

// generateCode returns a 4-letter human-friendly room code. Excludes
// visually-ambiguous letters (I/O/0/1) so people can dictate codes
// over voice. Retries on collision; 26^4 ≈ 460k codes is plenty for
// a demo.
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

	// Astronomical collision rate — fall back to a longer code.
	code := make([]byte, 6)
	for i := range code {
		code[i] = alphabet[rand.IntN(len(alphabet))]
	}
	return string(code)
}
