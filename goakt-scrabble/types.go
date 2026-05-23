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

import "time"

const (
	MinPlayers     = 2
	MaxPlayers     = 4
	TurnSeconds    = 90
	GameOverSecs   = 30
	BotMoveDelayMs = 700

	turnDuration = TurnSeconds * time.Second

	LobbyActorName       = "lobby"
	RoomActorPrefix      = "room."
	SessionActorPrefix   = "session."
	BotActorPrefix       = "bot."
	RoomTopicPrefix      = "room."
	GrainPrefix          = "scrabble.profile."
	LeaderboardKeyPrefix = "scrabble.wins."
)

const (
	PhaseWaiting  = "waiting"
	PhasePlaying  = "playing"
	PhasePaused   = "paused"
	PhaseGameOver = "gameOver"
)

const (
	InTypeStart     = "start"
	InTypeAddBot    = "addBot"
	InTypeRemoveBot = "removeBot"
	InTypePlace     = "place"
	InTypeExchange  = "exchange"
	InTypePass      = "pass"
	InTypePause     = "pause"
	InTypeResume    = "resume"
	InTypeChat      = "chat"
	InTypePlayAgain = "playAgain"
)

const (
	OutTypeJoined   = "joined"
	OutTypeState    = "state"
	OutTypeMove     = "move"
	OutTypeChat     = "chat"
	OutTypeError    = "error"
	OutTypeGameOver = "gameOver"
)

// PlacementWire is one tile placement from the browser. Letter is the
// uppercase rune the player chose for this tile; Blank true means the
// rack tile is a blank assigned to that letter.
type PlacementWire struct {
	Row    int    `json:"row"`
	Col    int    `json:"col"`
	Letter string `json:"letter"`
	Blank  bool   `json:"blank,omitempty"`
}

// WSIn is the single inbound envelope. Type discriminates which fields
// are populated.
type WSIn struct {
	Type       string          `json:"type"`
	Placements []PlacementWire `json:"placements,omitempty"`
	Indices    []int           `json:"indices,omitempty"`
	Text       string          `json:"text,omitempty"`
	Seat       int             `json:"seat,omitempty"`
}

// PlayerView is one entry in the public player list. RackSize is the
// number of tiles the player still holds; the rack contents themselves
// are sent only to that player via StateEvent.YourRack.
type PlayerView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Score    int    `json:"score"`
	RackSize int    `json:"rackSize"`
	Bot      bool   `json:"bot"`
}

// ScoreEntry is one row of a final scoreboard.
type ScoreEntry struct {
	PlayerID string `json:"playerID"`
	Name     string `json:"name"`
	Score    int    `json:"score"`
}

// LeaderboardEntry is one row of the cluster-wide CRDT leaderboard.
type LeaderboardEntry struct {
	PlayerID string `json:"playerID"`
	Name     string `json:"name"`
	Wins     int64  `json:"wins"`
}

// ProfileView is the persistent slice of a player's stats.
type ProfileView struct {
	PlayerID    string `json:"playerID"`
	Name        string `json:"name"`
	GamesPlayed int    `json:"gamesPlayed"`
	Wins        int    `json:"wins"`
	TotalScore  int    `json:"totalScore"`
}

// FormedWordWire is one word formed by a move.
type FormedWordWire struct {
	Word  string `json:"word"`
	Score int    `json:"score"`
}

type JoinedEvent struct {
	For         string             `json:"-"`
	Room        string             `json:"room"`
	Language    string             `json:"language"`
	PlayerID    string             `json:"playerID"`
	Owner       bool               `json:"owner"`
	Profile     ProfileView        `json:"profile"`
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

// StateEvent is the per-player full snapshot. Rack content is in the
// PerRack map keyed by playerID; the session forwards only the entry
// matching its own playerID.
type StateEvent struct {
	For          string              `json:"-"`
	Phase        string              `json:"phase"`
	Board        [][]string          `json:"board"`
	Players      []PlayerView        `json:"players"`
	CurrentID    string              `json:"currentID"`
	OwnerID      string              `json:"ownerID"`
	BagRemaining int                 `json:"bagRemaining"`
	TimerMs      int                 `json:"timerMs"`
	PerRack      map[string][]string `json:"-"`
}

type MoveEvent struct {
	For        string           `json:"-"`
	PlayerID   string           `json:"playerID"`
	Name       string           `json:"name"`
	Placements []PlacementWire  `json:"placements"`
	Words      []FormedWordWire `json:"words"`
	Score      int              `json:"score"`
	NewTotal   int              `json:"newTotal"`
	Bingo      bool             `json:"bingo"`
}

type ChatEvent struct {
	For  string `json:"-"`
	From string `json:"from"`
	Text string `json:"text"`
}

type ErrorEvent struct {
	For     string `json:"-"`
	Message string `json:"message"`
}

type GameOverEvent struct {
	For         string             `json:"-"`
	WinnerID    string             `json:"winnerID"`
	WinnerName  string             `json:"winnerName"`
	Scores      []ScoreEntry       `json:"scores"`
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

// JoinOrCreate is the gateway's Ask to the LobbyActor singleton.
type JoinOrCreate struct {
	Room       string
	Language   string
	PlayerID   string
	PlayerName string
}

type JoinOrCreateResult struct {
	RoomCode string
	RoomName string
	Err      string
}

// PlayerHello is the session's first message to the RoomActor.
type PlayerHello struct {
	PlayerID    string
	Name        string
	SessionName string
}

type GoodbyePlayer struct {
	SessionName string
}

type PlayerInput struct {
	PlayerID string
	In       WSIn
}

// BotPlay is the bot actor's reply to YourTurn — the chosen move, or
// nil placements for a pass.
type BotPlay struct {
	BotID      string
	Placements []PlacementWire
}

// YourTurn is the room → bot tell with the current board/rack snapshot
// the bot should base its move on.
type YourTurn struct {
	BotID string
	Board [][]string
	Rack  []string
}

// Room-internal scheduled messages.
type turnTimeout struct{}
type shutdownRoom struct{}

// Profile-grain wire payloads.
type GetProfile struct{}
type RecordGame struct {
	WonThisGame bool
	GameScore   int
}
