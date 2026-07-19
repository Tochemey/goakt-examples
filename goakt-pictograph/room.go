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
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"
)

const (
	// schedRefCountdown is the per-room schedule reference used by the
	// recurring 1-Hz countdown in the drawing phase. Combined with the
	// room actor name to stay globally unique (schedule refs collide
	// system-wide otherwise — same constraint as goakt-tetris).
	schedRefCountdown = "countdown."
	schedRefChoose    = "choose."
	schedRefRoundEnd  = "roundend."
	schedRefShutdown  = "shutdown."
	schedRefStartGame = "startgame."

	// strokeBufferMax bounds the per-round stroke history kept for
	// late-joiners. Each stroke is small (~tens of bytes) but a fast
	// drawer can emit thousands per round; clip so memory doesn't grow
	// without bound for very long matches.
	strokeBufferMax = 2000

	// joinGatherWindow gives a forming room a few seconds for more
	// players to arrive before round 1 starts. Resets every time a new
	// player joins (so 8 players trickling in over 30s all play together).
	joinGatherWindow = 5 * time.Second
)

// roomPlayer is the per-player state the room tracks. Score persists
// across rounds; resets when the room shuts down.
type roomPlayer struct {
	id          string
	name        string
	sessionName string // cluster-stable name; matches PlayerSessionActor.Self().Name()
	sessionPID  *actor.PID
	score       int
	hasDrawn    bool // set true on becoming drawer; reset when all have drawn
}

// RoomActor owns one match. The FSM is implemented as four named
// Behaviors switched via Become; the constructor's PostStart picks the
// initial one (waiting). The actor *never* runs the default Receive
// after PostStart — every message is routed through whichever behavior
// is current.
//
// SpawnOn re-instantiates the actor via the kind registry (`new(T)`)
// when placing it onto a cluster node, so all state — including
// dependencies — must be derivable from the actor name + system
// extensions. No constructor-injected deps allowed.
type RoomActor struct {
	leaderboard *Leaderboard // resolved in PostStart from system extensions

	code      string
	maxRounds int

	players []*roomPlayer

	// round state
	round       int
	drawerID    string
	word        string
	wordChoices []string
	timeLeft    int
	guessed     map[string]bool // playerIDs that already guessed correctly

	// per-round stroke buffer for state replay to late-joiners
	strokes []*StrokeEvent

	// schedule reference suffix (per-actor unique) — see schedRef*().
	schedSuffix string
	activeRefs  map[string]struct{} // refs we've actually scheduled, for cleanup

	topic string

	// hadPlayer prevents the actor from shutting itself down between
	// PostStart and the first PlayerHello (same guard pattern as
	// MatchActor in goakt-tetris).
	hadPlayer bool
}

var _ actor.Actor = (*RoomActor)(nil)

func (*RoomActor) PreStart(*actor.Context) error { return nil }

// PostStop cancels every schedule the room ever set so no further
// messages are queued to a dead actor (which would land in the dead-
// letter queue).
func (r *RoomActor) PostStop(ctx *actor.Context) error {
	for ref := range r.activeRefs {
		_ = ctx.ActorSystem().CancelSchedule(ref)
	}
	return nil
}

// Receive is the bootstrap entry point. It runs only once — for
// PostStart — and then hands off to the behavior stack.
func (r *RoomActor) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
		// Derive code from the cluster-stable actor name
		// (lobby spawns rooms as "room.<lowercase code>"); upper-case
		// it back for the public-facing UI.
		r.code = strings.ToUpper(strings.TrimPrefix(ctx.Self().Name(), RoomActorPrefix))
		r.maxRounds = DefaultRounds
		r.guessed = make(map[string]bool)
		r.activeRefs = make(map[string]struct{})
		r.topic = RoomTopicPrefix + r.code
		r.schedSuffix = "." + ctx.Self().Name()
		r.leaderboard = leaderboardFromExtension(ctx.ActorSystem())
		if r.leaderboard == nil {
			ctx.Logger().Errorf("room %s: leaderboard extension not registered — leaderboard disabled", r.code)
		}
		ctx.Become(r.waitingBehavior)
	default:
		// Defensive: route any straggler to waiting.
		r.waitingBehavior(ctx)
	}
}

