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
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-scrabble/scrabble"
)

const (
	schedRefTurn     = "turn."
	schedRefShutdown = "shutdown."
	schedRefBotMove  = "botmove."
)

// roomPlayer is the per-seat state the room tracks.
type roomPlayer struct {
	id          string
	name        string
	sessionName string
	sessionPID  *actor.PID
	score       int
	rack        *scrabble.Rack
	bot         bool
	botPID      *actor.PID
}

// RoomActor owns one Scrabble game. The FSM has three behaviors
// (waiting, playing, gameOver) wired via Become from PostStart.
//
// SpawnOn re-instantiates the actor via the kind registry, so all state
// must derive from the actor name + system extensions. The actor name
// has the form "room.<lang>.<code>".
type RoomActor struct {
	code     string
	language string
	bundle   *LangBundle

	leaderboard *Leaderboard

	players []*roomPlayer
	ownerID string

	bag   *scrabble.Bag
	board *scrabble.Board

	currentIdx     int
	scorelessTurns int
	turnDeadline   time.Time

	// pausedRemaining is non-zero while the room is in pauseBehavior:
	// the turn-timer balance held over for the eventual resume.
	pausedRemaining time.Duration

	schedSuffix string
	activeRefs  map[string]struct{}
	topic       string

	hadPlayer bool
}

var _ actor.Actor = (*RoomActor)(nil)

func (*RoomActor) PreStart(*actor.Context) error { return nil }

func (r *RoomActor) PostStop(ctx *actor.Context) error {
	for ref := range r.activeRefs {
		_ = ctx.ActorSystem().CancelSchedule(ref)
	}

	return nil
}

func (r *RoomActor) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
		// Name format: "room.<lang>.<code>". Parse out both.
		rest := strings.TrimPrefix(ctx.Self().Name(), RoomActorPrefix)
		language, code, ok := strings.Cut(rest, ".")
		if !ok {
			ctx.Logger().Errorf("room: malformed actor name %q", ctx.Self().Name())
			ctx.Shutdown()
			return
		}

		r.language = language
		r.code = strings.ToUpper(code)
		r.activeRefs = make(map[string]struct{})
		r.topic = RoomTopicPrefix + r.code
		r.schedSuffix = "." + ctx.Self().Name()

		registry := registryFromExtension(ctx.ActorSystem())
		if registry == nil {
			ctx.Logger().Errorf("room %s: no registry extension", r.code)
			ctx.Shutdown()
			return
		}

		bundle, err := registry.Get(r.language)
		if err != nil {
			ctx.Logger().Errorf("room %s: %v", r.code, err)
			ctx.Shutdown()
			return
		}

		r.bundle = bundle
		r.leaderboard = leaderboardFromExtension(ctx.ActorSystem())

		ctx.Become(r.waitingBehavior)

	default:
		r.waitingBehavior(ctx)
	}
}

func (r *RoomActor) waitingBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhaseWaiting)
		}

	case *GoodbyePlayer:
		if r.removePlayerByName(msg.SessionName) {
			r.broadcastState(ctx, PhaseWaiting)
		}
		r.maybeShutdown(ctx)

	case *actor.Terminated:
		if _, ok := r.removePlayerByPath(msg.ActorPath()); ok {
			r.broadcastState(ctx, PhaseWaiting)
		}
		r.maybeShutdown(ctx)

	case *PlayerInput:
		switch msg.In.Type {
		case InTypeStart:
			if msg.PlayerID != r.ownerID || len(r.players) < MinPlayers {
				return
			}
			r.startGame(ctx)
		case InTypeAddBot:
			if msg.PlayerID != r.ownerID {
				return
			}
			r.addBot(ctx)
		case InTypeRemoveBot:
			if msg.PlayerID != r.ownerID {
				return
			}
			r.removeBot(ctx, msg.In.Seat)
		case InTypeChat:
			r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
		}

	default:
		ctx.Unhandled()
	}
}

