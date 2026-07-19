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
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
)

// Prefixes for the per-connection actor names. Combined with a UUID they
// give each WS connection its own private MatchActor + PlayerSessionActor
// pair. In a clustered setup these names are also how peers refer to the
// actor across the network.
const (
	matchActorPrefix   = "match."
	sessionActorPrefix = "session."

	// matchCreateTimeout caps how long the gateway waits for the
	// matchmaker singleton to reply with a freshly placed match. Generous
	// enough to absorb cluster bootstrap or a placement round-trip across
	// a slow link; short enough that a wedged matchmaker doesn't hang the
	// WS upgrade.
	matchCreateTimeout = 5 * time.Second
)

// matchResolveDeadline is the upper bound on how long requestMatch will
// keep retrying ActorOf for a freshly-spawned match. Cross-node SpawnOn
// returns before the cluster's actor registry has propagated to the
// caller, so the first lookup may legitimately race. 2 s is comfortably
// longer than any propagation we've observed.
const matchResolveDeadline = 2 * time.Second

// requestMatch asks the cluster matchmaker singleton for a fresh match and
// resolves it to a usable *PID. Encapsulated here so the WS handler stays
// readable. May return a remote PID — Tells will be routed transparently.
func requestMatch(ctx context.Context, system actor.ActorSystem) (*actor.PID, string, error) {
	mm, err := system.ActorOf(ctx, MatchmakerActorName)
	if err != nil {
		return nil, "", fmt.Errorf("locate matchmaker: %w", err)
	}
	reply, err := actor.Ask(ctx, mm, &CreateMatch{}, matchCreateTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("matchmaker.CreateMatch: %w", err)
	}
	created, ok := reply.(*MatchCreated)
	if !ok {
		return nil, "", fmt.Errorf("unexpected matchmaker reply type %T", reply)
	}

	// Cross-node ActorOf races SpawnOn's cluster-wide registration —
	// retry briefly so the first lookup after a remote spawn doesn't
	// fail spuriously.
	deadline := time.Now().Add(matchResolveDeadline)
	var lookupErr error
	for time.Now().Before(deadline) {
		match, err := system.ActorOf(ctx, created.MatchName)
		if err == nil && match != nil {
			return match, created.MatchName, nil
		}
		lookupErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return nil, "", fmt.Errorf("resolve match %s: %w", created.MatchName, lookupErr)
}

// wsHandler upgrades the request to a WebSocket and, for that connection,
// asks the cluster matchmaker for a fresh match, then spawns a local
// PlayerSessionActor that owns the connection. The reader loop here exists
// only because conn.Read blocks — it's the boundary between a blocking I/O
// API and the actor world. When the WS closes, the session unsubscribes;
// the match observes that via Watch / *Terminated and self-stops, taking
// its scheduled tick with it.
func wsHandler(system actor.ActorSystem, logger log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // Phase 2.1: no origin auth yet
		})
		if err != nil {
			logger.Errorf("ws accept: %v", err)
			return
		}

		match, matchName, err := requestMatch(r.Context(), system)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "matchmaker unavailable")
			logger.Errorf("ws %v", err)
			return
		}

		// One id per WS connection. Note we DON'T derive this from the
		// match name — that name belongs to the cluster and may have been
		// assigned by a remote matchmaker.
		sessionName := sessionActorPrefix + uuid.NewString()
		session := &PlayerSessionActor{match: match, conn: conn}
		sessionPID, err := system.Spawn(r.Context(), sessionName, session, actor.WithLongLived())
		if err != nil {
			_ = match.Shutdown(r.Context())
			_ = conn.Close(websocket.StatusInternalError, "session spawn failed")
			logger.Errorf("session spawn (%s): %v", sessionName, err)
			return
		}
		matchLoc := "local"
		if match.IsRemote() {
			matchLoc = "remote@" + match.Path().HostPort()
		}
		logger.Infof("ws connected: session=%s match=%s (%s)", sessionName, matchName, matchLoc)

		// Reader loop: blocks on Read until the client disconnects. Inbound
		// frames are parsed and forwarded to the session as typed actor
		// messages — this goroutine is the only blocking-I/O bridge into
		// the actor world for this connection.
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				var ce websocket.CloseError
				if !errors.As(err, &ce) {
					logger.Infof("ws read end: %s: %v", sessionName, err)
				}
				break
			}
			var in PlayerInput
			if err := json.Unmarshal(data, &in); err != nil || in.Type != MessageTypeInput {
				continue
			}
			_ = actor.Tell(r.Context(), sessionPID, &in)
		}

		// Gateway-driven teardown. We emit Unsubscribe *before* shutting
		// the session because (a) GoAkt's Watch/*Terminated is local-only
		// — a remote match will never see the session's death — and (b)
		// a Tell from inside a dying actor can be aborted in mid-flight
		// across the wire. This goroutine outlives both actors, so the
		// Tell here is reliable.
		_ = actor.Tell(context.Background(), match, &Unsubscribe{SessionName: sessionName})
		_ = actor.Tell(context.Background(), sessionPID, &closed{})
		_ = conn.Close(websocket.StatusNormalClosure, "")
		logger.Infof("ws closed: session=%s", sessionName)
	}
}