func (r *RoomActor) schedRef(prefix string) string { return prefix + r.schedSuffix }

func (r *RoomActor) schedule(ctx *actor.ReceiveContext, msg any, delay time.Duration, refPrefix string) {
	ref := r.schedRef(refPrefix)
	if err := ctx.ActorSystem().ScheduleOnce(ctx.Context(), msg, ctx.Self(), delay, actor.WithReference(ref)); err != nil {
		ctx.Logger().Errorf("room %s schedule(%s): %v", r.code, refPrefix, err)
		return
	}
	r.activeRefs[ref] = struct{}{}
}

func (r *RoomActor) scheduleRecurring(ctx *actor.ReceiveContext, msg any, interval time.Duration, refPrefix string) {
	ref := r.schedRef(refPrefix)
	if err := ctx.ActorSystem().Schedule(ctx.Context(), msg, ctx.Self(), interval, actor.WithReference(ref)); err != nil {
		ctx.Logger().Errorf("room %s schedule recurring(%s): %v", r.code, refPrefix, err)
		return
	}
	r.activeRefs[ref] = struct{}{}
}

func (r *RoomActor) cancelSchedule(ctx *actor.ReceiveContext, refPrefix string) {
	ref := r.schedRef(refPrefix)
	if _, ok := r.activeRefs[ref]; !ok {
		return
	}
	_ = ctx.ActorSystem().CancelSchedule(ref)
	delete(r.activeRefs, ref)
}

// publish sends an event to the room's pub/sub topic. The event's For
// field is what sessions consult to filter — empty = broadcast, else
// only the named player's session forwards it.
func (r *RoomActor) publish(ctx *actor.ReceiveContext, evt any) {
	topic := ctx.ActorSystem().TopicActor()
	if topic == nil {
		ctx.Logger().Errorf("room %s: pub/sub disabled — drop event %T", r.code, evt)
		return
	}
	ctx.Tell(topic, actor.NewPublish(uuid.NewString(), r.topic, evt))
}

// broadcastState publishes a phase-appropriate StateEvent. wordMask is
// derived from r.word during drawing and zeroed otherwise. The drawer-
// only sub-state ("you are drawer") is left to the session: it knows
// the recipient's player id and toggles the flag accordingly.
func (r *RoomActor) broadcastState(ctx *actor.ReceiveContext, phase string) {
	r.publish(ctx, &StateEvent{
		Phase:     phase,
		Round:     r.round,
		MaxRounds: r.maxRounds,
		TimeLeft:  r.timeLeft,
		Players:   r.playerViews(),
		DrawerID:  r.drawerID,
		WordMask:  r.currentMask(),
	})
}

func (r *RoomActor) currentMask() string {
	if r.word == "" {
		return ""
	}
	return maskWord(r.word)
}

func (r *RoomActor) playerViews() []PlayerView {
	out := make([]PlayerView, 0, len(r.players))
	for _, player := range r.players {
		out = append(out, PlayerView{ID: player.id, Name: player.name, Score: player.score})
	}
	return out
}

func (r *RoomActor) playerByID(id string) *roomPlayer {
	for _, player := range r.players {
		if player.id == id {
			return player
		}
	}
	return nil
}

// addPlayer records a player joining or reconnecting. Returns true if
// the room state changed (and a broadcastState should follow).
//
// Reconnect semantics: when a player with the same PlayerID is already
// present, we update the existing record's session identity in place
// instead of rejecting. This lets a returning player (URL ?id=...)
// pick up where they left off without the old GoodbyePlayer evicting
// them — its SessionName no longer matches anything.
func (r *RoomActor) addPlayer(ctx *actor.ReceiveContext, msg *PlayerHello, sender *actor.PID) bool {
	r.leaderboard.RememberName(msg.PlayerID, msg.Name)

	if existing := r.playerByID(msg.PlayerID); existing != nil {
		existing.sessionName = msg.SessionName
		existing.sessionPID = sender
		existing.name = msg.Name
		if sender != nil {
			ctx.Watch(sender)
		}
		return true
	}

	if len(r.players) >= MaxPlayers {
		ctx.Tell(sender, &ErrorEvent{For: msg.PlayerID, Message: "room is full"})
		return false
	}

	r.players = append(r.players, &roomPlayer{
		id:          msg.PlayerID,
		name:        msg.Name,
		sessionName: msg.SessionName,
		sessionPID:  sender,
	})
	r.hadPlayer = true

	// Watch is cluster-aware in GoAkt v4 (RemoteWatch handles cross-node
	// peers), so we no longer need an IsLocal() guard. The gateway still
	// emits GoodbyePlayer on a clean WS close — that's the fast path;
	// Terminated is the safety net for ungraceful gateway crashes.
	if sender != nil {
		ctx.Watch(sender)
	}
	return true
}