func (r *RoomActor) startGame(ctx *actor.ReceiveContext) {
	r.bag = scrabble.NewBag(r.bundle.Lang, newRoomRNG())
	r.board = scrabble.NewBoard()
	r.currentIdx = 0
	r.scorelessTurns = 0

	for _, player := range r.players {
		player.rack = scrabble.NewRack()
		player.rack.Refill(r.bag)
		player.score = 0
	}

	ctx.Logger().Infof("room %s: starting game with %d players", r.code, len(r.players))

	ctx.Become(r.playingBehavior)
	r.beginTurn(ctx)
}

func (r *RoomActor) playingBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		// Late-joiner: spectator for now (no engine support for mid-game seat).
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhasePlaying)
		}

	case *GoodbyePlayer:
		r.removePlayerByName(msg.SessionName)
		r.maybeShutdown(ctx)

	case *actor.Terminated:
		r.removePlayerByPath(msg.ActorPath())
		r.maybeShutdown(ctx)

	case *PlayerInput:
		r.handlePlayingInput(ctx, msg)

	case *BotPlay:
		r.handleBotPlay(ctx, msg)

	case *turnTimeout:
		// Treat as pass.
		r.applyPass(ctx, r.currentPlayer().id, true)

	default:
		ctx.Unhandled()
	}
}

func (r *RoomActor) handlePlayingInput(ctx *actor.ReceiveContext, msg *PlayerInput) {
	switch msg.In.Type {
	case InTypePlace:
		r.applyPlace(ctx, msg.PlayerID, msg.In.Placements)
	case InTypeExchange:
		r.applyExchange(ctx, msg.PlayerID, msg.In.Indices)
	case InTypePass:
		r.applyPass(ctx, msg.PlayerID, false)
	case InTypePause:
		r.pauseGame(ctx, msg.PlayerID)
	case InTypeChat:
		r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
	}
}

// pauseGame freezes the turn timer and any pending bot move and switches
// into pauseBehavior. Any player at the table may pause.
func (r *RoomActor) pauseGame(ctx *actor.ReceiveContext, playerID string) {
	r.cancelSchedule(ctx, schedRefTurn)
	r.cancelBotMove(ctx)

	r.pausedRemaining = max(time.Until(r.turnDeadline), time.Second)
	r.turnDeadline = time.Time{}

	r.publish(ctx, &ChatEvent{
		From: "⏸",
		Text: fmt.Sprintf("%s paused the game", r.nameFor(playerID)),
	})

	ctx.Become(r.pauseBehavior)
	r.broadcastState(ctx, PhasePaused)
}

func (r *RoomActor) pauseBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *PlayerHello:
		if r.addPlayer(ctx, msg, ctx.Sender()) {
			r.sendJoined(ctx, msg)
			r.broadcastState(ctx, PhasePaused)
		}

	case *GoodbyePlayer:
		r.removePlayerByName(msg.SessionName)
		r.maybeShutdown(ctx)

	case *actor.Terminated:
		r.removePlayerByPath(msg.ActorPath())
		r.maybeShutdown(ctx)

	case *PlayerInput:
		switch msg.In.Type {
		case InTypeResume:
			r.resumeGame(ctx, msg.PlayerID)
		case InTypeChat:
			r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
		}

	case *BotPlay:
		// A bot move scheduled before pause may still arrive; drop it.

	default:
		ctx.Unhandled()
	}
}

