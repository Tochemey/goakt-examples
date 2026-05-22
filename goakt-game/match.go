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
	"math/rand/v2"
	"time"

	"github.com/tochemey/goakt/v4/actor"
)

const (
	tickInterval = 16 * time.Millisecond // 60 Hz — keeps input snappy

	// schedRefPrefix is combined with the match actor's own name to produce
	// a globally-unique reference for its tick schedule. (`Schedule`
	// references collide system-wide, so this MUST be unique per match.)
	schedRefPrefix = "tick."

	// Gravity rate in ticks per piece drop. Higher level = faster drop.
	gravityStart  = 50 // ~0.83 s/drop at level 1
	gravityMin    = 6  // fastest (level ≥ 10), ~0.1 s/drop
	gravityPerLvl = 4  // ticks subtracted per level

	// Soft-drop holds Down; drops every N ticks while held.
	softDropTicks = 3

	// hardDropBonus is added to score per cell traveled on a hard drop.
	hardDropBonus = 2
)

// Tetromino kind indices into pieceShapes. Used for the wire payload and
// to short-circuit O-piece rotation.
const (
	kindI = iota
	kindO
	kindT
	kindS
	kindZ
	kindJ
	kindL
)

// pieceShapes — base orientation of each tetromino, as 4 (dx, dy) cell
// offsets from the piece's pivot. Server and client share the same table
// (mirrored in main.js) so the wire payload only needs piece kind + rot.
var pieceShapes = [7][4][2]int{
	{{-1, 0}, {0, 0}, {1, 0}, {2, 0}},  // I
	{{0, 0}, {1, 0}, {0, 1}, {1, 1}},   // O
	{{-1, 0}, {0, 0}, {1, 0}, {0, 1}},  // T
	{{0, 0}, {1, 0}, {-1, 1}, {0, 1}},  // S
	{{-1, 0}, {0, 0}, {0, 1}, {1, 1}},  // Z
	{{-1, 0}, {0, 0}, {1, 0}, {1, 1}},  // J
	{{-1, 0}, {0, 0}, {1, 0}, {-1, 1}}, // L
}

// pieceCells returns the cell offsets for a piece in a given rotation by
// applying 90° clockwise rotation `rot` times to the base shape. (Each
// rotation step maps (dx,dy) → (-dy, dx).) The O piece (kind == 1) is
// intentionally never rotated — callers gate this.
func pieceCells(kind, rot int) [4][2]int {
	cells := pieceShapes[kind]
	for i := 0; i < rot; i++ {
		for j := range cells {
			cells[j] = [2]int{-cells[j][1], cells[j][0]}
		}
	}
	return cells
}

// MatchActor owns one Tetris game: the board, the active piece, the next
// piece, the score/lines/level, and the gravity countdown. Tick advances
// the gravity countdown; once it hits zero the piece drops by 1 row (or
// locks if it can't). PlayerInput messages mutate the piece position.
type MatchActor struct {
	grid     [BoardH][BoardW]int8
	piece    PieceState
	nextKind int
	score    int
	lines    int
	level    int
	gameOver bool

	softDrop bool
	paused   bool
	gravity  int // ticks/drop
	fallTick int // ticks until next fall

	tickN    int
	subs     []*actor.PID
	schedRef string // reference id for our tick schedule

	// hadSubscriber prevents the actor from self-stopping during the brief
	// window between PostStart and the owner's first Subscribe message.
	hadSubscriber bool
}

var _ actor.Actor = (*MatchActor)(nil)

func (*MatchActor) PreStart(*actor.Context) error { return nil }

// PostStop cancels the recurring tick so no further messages are scheduled
// to a stopped actor (they'd end up in the dead-letter queue).
func (m *MatchActor) PostStop(ctx *actor.Context) error {
	if m.schedRef != "" {
		_ = ctx.ActorSystem().CancelSchedule(m.schedRef)
	}
	return nil
}