// removePlayerByName removes by cluster-stable session name. Returns
// true if a player was actually removed.
func (r *RoomActor) removePlayerByName(name string) bool {
	for i, player := range r.players {
		if player.sessionName == name {
			r.players = append(r.players[:i], r.players[i+1:]...)
			return true
		}
	}
	return false
}

func (r *RoomActor) removePlayerByPath(path actor.Path) (*roomPlayer, bool) {
	for i, player := range r.players {
		if player.sessionPID != nil && player.sessionPID.Path().Equals(path) {
			removed := player
			r.players = append(r.players[:i], r.players[i+1:]...)
			return removed, true
		}
	}
	return nil, false
}

// pickNextDrawer rotates the drawer fairly. We mark each player as
// hasDrawn when they become drawer; once every player has drawn we
// reset the flags and start again. New joiners default to hasDrawn=false
// so they'll be picked sooner than already-drawn players.
func (r *RoomActor) pickNextDrawer() string {
	if len(r.players) == 0 {
		return ""
	}

	for _, player := range r.players {
		if !player.hasDrawn {
			player.hasDrawn = true
			return player.id
		}
	}

	for _, player := range r.players {
		player.hasDrawn = false
	}

	r.players[0].hasDrawn = true
	return r.players[0].id
}

func (r *RoomActor) waitingBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		if !r.addPlayer(ctx, msg, ctx.Sender()) {
			return
		}

		r.sendJoined(ctx, msg)
		r.broadcastState(ctx, PhaseWaiting)
		if len(r.players) >= MinPlayers {
			// Reset the gather countdown every time someone new joins
			// so a slowly-coalescing group still ends up playing together.
			r.cancelSchedule(ctx, schedRefStartGame)
			r.schedule(ctx, &nextRound{}, joinGatherWindow, schedRefStartGame)
		}

	case *GoodbyePlayer:
		if r.removePlayerByName(msg.SessionName) {
			r.broadcastState(ctx, PhaseWaiting)
		}

		r.cancelGatherIfBelowMin(ctx)
		r.maybeShutdown(ctx)

	case *actor.Terminated:
		if removed, ok := r.removePlayerByPath(msg.ActorPath()); ok {
			ctx.Logger().Infof("room %s: %s disconnected (waiting)", r.code, removed.name)
			r.broadcastState(ctx, PhaseWaiting)
		}

		r.cancelGatherIfBelowMin(ctx)
		r.maybeShutdown(ctx)

	case *PlayerInput:
		// In waiting phase only chat is meaningful; treat any
		// chat-shaped input as such, ignore the rest.
		if msg.In.Type == InTypeGuess {
			r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
		}

	case *nextRound:
		// Gather window elapsed — start the first round.
		r.round = 0 // enterChoosing increments to 1
		r.enterChoosing(ctx)

	default:
		ctx.Unhandled()
	}
}

// sendJoined Tells the joining session its JoinedEvent directly. We
// bypass the pub/sub topic here for two reasons:
//
//  1. The session subscribes to the room topic asynchronously from its
//     own PostStart; the topic actor may not have processed that
//     Subscribe by the time we'd Publish, which would drop the message
//     to that session entirely.
//
//  2. The JoinedEvent is targeted (only this player needs it), so
//     direct Tell is also cheaper than topic fan-out.
//
// The leaderboard field is intentionally left empty: fetching it would
// require a blocking Ask to the CRDT replicator from inside Receive,
// which would freeze the room's mailbox. The leaderboard is delivered
// fresh on every GameOverEvent instead.
func (r *RoomActor) sendJoined(ctx *actor.ReceiveContext, msg *PlayerHello) {
	sender := ctx.Sender()
	if sender == nil {
		return
	}

	ctx.Tell(sender, &JoinedEvent{
		For:      msg.PlayerID,
		Room:     r.code,
		PlayerID: msg.PlayerID,
		Profile:  ProfileView{PlayerID: msg.PlayerID, Name: msg.Name},
	})
}