// resumeGame restores the turn timer to its frozen remaining duration and
// returns to playingBehavior; if it's a bot's turn, kicks the bot again.
func (r *RoomActor) resumeGame(ctx *actor.ReceiveContext, playerID string) {
	remaining := r.pausedRemaining
	if remaining <= 0 {
		remaining = turnDuration
	}
	r.pausedRemaining = 0
	r.turnDeadline = time.Now().Add(remaining)
	r.schedule(ctx, &turnTimeout{}, remaining, schedRefTurn)

	r.publish(ctx, &ChatEvent{
		From: "▶",
		Text: fmt.Sprintf("%s resumed the game", r.nameFor(playerID)),
	})

	ctx.Become(r.playingBehavior)
	r.broadcastState(ctx, PhasePlaying)

	current := r.currentPlayer()
	if current != nil && current.bot && current.botPID != nil {
		r.scheduleBotMove(ctx, current)
	}
}

func (r *RoomActor) cancelBotMove(ctx *actor.ReceiveContext) {
	current := r.currentPlayer()
	if current == nil || !current.bot {
		return
	}

	ref := schedRefBotMove + r.schedSuffix + "." + current.id
	if _, ok := r.activeRefs[ref]; !ok {
		return
	}
	_ = ctx.ActorSystem().CancelSchedule(ref)
	delete(r.activeRefs, ref)
}

func (r *RoomActor) applyPlace(ctx *actor.ReceiveContext, playerID string, wires []PlacementWire) {
	current := r.currentPlayer()
	if current == nil || current.id != playerID {
		r.tellError(ctx, playerID, "not your turn")
		return
	}

	placements, err := toEnginePlacements(wires, r.bundle.Lang)
	if err != nil {
		r.tellError(ctx, playerID, err.Error())
		return
	}

	required := tilesUsed(placements)
	taken, err := current.rack.Remove(required)
	if err != nil {
		r.tellError(ctx, playerID, "tiles not in your rack")
		return
	}

	move := scrabble.Move{Placements: placements}
	result, err := move.Validate(r.board, r.bundle.Dawg, r.bundle.Lang)
	if err != nil {
		current.rack.Add(taken)
		r.tellError(ctx, playerID, err.Error())
		return
	}

	if err := move.Apply(r.board); err != nil {
		current.rack.Add(taken)
		r.tellError(ctx, playerID, err.Error())
		return
	}

	current.score += result.Score
	current.rack.Refill(r.bag)
	r.scorelessTurns = 0

	r.publish(ctx, &MoveEvent{
		PlayerID:   current.id,
		Name:       current.name,
		Placements: wires,
		Words:      formedWords(result),
		Score:      result.Score,
		NewTotal:   current.score,
		Bingo:      result.Bingo,
	})

	if r.checkGameOver(ctx) {
		return
	}

	r.advanceTurn(ctx)
}

func (r *RoomActor) applyExchange(ctx *actor.ReceiveContext, playerID string, indices []int) {
	current := r.currentPlayer()
	if current == nil || current.id != playerID {
		r.tellError(ctx, playerID, "not your turn")
		return
	}

	if err := current.rack.Exchange(indices, r.bag); err != nil {
		r.tellError(ctx, playerID, err.Error())
		return
	}

	r.scorelessTurns++

	if r.checkGameOver(ctx) {
		return
	}

	r.advanceTurn(ctx)
}

func (r *RoomActor) applyPass(ctx *actor.ReceiveContext, playerID string, timedOut bool) {
	current := r.currentPlayer()
	if current == nil || current.id != playerID {
		if !timedOut {
			r.tellError(ctx, playerID, "not your turn")
		}
		return
	}

	r.scorelessTurns++

	if r.checkGameOver(ctx) {
		return
	}

	r.advanceTurn(ctx)
}

func (r *RoomActor) advanceTurn(ctx *actor.ReceiveContext) {
	r.cancelSchedule(ctx, schedRefTurn)
	r.currentIdx = (r.currentIdx + 1) % len(r.players)
	r.beginTurn(ctx)
}

func (r *RoomActor) beginTurn(ctx *actor.ReceiveContext) {
	r.turnDeadline = time.Now().Add(turnDuration)
	r.schedule(ctx, &turnTimeout{}, turnDuration, schedRefTurn)
	r.broadcastState(ctx, PhasePlaying)

	current := r.currentPlayer()
	if current != nil && current.bot && current.botPID != nil {
		r.scheduleBotMove(ctx, current)
	}
}

