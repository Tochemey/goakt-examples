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
	lobbyAskTimeout       = 5 * time.Second
	roomResolveDeadline   = 2 * time.Second
	roomResolveStartDelay = 10 * time.Millisecond
	roomResolveMaxDelay   = 200 * time.Millisecond
	defaultLanguageCode   = "en"
)

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

	return resolveRoom(ctx, system, result)
}

// resolveRoom polls the cluster registry for a freshly-spawned room.
// SpawnOn returns before cross-node registration propagates; ActorOf
// fails until it does. Exponential backoff (10 → 20 → 40 → 80 → 160 →
// 200 ms cap) keeps the common case fast while bounded by deadline +
// context cancellation.
func resolveRoom(ctx context.Context, system actor.ActorSystem, result *JoinOrCreateResult) (*actor.PID, string, error) {
	deadline := time.Now().Add(roomResolveDeadline)
	delay := roomResolveStartDelay
	var lookupErr error

	for {
		room, err := system.ActorOf(ctx, result.RoomName)
		if err == nil && room != nil {
			return room, result.RoomCode, nil
		}
		lookupErr = err

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, "", fmt.Errorf("resolve room %s: %w", result.RoomName, lookupErr)
		}

		wait := min(delay, remaining)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}

		if delay < roomResolveMaxDelay {
			delay *= 2
		}
	}
}

func wsHandler(system actor.ActorSystem, leaderboard *Leaderboard, drainCtx context.Context, wg *sync.WaitGroup, logger log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wg.Add(1)
		defer wg.Done()

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
		playerID := strings.TrimSpace(q.Get("id"))
		if playerID == "" {
			playerID = uuid.NewString()
		}
		language := strings.ToLower(strings.TrimSpace(q.Get("lang")))
		if language == "" {
			language = defaultLanguageCode
		}
		room := strings.ToUpper(strings.TrimSpace(q.Get("room")))

		roomPID, code, err := requestRoom(r.Context(), system, &JoinOrCreate{
			Room: room, Language: language, PlayerID: playerID, PlayerName: name,
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

		placement := "local"
		if roomPID.IsRemote() {
			placement = "remote@" + roomPID.Path().HostPort()
		}

		logger.Infof("ws connected: name=%q session=%s room=%s lang=%s (%s)",
			name, sessionName, code, language, placement)

		readCtx, cancelRead := context.WithCancel(r.Context())
		defer cancelRead()
		go func() {
			select {
			case <-drainCtx.Done():
				cancelRead()
			case <-readCtx.Done():
			}
		}()

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

		_ = actor.Tell(context.Background(), roomPID, &GoodbyePlayer{SessionName: sessionName})
		_ = actor.Tell(context.Background(), sessionPID, &closed{})
		_ = conn.Close(websocket.StatusNormalClosure, "")
		logger.Infof("ws closed: session=%s", sessionName)
	}
}
