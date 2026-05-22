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

const (
	BoardW = 10
	BoardH = 20
)

// MessageType is the discriminator value for PlayerInput.Type. There is
// only one inbound message kind today, but the field exists so we can add
// more later without breaking the wire format.
const MessageTypeInput = "input"

// MatchmakerActorName is the cluster-singleton name of the MatchFactory.
// Gateway goroutines resolve this via ActorOf to request a fresh match.
const MatchmakerActorName = "matchmaker"

// CreateMatch is the Ask-message sent by the gateway to the matchmaker
// when a new WS connection arrives. Response is *MatchCreated.
type CreateMatch struct{}

// MatchCreated is the matchmaker's reply to CreateMatch. The gateway uses
// the name to resolve a usable *PID via ActorOf — SpawnOn may have placed
// the actual MatchActor on a remote node, so we don't ship the PID itself.
type MatchCreated struct {
	MatchName string `json:"matchName"`
}

// Action strings — the wire-protocol contract between server (match.go)
// and client (web/main.js). The JS side has a mirror copy in main.js;
// keep these two in sync.
const (
	ActionLeft        = "left"
	ActionRight       = "right"
	ActionRotate      = "rotate"
	ActionSoftDrop    = "softdrop"
	ActionSoftDropEnd = "softdrop-end"
	ActionHardDrop    = "harddrop"
	ActionPause       = "pause"
	ActionRestart     = "restart"
)

// PlayerInput is sent by the browser on every keypress (and on key release
// for softdrop). Type discriminates the inbound message kind; Action is
// one of the Action* constants above.
type PlayerInput struct {
	Type   string `json:"type"`
	Action string `json:"action"`
}

// PieceState is the falling tetromino. Cells are derived from Kind+Rot via
// the same shape table on the server and client so the wire payload stays
// tiny (4 ints instead of 4 cell coordinates).
type PieceState struct {
	Kind int `json:"kind"` // 0..6 = I, O, T, S, Z, J, L
	Rot  int `json:"rot"`  // 0..3
	X    int `json:"x"`    // pivot column
	Y    int `json:"y"`    // pivot row, 0 = top
}

// Snapshot is the wire payload sent every tick. Grid is a flat 2-D slice;
// 0 = empty cell, 1..7 = piece kind that filled the cell (for color).
// GhostY is the y-coordinate the piece would land at if hard-dropped — the
// client renders an outline there as a landing hint.
type Snapshot struct {
	Tick     int        `json:"tick"`
	T        int64      `json:"t"`
	Grid     [][]int8   `json:"grid"`
	Piece    PieceState `json:"piece"`
	GhostY   int        `json:"ghostY"`
	Next     int        `json:"next"`
	Score    int        `json:"score"`
	Lines    int        `json:"lines"`
	Level    int        `json:"level"`
	GameOver bool       `json:"gameOver"`
	Paused   bool       `json:"paused"`
}

// tick is the internal scheduled message that drives the gravity loop.
type tick struct{}

// Subscribe registers the sender as a subscriber for direct snapshot
// delivery. The match identifies the subscriber via ctx.Sender() — we
// intentionally do not embed an *actor.PID in the wire payload because
// PIDs contain unexported fields that don't survive CBOR/cluster
// serialization. ctx.Sender() is cluster-aware and always correct.
type Subscribe struct{}

// Unsubscribe removes the named subscriber from the match's broadcast
// list. SessionName is the actor name of the PlayerSessionActor to
// remove — we identify by string rather than by ctx.Sender() because
// the gateway (not the session itself) sends this message, so the sender
// in the receive-context isn't the subscriber being removed.
//
// Why the gateway and not the session: in cluster mode the session must
// shut down its WS-write loop the instant the socket closes, and a
// Tell(match) from inside the dying session is fire-and-forget — the
// cross-node packet can be aborted before it flushes. The gateway lives
// outside the actor system and can reliably emit Unsubscribe (and wait
// briefly via its own ctx) before tearing the session down.
type Unsubscribe struct {
	SessionName string `json:"sessionName"`
}