func (r *RoomActor) scheduleBotMove(ctx *actor.ReceiveContext, bot *roomPlayer) {
	turn := &YourTurn{
		BotID: bot.id,
		Board: boardToWire(r.board, r.bundle.Lang),
		Rack:  rackToWire(bot.rack, r.bundle.Lang),
	}

	// Brief "thinking" delay so the bot's move doesn't appear instantly.
	ref := schedRefBotMove + r.schedSuffix + "." + bot.id
	delay := time.Duration(BotMoveDelayMs) * time.Millisecond
	if err := ctx.ActorSystem().ScheduleOnce(ctx.Context(), turn, bot.botPID, delay, actor.WithReference(ref)); err != nil {
		ctx.Logger().Errorf("room %s schedule bot move(%s): %v", r.code, bot.id, err)
		return
	}
	r.activeRefs[ref] = struct{}{}
}

func (r *RoomActor) handleBotPlay(ctx *actor.ReceiveContext, play *BotPlay) {
	current := r.currentPlayer()
	if current == nil || current.id != play.BotID {
		return
	}

	if len(play.Placements) == 0 {
		r.applyPass(ctx, play.BotID, false)
		return
	}

	r.applyPlace(ctx, play.BotID, play.Placements)
}

func (r *RoomActor) checkGameOver(ctx *actor.ReceiveContext) bool {
	racks := make([]*scrabble.Rack, len(r.players))

	for i, player := range r.players {
		racks[i] = player.rack
	}

	if !scrabble.IsGameOver(r.bag, racks, r.scorelessTurns) {
		return false
	}

	r.enterGameOver(ctx)

	return true
}

func (r *RoomActor) enterGameOver(ctx *actor.ReceiveContext) {
	r.cancelSchedule(ctx, schedRefTurn)

	scores := make([]int, len(r.players))
	racks := make([]*scrabble.Rack, len(r.players))

	for i, player := range r.players {
		scores[i] = player.score
		racks[i] = player.rack
	}

	final := scrabble.FinalScores(scores, racks, r.bundle.Lang)
	for i, player := range r.players {
		player.score = final[i]
	}

	winnerID, winnerName := r.computeWinner()

	r.publish(ctx, &GameOverEvent{
		WinnerID:   winnerID,
		WinnerName: winnerName,
		Scores:     r.scoreEntries(),
	})

	r.recordResults(ctx, winnerID)
	r.broadcastState(ctx, PhaseGameOver)
	r.schedule(ctx, &shutdownRoom{}, GameOverSecs*time.Second, schedRefShutdown)

	ctx.Become(r.gameOverBehavior)
}

func (r *RoomActor) gameOverBehavior(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case []LeaderboardEntry:
		winnerID, winnerName := r.computeWinner()
		r.publish(ctx, &GameOverEvent{
			WinnerID:    winnerID,
			WinnerName:  winnerName,
			Scores:      r.scoreEntries(),
			Leaderboard: msg,
		})

	case *PlayerHello:
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
		case InTypePlayAgain:
			r.restartGame(ctx, msg.PlayerID)
		case InTypeChat:
			r.publish(ctx, &ChatEvent{From: r.nameFor(msg.PlayerID), Text: msg.In.Text})
		}

	case *shutdownRoom:
		ctx.Shutdown()

	default:
		ctx.Unhandled()
	}
}

func (r *RoomActor) restartGame(ctx *actor.ReceiveContext, byPlayerID string) {
	r.cancelSchedule(ctx, schedRefShutdown)

	r.publish(ctx, &ChatEvent{
		From: "📣",
		Text: fmt.Sprintf("%s started a new game", r.nameFor(byPlayerID)),
	})

	if len(r.players) < MinPlayers {
		r.broadcastState(ctx, PhaseWaiting)
		ctx.Become(r.waitingBehavior)
		return
	}

	r.startGame(ctx)
}

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

	if r.ownerID == "" {
		r.ownerID = msg.PlayerID
	}

	r.hadPlayer = true

	if sender != nil {
		ctx.Watch(sender)
	}

	return true
}