func (r *RoomActor) enterChoosing(ctx *actor.ReceiveContext) {
	r.cancelSchedule(ctx, schedRefStartGame)
	r.round++
	if r.round > r.maxRounds {
		r.enterGameOver(ctx)
		return
	}

	r.drawerID = r.pickNextDrawer()
	r.wordChoices = pickWords(WordChoices)
	r.word = ""
	r.timeLeft = ChooseSeconds
	r.guessed = make(map[string]bool)
	r.strokes = r.strokes[:0]

	// StateEvent before WordChoicesEvent: the client clears any stale
	// overlay (e.g. the gameOver winner reveal after a restart) on
	// phase change, so the wordChoices overlay needs to land second
	// to survive that clear.
	r.broadcastState(ctx, PhaseChoosing)
	r.publish(ctx, &WordChoicesEvent{For: r.drawerID, Choices: r.wordChoices})
	r.schedule(ctx, &autoPickFirst{}, ChooseSeconds*time.Second, schedRefChoose)

	ctx.Become(r.choosingBehavior)
}

func (r *RoomActor) choosingBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		// Late-joiner during choosing: add and bring them up to date.
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhaseChoosing)
		}

	case *GoodbyePlayer:
		if r.removePlayerByName(msg.SessionName) {
			r.broadcastState(ctx, PhaseChoosing)
		}
		r.maybeAbortRound(ctx)

	case *actor.Terminated:
		if _, ok := r.removePlayerByPath(msg.ActorPath()); ok {
			r.broadcastState(ctx, PhaseChoosing)
		}
		r.maybeAbortRound(ctx)

	case *PlayerInput:
		switch msg.In.Type {
		case InTypePickWord:
			if msg.PlayerID != r.drawerID || !r.isValidChoice(msg.In.Word) {
				return
			}
			r.word = msg.In.Word
			r.cancelSchedule(ctx, schedRefChoose)
			r.enterDrawing(ctx)
		case InTypeGuess:
			// Guesses during choosing belong to the next phase — buffer
			// them. UnstashAll on drawing entry replays in arrival order.
			ctx.Stash()
		default:
			// strokes / clears in this phase are nonsense
		}

	case *autoPickFirst:
		if len(r.wordChoices) > 0 {
			r.word = r.wordChoices[0]
		}
		r.enterDrawing(ctx)

	default:
		ctx.Unhandled()
	}
}

func (r *RoomActor) isValidChoice(word string) bool {
	return slices.Contains(r.wordChoices, word)
}

func (r *RoomActor) enterDrawing(ctx *actor.ReceiveContext) {
	r.timeLeft = DrawSeconds
	// Send the secret to the drawer only; others see only the mask.
	r.publish(ctx, &SecretWordEvent{For: r.drawerID, Word: r.word})
	r.broadcastState(ctx, PhaseDrawing)
	r.scheduleRecurring(ctx, &tickCountdown{}, time.Second, schedRefCountdown)

	ctx.Become(r.drawingBehavior)
	// Drain any guesses stashed during choosing.
	ctx.UnstashAll()
}