func (m *MatchActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		m.reset()
		m.schedRef = schedRefPrefix + ctx.Self().Name()
		if err := ctx.ActorSystem().Schedule(ctx.Context(), &tick{}, ctx.Self(),
			tickInterval, actor.WithReference(m.schedRef)); err != nil {
			ctx.Err(err)
		}

	case *Subscribe:
		sender := ctx.Sender()
		m.subs = append(m.subs, sender)
		m.hadSubscriber = true
		ctx.Watch(sender) // self-stop if the subscriber dies

	case *Unsubscribe:
		// Gateway-driven. Identified by actor name because the sender of
		// this message is the gateway-side helper, not the subscriber.
		m.removeSubscriberByName(msg.SessionName)
		m.maybeShutdown(ctx)

	case *actor.Terminated:
		m.removeSubscriberByPath(msg.ActorPath())
		m.maybeShutdown(ctx)

	case *PlayerInput:
		m.handleInput(msg.Action)

	case *tick:
		if !m.gameOver && !m.paused {
			m.step()
		}
		m.broadcast(ctx)

	default:
		ctx.Unhandled()
	}
}

func (m *MatchActor) reset() {
	for r := range m.grid {
		for c := range m.grid[r] {
			m.grid[r][c] = 0
		}
	}
	m.nextKind = rand.IntN(7)
	m.score = 0
	m.lines = 0
	m.level = 1
	m.gameOver = false
	m.softDrop = false
	m.paused = false
	m.gravity = gravityStart
	m.fallTick = m.gravity
	m.spawnPiece()
}

func (m *MatchActor) spawnPiece() {
	m.piece = PieceState{
		Kind: m.nextKind,
		Rot:  0,
		X:    BoardW / 2,
		Y:    1,
	}
	m.nextKind = rand.IntN(7)
	// If the new piece overlaps an existing block, the stack reached the
	// top — game over.
	if !m.canPlace(m.piece) {
		m.gameOver = true
	}
}

// canPlace returns true if the piece in its given orientation/position fits
// on the board (in bounds and not overlapping locked cells). Cells with
// y < 0 are tolerated so a piece can spawn partially above the board.
func (m *MatchActor) canPlace(p PieceState) bool {
	cells := pieceCells(p.Kind, p.Rot)
	for _, c := range cells {
		x := p.X + c[0]
		y := p.Y + c[1]
		if x < 0 || x >= BoardW || y >= BoardH {
			return false
		}
		if y >= 0 && m.grid[y][x] != 0 {
			return false
		}
	}
	return true
}

func (m *MatchActor) step() {
	m.tickN++
	m.fallTick--
	if m.fallTick <= 0 {
		m.tryFall()
		m.fallTick = m.currentGravity()
	}
}

func (m *MatchActor) currentGravity() int {
	if m.softDrop {
		return softDropTicks
	}
	return m.gravity
}

func (m *MatchActor) tryFall() {
	next := m.piece
	next.Y++
	if m.canPlace(next) {
		m.piece = next
		return
	}
	m.lockPiece()
}

// lockPiece writes the active piece into the grid, clears any full rows,
// scores them, levels up if appropriate, and spawns the next piece.
func (m *MatchActor) lockPiece() {
	cells := pieceCells(m.piece.Kind, m.piece.Rot)
	for _, c := range cells {
		x := m.piece.X + c[0]
		y := m.piece.Y + c[1]
		if x >= 0 && x < BoardW && y >= 0 && y < BoardH {
			m.grid[y][x] = int8(m.piece.Kind + 1)
		}
	}
	cleared := m.clearLines()
	// Classic scoring: 1=100, 2=300, 3=500, 4=800, multiplied by level.
	scoreFor := [...]int{0, 100, 300, 500, 800}
	m.score += scoreFor[cleared] * m.level
	m.lines += cleared
	m.level = 1 + m.lines/10
	m.gravity = gravityStart - (m.level-1)*gravityPerLvl
	if m.gravity < gravityMin {
		m.gravity = gravityMin
	}
	m.spawnPiece()
}

// clearLines collapses any fully-filled rows toward the bottom and returns
// how many were removed.
func (m *MatchActor) clearLines() int {
	cleared := 0
	write := BoardH - 1
	for read := BoardH - 1; read >= 0; read-- {
		full := true
		for c := 0; c < BoardW; c++ {
			if m.grid[read][c] == 0 {
				full = false
				break
			}
		}

		if full {
			cleared++
			continue
		}

		if write != read {
			m.grid[write] = m.grid[read]
		}
		write--
	}
	for write >= 0 {
		for c := 0; c < BoardW; c++ {
			m.grid[write][c] = 0
		}
		write--
	}
	return cleared
}