func (r *RoomActor) addBot(ctx *actor.ReceiveContext) {
	if len(r.players) >= MaxPlayers {
		return
	}

	botID := "bot-" + shortID()
	botName := "Bot " + strings.ToUpper(botID[len(botID)-3:])

	pid := ctx.Spawn(BotActorPrefix+r.language+"."+r.code+"."+botID, new(BotActor),
		actor.WithLongLived())
	if pid == nil {
		ctx.Logger().Errorf("room %s: spawn bot failed", r.code)
		return
	}

	r.players = append(r.players, &roomPlayer{
		id:     botID,
		name:   botName,
		bot:    true,
		botPID: pid,
	})

	r.broadcastState(ctx, PhaseWaiting)
}

func (r *RoomActor) removeBot(ctx *actor.ReceiveContext, seat int) {
	if seat < 0 || seat >= len(r.players) {
		return
	}

	target := r.players[seat]
	if !target.bot {
		return
	}

	if target.botPID != nil {
		_ = actor.Tell(context.Background(), target.botPID, &shutdownBot{})
	}

	r.players = append(r.players[:seat], r.players[seat+1:]...)

	r.broadcastState(ctx, PhaseWaiting)
}

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

func (r *RoomActor) currentPlayer() *roomPlayer {
	if r.currentIdx < 0 || r.currentIdx >= len(r.players) {
		return nil
	}

	return r.players[r.currentIdx]
}

func (r *RoomActor) playerByID(id string) *roomPlayer {
	for _, player := range r.players {
		if player.id == id {
			return player
		}
	}

	return nil
}