func (r *RoomActor) drawingBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhaseDrawing)
			// Replay strokes so the late joiner sees the in-progress drawing.
			for _, stroke := range r.strokes {
				ctx.Tell(ctx.Sender(), &StrokeEvent{
					For:    msg.PlayerID,
					Points: stroke.Points,
					Color:  stroke.Color,
					Width:  stroke.Width,
				})
			}
		}

	case *GoodbyePlayer:
		if r.removePlayerByName(msg.SessionName) {
			r.broadcastState(ctx, PhaseDrawing)
		}
		r.maybeAbortRound(ctx)

	case *actor.Terminated:
		if _, ok := r.removePlayerByPath(msg.ActorPath()); ok {
			r.broadcastState(ctx, PhaseDrawing)
		}
		r.maybeAbortRound(ctx)

	case *PlayerInput:
		switch msg.In.Type {
		case InTypeStroke:
			if msg.PlayerID != r.drawerID {
				return
			}
			evt := &StrokeEvent{Points: msg.In.Points, Color: msg.In.Color, Width: msg.In.Width}
			if len(r.strokes) < strokeBufferMax {
				r.strokes = append(r.strokes, evt)
			}
			r.publish(ctx, evt)
		case InTypeClear:
			if msg.PlayerID != r.drawerID {
				return
			}
			r.strokes = r.strokes[:0]
			r.publish(ctx, &ClearEvent{})
		case InTypeGuess:
			r.handleGuess(ctx, msg)
		}

	case *tickCountdown:
		r.timeLeft--
		if r.timeLeft <= 0 {
			r.enterRoundOver(ctx)
			return
		}
		// Light-weight tick: state event is small (≤ a few hundred bytes
		// even with 8 players); 1 Hz is fine for the countdown display.
		r.broadcastState(ctx, PhaseDrawing)

	default:
		ctx.Unhandled()
	}
}

func (r *RoomActor) handleGuess(ctx *actor.ReceiveContext, msg *PlayerInput) {
	player := r.playerByID(msg.PlayerID)
	if player == nil {
		return
	}

	if msg.PlayerID == r.drawerID {
		return // drawer can't guess
	}

	if r.guessed[msg.PlayerID] {
		return // already won this round
	}

	guess := strings.ToLower(strings.TrimSpace(msg.In.Text))
	if guess == "" {
		return
	}

	if guess != strings.ToLower(r.word) {
		// Wrong guess goes out as ordinary chat so others can see it.
		r.publish(ctx, &ChatEvent{From: player.name, Text: msg.In.Text})
		return
	}

	r.guessed[msg.PlayerID] = true
	delta := r.scoreForCorrectGuess()
	player.score += delta
	r.publish(ctx, &GuessedEvent{PlayerID: player.id, Name: player.name})
	r.publish(ctx, &ScoreEvent{PlayerID: player.id, Delta: delta, Total: player.score})

	// Drawer gets a bonus per correct guess — motivates good drawing.
	if drawer := r.playerByID(r.drawerID); drawer != nil {
		drawer.score += DrawerBonus
		r.publish(ctx, &ScoreEvent{PlayerID: drawer.id, Delta: DrawerBonus, Total: drawer.score})
	}

	// If every non-drawer has guessed, end the round early.
	if r.allNonDrawersGuessed() {
		r.enterRoundOver(ctx)
	}
}

func (r *RoomActor) scoreForCorrectGuess() int {
	// Linear decay from BaseScore at t=Draw down to MinGuessScore at t=0.
	pct := float64(r.timeLeft) / float64(DrawSeconds)
	score := int(float64(BaseScore)*pct + float64(MinGuessScore)*(1-pct))
	return max(score, MinGuessScore)
}

func (r *RoomActor) allNonDrawersGuessed() bool {
	for _, player := range r.players {
		if player.id == r.drawerID {
			continue
		}

		if !r.guessed[player.id] {
			return false
		}
	}
	return true
}

func (r *RoomActor) enterRoundOver(ctx *actor.ReceiveContext) {
	// Cancel every timer the prior phases may have armed. Abort paths
	// (drawer disconnect during choosing) jump straight here without
	// going through the normal cancel sites — leaving the choose-window
	// auto-pick timer alive would let it fire in roundOver (dead-letter)
	// or, worse, leak into the NEXT round's choosing and prematurely
	// auto-pick the new drawer's word.
	r.cancelSchedule(ctx, schedRefCountdown)
	r.cancelSchedule(ctx, schedRefChoose)

	r.publish(ctx, &RoundOverEvent{Word: r.word, Scores: r.scoreEntries()})
	r.broadcastState(ctx, PhaseRoundOver)
	r.schedule(ctx, &nextRound{}, RoundOverSecs*time.Second, schedRefRoundEnd)
	ctx.Become(r.roundOverBehavior)
}