func (m *MatchActor) handleInput(action string) {
	// Pause is always honored (except after game-over — there's nothing
	// running to pause). All other inputs are gated on !paused.
	if action == ActionPause {
		if !m.gameOver {
			m.paused = !m.paused
		}
		return
	}

	if m.gameOver {
		if action == ActionRestart {
			m.reset()
		}
		return
	}

	if m.paused {
		return
	}

	switch action {
	case ActionLeft:
		m.tryShift(-1)
	case ActionRight:
		m.tryShift(1)
	case ActionRotate:
		if m.piece.Kind == kindO { // O piece has no meaningful rotation
			return
		}

		next := m.piece
		next.Rot = (next.Rot + 1) % 4
		if m.canPlace(next) {
			m.piece = next
		}
	case ActionSoftDrop:
		m.softDrop = true
		if m.fallTick > softDropTicks {
			m.fallTick = softDropTicks
		}
	case ActionSoftDropEnd:
		m.softDrop = false
	case ActionHardDrop:
		for {
			next := m.piece
			next.Y++
			if !m.canPlace(next) {
				break
			}
			m.piece = next
			m.score += hardDropBonus
		}
		m.lockPiece()
		m.fallTick = m.currentGravity()
	}
}

func (m *MatchActor) tryShift(dx int) {
	next := m.piece
	next.X += dx
	if m.canPlace(next) {
		m.piece = next
	}
}

// removeSubscriberByName removes the subscriber whose actor name matches
// `name`. Used by the gateway-driven Unsubscribe — the gateway holds the
// session's name as a string (cluster-stable) and never the PID itself.
func (m *MatchActor) removeSubscriberByName(name string) {
	for i, p := range m.subs {
		if p.Name() == name {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}

// removeSubscriberByPath removes the subscriber whose Path matches `path`.
// Used when *Terminated arrives for a *local* subscriber — GoAkt's Watch
// only fires for same-node deaths, so this code path is only reached for
// session-and-match-co-located cases. Cross-node teardown goes through
// removeSubscriberByName via the gateway-emitted Unsubscribe.
func (m *MatchActor) removeSubscriberByPath(path actor.Path) {
	for i, p := range m.subs {
		if p.Path().Equals(path) {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}

// maybeShutdown stops the match once its owning subscriber has gone away.
// The hadSubscriber guard prevents the actor from killing itself during
// the brief window between PostStart and the owner's first Subscribe.
func (m *MatchActor) maybeShutdown(ctx *actor.ReceiveContext) {
	if m.hadSubscriber && len(m.subs) == 0 {
		ctx.Shutdown()
	}
}

// ghostY returns the row the active piece would land on if hard-dropped
// from its current X/Rot. The client renders an outline at this Y as a
// landing hint. While paused or after game-over, returns the piece's own
// Y (i.e., no visible ghost).
func (m *MatchActor) ghostY() int {
	if m.gameOver || m.paused {
		return m.piece.Y
	}
	p := m.piece
	for {
		next := p
		next.Y++
		if !m.canPlace(next) {
			return p.Y
		}
		p = next
	}
}

func (m *MatchActor) broadcast(ctx *actor.ReceiveContext) {
	snap := &Snapshot{
		Tick:     m.tickN,
		T:        time.Now().UnixMilli(),
		Grid:     m.gridCopy(),
		Piece:    m.piece,
		GhostY:   m.ghostY(),
		Next:     m.nextKind,
		Score:    m.score,
		Lines:    m.lines,
		Level:    m.level,
		GameOver: m.gameOver,
		Paused:   m.paused,
	}
	for _, sub := range m.subs {
		ctx.Tell(sub, snap)
	}
}

func (m *MatchActor) gridCopy() [][]int8 {
	g := make([][]int8, BoardH)
	for r := 0; r < BoardH; r++ {
		g[r] = make([]int8, BoardW)
		copy(g[r], m.grid[r][:])
	}
	return g
}