func (r *RoomActor) nameFor(id string) string {
	if player := r.playerByID(id); player != nil {
		return player.name
	}

	return id
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

func (r *RoomActor) scoreEntries() []ScoreEntry {
	out := make([]ScoreEntry, 0, len(r.players))

	for _, player := range r.players {
		out = append(out, ScoreEntry{PlayerID: player.id, Name: player.name, Score: player.score})
	}

	return out
}

func (r *RoomActor) playerViews() []PlayerView {
	out := make([]PlayerView, 0, len(r.players))

	for _, player := range r.players {
		rackSize := 0
		if player.rack != nil {
			rackSize = player.rack.Size()
		}
		out = append(out, PlayerView{
			ID:       player.id,
			Name:     player.name,
			Score:    player.score,
			RackSize: rackSize,
			Bot:      player.bot,
		})
	}

	return out
}

func (r *RoomActor) broadcastState(ctx *actor.ReceiveContext, phase string) {
	var (
		board   [][]string
		bagLeft int
		perRack map[string][]string
	)

	if r.board != nil {
		board = boardToWire(r.board, r.bundle.Lang)
	} else {
		board = emptyBoardWire()
	}

	if r.bag != nil {
		bagLeft = r.bag.Remaining()
	}

	currentID := ""
	if current := r.currentPlayer(); current != nil && phase == PhasePlaying {
		currentID = current.id
	}

	timerMs := 0
	if phase == PhasePlaying && !r.turnDeadline.IsZero() {
		remaining := time.Until(r.turnDeadline)
		if remaining > 0 {
			timerMs = int(remaining / time.Millisecond)
		}
	}

	perRack = make(map[string][]string, len(r.players))
	for _, player := range r.players {
		if player.rack != nil {
			perRack[player.id] = rackToWire(player.rack, r.bundle.Lang)
		}
	}

	r.publish(ctx, &StateEvent{
		Phase:        phase,
		Board:        board,
		Players:      r.playerViews(),
		CurrentID:    currentID,
		OwnerID:      r.ownerID,
		BagRemaining: bagLeft,
		TimerMs:      timerMs,
		PerRack:      perRack,
	})
}

func (r *RoomActor) publish(ctx *actor.ReceiveContext, evt any) {
	topic := ctx.ActorSystem().TopicActor()
	if topic == nil {
		ctx.Logger().Errorf("room %s: pub/sub disabled — drop event %T", r.code, evt)
		return
	}
	ctx.Tell(topic, actor.NewPublish(uuid.NewString(), r.topic, evt))
}

func (r *RoomActor) tellError(ctx *actor.ReceiveContext, playerID, message string) {
	player := r.playerByID(playerID)
	if player == nil || player.sessionPID == nil {
		return
	}
	ctx.Tell(player.sessionPID, &ErrorEvent{For: playerID, Message: message})
}

func (r *RoomActor) sendJoined(ctx *actor.ReceiveContext, msg *PlayerHello) {
	sender := ctx.Sender()
	if sender == nil {
		return
	}

	ctx.Tell(sender, &JoinedEvent{
		For:      msg.PlayerID,
		Room:     r.code,
		Language: r.language,
		PlayerID: msg.PlayerID,
		Owner:    msg.PlayerID == r.ownerID,
		Profile:  ProfileView{PlayerID: msg.PlayerID, Name: msg.Name},
	})
}

func (r *RoomActor) schedule(ctx *actor.ReceiveContext, msg any, delay time.Duration, refPrefix string) {
	ref := refPrefix + r.schedSuffix
	if err := ctx.ActorSystem().ScheduleOnce(ctx.Context(), msg, ctx.Self(), delay, actor.WithReference(ref)); err != nil {
		ctx.Logger().Errorf("room %s schedule(%s): %v", r.code, refPrefix, err)
		return
	}
	r.activeRefs[ref] = struct{}{}
}

func (r *RoomActor) cancelSchedule(ctx *actor.ReceiveContext, refPrefix string) {
	ref := refPrefix + r.schedSuffix
	if _, ok := r.activeRefs[ref]; !ok {
		return
	}
	_ = ctx.ActorSystem().CancelSchedule(ref)
	delete(r.activeRefs, ref)
}

func (r *RoomActor) maybeShutdown(ctx *actor.ReceiveContext) {
	humans := 0
	for _, player := range r.players {
		if !player.bot {
			humans++
		}
	}

	if r.hadPlayer && humans == 0 {
		ctx.Logger().Infof("room %s empty — shutting down", r.code)
		ctx.Shutdown()
	}
}

func (r *RoomActor) recordResults(ctx *actor.ReceiveContext, winnerID string) {
	system := ctx.ActorSystem()
	store := profileStoreFromExtension(system)
	leaderboard := r.leaderboard
	language := r.language

	for _, player := range r.players {
		if player.bot {
			continue
		}
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

			return nil, system.TellGrain(bg, ident, &RecordGame{WonThisGame: won, GameScore: score})
		})
	}

	if winnerID == "" || leaderboard == nil {
		return
	}

	winnerName := ""
	if player := r.playerByID(winnerID); player != nil {
		winnerName = player.name
	}

	ctx.PipeTo(ctx.Self(), func() (any, error) {
		bg := context.Background()

		if err := leaderboard.RecordWin(bg, language, winnerID, winnerName); err != nil {
			return nil, err
		}

		return leaderboard.Top(bg, language, 10)
	})
}

func shortID() string {
	return uuid.NewString()[:6]
}

// newRoomRNG seeds a fresh PCG from the wall clock. Each room gets its
// own deterministic-from-this-seed stream; collisions between
// simultaneously-starting rooms are harmless (different bags either way).
func newRoomRNG() *rand.Rand {
	now := uint64(time.Now().UnixNano())
	return rand.New(rand.NewPCG(now, now>>32))
}