func (r *RoomActor) roundOverBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhaseRoundOver)
		}

	case *GoodbyePlayer:
		if r.removePlayerByName(msg.SessionName) {
			r.broadcastState(ctx, PhaseRoundOver)
		}
		r.maybeShutdown(ctx)

	case *actor.Terminated:
		if _, ok := r.removePlayerByPath(msg.ActorPath()); ok {
			r.broadcastState(ctx, PhaseRoundOver)
		}
		r.maybeShutdown(ctx)

	case *PlayerInput:
		if msg.In.Type == InTypeGuess {
			r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
		}

	case *nextRound:
		// If players left below the threshold during the pause, bail to game-over.
		if len(r.players) < MinPlayers {
			r.enterGameOver(ctx)
			return
		}
		r.enterChoosing(ctx)

	default:
		ctx.Unhandled()
	}
}

func (r *RoomActor) enterGameOver(ctx *actor.ReceiveContext) {
	r.cancelSchedule(ctx, schedRefCountdown)
	r.cancelSchedule(ctx, schedRefRoundEnd)

	winnerID, winnerName := r.computeWinner()

	// Provisional GameOverEvent — leaderboard may be stale by a few ms
	// while the win is being persisted across the cluster. A follow-up
	// event will arrive once the CRDT write + read round-trip completes.
	r.publish(ctx, &GameOverEvent{
		WinnerID:   winnerID,
		WinnerName: winnerName,
		Scores:     r.scoreEntries(),
	})

	// Off-actor work: increment CRDT counters + roll into profile
	// grains + fetch the fresh leaderboard. PipeTo runs the closure on
	// a separate goroutine and delivers the result to the actor's
	// mailbox so we don't block the room.
	r.recordResults(ctx, winnerID)

	r.broadcastState(ctx, PhaseGameOver)
	r.schedule(ctx, &shutdownRoom{}, GameOverSecs*time.Second, schedRefShutdown)
	ctx.Become(r.gameOverBehavior)
}

func (r *RoomActor) gameOverBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case []LeaderboardEntry:
		// Fresh leaderboard arrived from the CRDT read — broadcast the
		// final game-over with the updated standings.
		winnerID, winnerName := r.computeWinner()
		r.publish(ctx, &GameOverEvent{
			WinnerID:    winnerID,
			WinnerName:  winnerName,
			Scores:      r.scoreEntries(),
			Leaderboard: msg,
		})

	case *PlayerHello:
		// Allow late spectators to subscribe to the after-game lobby.
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhaseGameOver)
		}

	case *GoodbyePlayer:
		r.removePlayerByName(msg.SessionName)

	case *actor.Terminated:
		r.removePlayerByPath(msg.ActorPath())

	case *PlayerInput:
		switch msg.In.Type {
		case InTypeRestart:
			r.restartGame(ctx, msg.PlayerID)
		case InTypeGuess:
			r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
		}

	case *shutdownRoom:
		ctx.Shutdown()

	default:
		ctx.Unhandled()
	}
}

// restartGame resets per-game state and jumps straight into the next
// game without tearing down the room. Triggered by any player clicking
// "Play Again" during gameOver. The pending shutdownRoom schedule is
// cancelled so the room survives, scores and drawer rotation are
// wiped, and we either enter the next round directly (if 2+ players
// are still present) or fall back to waiting (otherwise).
//
// First click wins: subsequent restarts arriving while the room is
// already in choosing/drawing/etc. fall to the inner switch's no-op
// default in those behaviors and are silently dropped.
func (r *RoomActor) restartGame(ctx *actor.ReceiveContext, byPlayerID string) {
	r.cancelSchedule(ctx, schedRefShutdown)
	r.cancelSchedule(ctx, schedRefStartGame)
	r.cancelSchedule(ctx, schedRefChoose)
	r.cancelSchedule(ctx, schedRefCountdown)
	r.cancelSchedule(ctx, schedRefRoundEnd)

	for _, player := range r.players {
		player.score = 0
		player.hasDrawn = false
	}
	r.round = 0
	r.drawerID = ""
	r.word = ""
	r.wordChoices = nil
	r.timeLeft = 0
	r.guessed = make(map[string]bool)
	r.strokes = r.strokes[:0]

	r.publish(ctx, &ChatEvent{
		From: "📣",
		Text: fmt.Sprintf("%s started a new game", r.nameFor(byPlayerID)),
	})

	if len(r.players) < MinPlayers {
		r.broadcastState(ctx, PhaseWaiting)
		ctx.Become(r.waitingBehavior)
		return
	}

	r.enterChoosing(ctx)
}

