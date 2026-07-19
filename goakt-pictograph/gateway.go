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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
)

const (
	// lobbyAskTimeout bounds the wait on the singleton's JoinOrCreate
	// reply. Long enough to absorb cluster bootstrap; short enough that
	// a wedged singleton doesn't hang the WS upgrade.
	lobbyAskTimeout = 5 * time.Second

	// roomResolveDeadline is the upper bound on how long we retry
	// ActorOf for a freshly-spawned room. Same race window as goakt-tetris:
	// cross-node SpawnOn returns before the cluster's actor registry has
	// propagated to this node.
	roomResolveDeadline = 2 * time.Second
)

// requestRoom asks the cluster singleton lobby for a room and resolves
// the returned actor name to a usable PID. The PID may be remote;
// Tells to it are routed transparently.
func requestRoom(ctx context.Context, system actor.ActorSystem, msg *JoinOrCreate) (*actor.PID, string, error) {
	lobby, err := system.ActorOf(ctx, LobbyActorName)
	if err != nil {
		return nil, "", fmt.Errorf("locate lobby: %w", err)
	}

	reply, err := actor.Ask(ctx, lobby, msg, lobbyAskTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("lobby.JoinOrCreate: %w", err)
	}

	result, ok := reply.(*JoinOrCreateResult)
	if !ok {
		return nil, "", fmt.Errorf("unexpected lobby reply type %T", reply)
	}

	if result.Err != "" {
		return nil, "", errors.New(result.Err)
	}

	deadline := time.Now().Add(roomResolveDeadline)
	var lookupErr error
	for time.Now().Before(deadline) {
		room, err := system.ActorOf(ctx, result.RoomName)
		if err == nil && room != nil {
			return room, result.RoomCode, nil
		}

		lookupErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return nil, "", fmt.Errorf("resolve room %s: %w", result.RoomName, lookupErr)
}

// wsHandler upgrades to a WebSocket, joins or creates the room, spawns
// a per-connection PlayerSessionActor that owns the conn, and bridges
// inbound WS frames into typed actor messages on a reader goroutine.
//
// On disconnect the gateway emits GoodbyePlayer to the room *before*
// shutting the session — this is the fast-path teardown. The room also
// Watches every session PID as a safety net (cluster-aware in v4), but
// the failure-detector-driven Terminated arrives seconds later, so the
// explicit GoodbyePlayer keeps room state in sync the moment the
// socket closes.
//
// drainCtx is cancelled by the server's RegisterOnShutdown hook: when
// it fires, the reader loop unblocks from conn.Read and runs the same
// teardown path as a normal client-initiated close. wg lets main wait
// for every handler's teardown to complete before stopping the actor
// system (http.Server.Shutdown does NOT wait for hijacked connections).
func wsHandler(system actor.ActorSystem, leaderboard *Leaderboard, drainCtx context.Context, wg *sync.WaitGroup, logger log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wg.Add(1)
		defer wg.Done()

		// Accept-options: InsecureSkipVerify mirrors goakt-tetris's stance
		// for an unauthenticated demo. A real deployment would enforce
		// Origin.
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			logger.Errorf("ws accept: %v", err)
			return
		}

		q := r.URL.Query()
		name := strings.TrimSpace(q.Get("name"))
		if name == "" {
			name = "Player-" + shortID()
		}
		// Stable per-WS-connection player id. URL ?id= lets a returning
		// player pick up their grain (and thereby their wins/games-played
		// history) across reconnects.
		playerID := strings.TrimSpace(q.Get("id"))
		if playerID == "" {
			playerID = uuid.NewString()
		}
		room := strings.ToUpper(strings.TrimSpace(q.Get("room")))

		roomPID, code, err := requestRoom(r.Context(), system, &JoinOrCreate{
			Room: room, PlayerID: playerID, PlayerName: name,
		})
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "lobby unavailable")
			logger.Errorf("ws %v", err)
			return
		}
		leaderboard.RememberName(playerID, name)

		sessionName := SessionActorPrefix + uuid.NewString()
		session := &PlayerSessionActor{
			room:      roomPID,
			conn:      conn,
			playerID:  playerID,
			name:      name,
			roomCode:  code,
			topicName: RoomTopicPrefix + code,
		}

		sessionPID, err := system.Spawn(r.Context(), sessionName, session, actor.WithLongLived())
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "session spawn failed")
			logger.Errorf("session spawn (%s): %v", sessionName, err)
			return
		}

		matchLoc := "local"
		if roomPID.IsRemote() {
			matchLoc = "remote@" + roomPID.Path().HostPort()
		}

		logger.Infof("ws connected: name=%q session=%s room=%s (%s)",
			name, sessionName, code, matchLoc)

		// Merge r.Context() with the server-wide drain so the reader
		// unblocks either when the client disconnects OR when the
		// server starts shutting down. coder/websocket's conn.Read
		// returns the context error in either case, and the teardown
		// block below then delivers GoodbyePlayer and closes the conn.
		readCtx, cancelRead := context.WithCancel(r.Context())
		defer cancelRead()
		go func() {
			select {
			case <-drainCtx.Done():
				cancelRead()
			case <-readCtx.Done():
			}
		}()

		// Reader loop: the only blocking-I/O bridge into the actor world
		// for this connection.
		for {
			_, data, err := conn.Read(readCtx)
			if err != nil {
				var ce websocket.CloseError
				if !errors.As(err, &ce) {
					logger.Infof("ws read end: %s: %v", sessionName, err)
				}
				break
			}

			var in WSIn
			if err := json.Unmarshal(data, &in); err != nil {
				continue
			}

			_ = actor.Tell(r.Context(), sessionPID, &PlayerInput{PlayerID: playerID, In: in})
		}

		// Gateway-driven teardown — see top comment on the rationale.
		_ = actor.Tell(context.Background(), roomPID, &GoodbyePlayer{SessionName: sessionName})
		_ = actor.Tell(context.Background(), sessionPID, &closed{})
		_ = conn.Close(websocket.StatusNormalClosure, "")
		logger.Infof("ws closed: session=%s", sessionName)
	}
}

func shortID() string {
	return uuid.NewString()[:6]
}