func (r *RoomActor) computeWinner() (string, string) {
	var winnerID, winnerName string
	best := -1
	for _, player := range r.players {
		if player.score > best {
			best = player.score
			winnerID = player.id
			winnerName = player.name
		}
	}
	return winnerID, winnerName
}

// recordResults fires off the side-effecting persistence work for game
// end. Each call is wrapped in a PipeTo so the room actor stays
// responsive. Results that need to come back to the room (the fresh
// leaderboard) are returned from the closure and delivered as a
// mailbox message — see gameOverBehavior's []LeaderboardEntry handler.
//
// Note: ReceiveContext.Context() must NOT be called from inside a
// PipeTo closure — the ReceiveContext is pooled and reset() nils its
// internal context as soon as Receive returns. We use context.Background
// for the off-actor work because it's fire-and-forget anyway; the
// actor lifecycle (shutdownRoom schedule) provides the timeout instead.
func (r *RoomActor) recordResults(ctx *actor.ReceiveContext, winnerID string) {
	system := ctx.ActorSystem()
	store := profileStoreFromExtension(system)
	leaderboard := r.leaderboard

	for _, player := range r.players {
		pid, name, score := player.id, player.name, player.score
		won := pid == winnerID

		ctx.PipeTo(ctx.Self(), func() (any, error) {
			bg := context.Background()

			ident, err := system.GrainIdentity(bg,
				GrainPrefix+pid,
				func(_ context.Context) (actor.Grain, error) {
					return &PlayerProfileGrain{store: store}, nil
				})
			if err != nil {
				return nil, err
			}

			if err := system.TellGrain(bg, ident, &SetName{Name: name}); err != nil {
				return nil, err
			}

			return nil, system.TellGrain(bg, ident,
				&RecordGame{WonThisGame: won, GameScore: score})
		})
	}

	if winnerID == "" {
		return
	}

	winnerName := ""
	if player := r.playerByID(winnerID); player != nil {
		winnerName = player.name
	}

	ctx.PipeTo(ctx.Self(), func() (any, error) {
		bg := context.Background()

		if err := leaderboard.RecordWin(bg, winnerID, winnerName); err != nil {
			return nil, err
		}

		return leaderboard.Top(bg, 10)
	})
}

func (r *RoomActor) scoreEntries() []ScoreEntry {
	out := make([]ScoreEntry, 0, len(r.players))
	for _, player := range r.players {
		out = append(out, ScoreEntry{PlayerID: player.id, Name: player.name, Score: player.score})
	}
	return out
}

func (r *RoomActor) nameFor(id string) string {
	if player := r.playerByID(id); player != nil {
		return player.name
	}
	return id
}

// maybeAbortRound — if the drawer left mid-round or we dropped below
// the player minimum, bail out gracefully into round-over (which will
// then advance to the next round or game-over).
func (r *RoomActor) maybeAbortRound(ctx *actor.ReceiveContext) {
	if len(r.players) < MinPlayers {
		r.enterRoundOver(ctx)
		return
	}

	if r.playerByID(r.drawerID) == nil {
		r.enterRoundOver(ctx)
		return
	}
}

// maybeShutdown stops the room once its last player goes away. The
// hadPlayer guard prevents the actor from killing itself in the
// window between PostStart and the first PlayerHello.
func (r *RoomActor) maybeShutdown(ctx *actor.ReceiveContext) {
	if r.hadPlayer && len(r.players) == 0 {
		ctx.Logger().Infof("room %s empty — shutting down", r.code)
		ctx.Shutdown()
	}
}

// cancelGatherIfBelowMin cancels the gather-window timer when the
// player count drops below the minimum during the waiting phase. Both
// the GoodbyePlayer (graceful WS close) and *actor.Terminated
// (ungraceful session death) branches need this; without it, a stale
// timer can fire enterChoosing on a sub-minimum room.
func (r *RoomActor) cancelGatherIfBelowMin(ctx *actor.ReceiveContext) {
	if len(r.players) < MinPlayers {
		r.cancelSchedule(ctx, schedRefStartGame)
	}
}
