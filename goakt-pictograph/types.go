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

// Game tuning. Round / phase durations are chosen to fit a 3-round game
// into a few minutes — long enough for guesses, short enough that idle
// browsers tear down via inactivity. Stroke coordinates travel as
// normalized [0,1] pairs so the server is resolution-agnostic; the
// browser scales them to whatever canvas size it renders at.
const (
	MinPlayers    = 2
	MaxPlayers    = 8
	DefaultRounds = 3
	DrawSeconds   = 80
	ChooseSeconds = 15
	RoundOverSecs = 5
	GameOverSecs  = 20
	WordChoices   = 3
	BaseScore     = 100
	DrawerBonus   = 50
	MinGuessScore = 10

	// LobbyActorName is the cluster-singleton name of the LobbyActor.
	LobbyActorName = "lobby"

	// RoomActorPrefix + lowercased room code = the cluster-stable
	// RoomActor name (e.g. code "ABCD" → "room.abcd"). Generated codes
	// are 4 chars; collisions retry up to 16 times before falling back
	// to a 6-char code.
	RoomActorPrefix    = "room."
	SessionActorPrefix = "session."

	// RoomTopicPrefix + uppercased room code = the pub/sub topic name
	// the room publishes events to and sessions subscribe to. The
	// uppercased code keeps the topic name aligned with what the
	// browser sees in the URL.
	RoomTopicPrefix = "room."

	// GrainPrefix scopes the player-profile grain names so they can't
	// collide with anything else activated in the system.
	GrainPrefix = "pictograph.profile."

	// LeaderboardKeyPrefix is the CRDT key prefix for per-player win
	// PNCounters. One PNCounter per player; the key carries the player
	// id. All replicators across the cluster converge on the same key.
	LeaderboardKeyPrefix = "pictograph.wins."
)

// Phase strings — used for the wire payload and for logging.
const (
	PhaseWaiting   = "waiting"
	PhaseChoosing  = "choosing"
	PhaseDrawing   = "drawing"
	PhaseRoundOver = "roundOver"
	PhaseGameOver  = "gameOver"
)

// Inbound wire frames (browser → session) share a single envelope:
// Type is the discriminator and the rest of the fields are populated
// depending on Type. Using a single struct rather than a per-type one
// keeps the WS reader cheap (one json.Unmarshal per frame).
const (
	InTypeStroke   = "stroke"
	InTypeClear    = "clear"
	InTypeGuess    = "guess"
	InTypePickWord = "pickWord"
	InTypeRestart  = "restart"
)

type WSIn struct {
	Type   string       `json:"type"`
	Points [][2]float64 `json:"points,omitempty"` // normalized [0,1]
	Color  int          `json:"color,omitempty"`
	Width  int          `json:"width,omitempty"`
	Word   string       `json:"word,omitempty"`
	Text   string       `json:"text,omitempty"`
}

// Outbound events (session → browser). Each is its own Go type so
// cross-node CBOR serialization stays straightforward and the session
// has a clean type switch instead of an `any` envelope. The `For`
// field (when non-empty) restricts delivery to one player id; empty
// means broadcast to every subscriber of the room topic.
const (
	OutTypeJoined      = "joined"
	OutTypeState       = "state"
	OutTypeStroke      = "stroke"
	OutTypeClear       = "clear"
	OutTypeChat        = "chat"
	OutTypeGuessed     = "guessed"
	OutTypeScore       = "score"
	OutTypeWordChoices = "wordChoices"
	OutTypeSecretWord  = "secretWord"
	OutTypeRoundOver   = "roundOver"
	OutTypeGameOver    = "gameOver"
	OutTypeError       = "error"
)

// PlayerView is the public view of a player included in StateEvent and
// score snapshots.
type PlayerView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Score int    `json:"score"`
}

// ScoreEntry is one row of an end-of-round / end-of-game scoreboard.
type ScoreEntry struct {
	PlayerID string `json:"playerID"`
	Name     string `json:"name"`
	Score    int    `json:"score"`
}

// LeaderboardEntry is one row of the cluster-wide CRDT leaderboard.
// Wins are int64 because PNCounter.Value() returns int64.
type LeaderboardEntry struct {
	PlayerID string `json:"playerID"`
	Name     string `json:"name"`
	Wins     int64  `json:"wins"`
}

// ProfileView is the persistent slice of a player's stats returned by
// the PlayerProfileGrain on join.
type ProfileView struct {
	PlayerID    string `json:"playerID"`
	Name        string `json:"name"`
	GamesPlayed int    `json:"gamesPlayed"`
	Wins        int    `json:"wins"`
	TotalScore  int    `json:"totalScore"`
}

type JoinedEvent struct {
	For         string             `json:"-"`
	Room        string             `json:"room"`
	PlayerID    string             `json:"playerID"`
	Profile     ProfileView        `json:"profile"`
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

type StateEvent struct {
	For          string       `json:"-"`
	Phase        string       `json:"phase"`
	Round        int          `json:"round"`
	MaxRounds    int          `json:"maxRounds"`
	TimeLeft     int          `json:"timeLeft"`
	Players      []PlayerView `json:"players"`
	DrawerID     string       `json:"drawerID"`
	WordMask     string       `json:"wordMask"`
	YouAreDrawer bool         `json:"-"` // session fills this in per-recipient
}

type StrokeEvent struct {
	For    string       `json:"-"`
	Points [][2]float64 `json:"points"`
	Color  int          `json:"color"`
	Width  int          `json:"width"`
}

type ClearEvent struct {
	For string `json:"-"`
}

type ChatEvent struct {
	For  string `json:"-"`
	From string `json:"from"`
	Text string `json:"text"`
}

type GuessedEvent struct {
	For      string `json:"-"`
	PlayerID string `json:"playerID"`
	Name     string `json:"name"`
}

type ScoreEvent struct {
	For      string `json:"-"`
	PlayerID string `json:"playerID"`
	Delta    int    `json:"delta"`
	Total    int    `json:"total"`
}

type WordChoicesEvent struct {
	For     string   `json:"-"`
	Choices []string `json:"choices"`
}

type SecretWordEvent struct {
	For  string `json:"-"`
	Word string `json:"word"`
}

type RoundOverEvent struct {
	For    string       `json:"-"`
	Word   string       `json:"word"`
	Scores []ScoreEntry `json:"scores"`
}

type GameOverEvent struct {
	For         string             `json:"-"`
	WinnerID    string             `json:"winnerID"`
	WinnerName  string             `json:"winnerName"`
	Scores      []ScoreEntry       `json:"scores"`
	Leaderboard []LeaderboardEntry `json:"leaderboard"`
}

type ErrorEvent struct {
	For     string `json:"-"`
	Message string `json:"message"`
}

// JoinOrCreate is the gateway's Ask to the LobbyActor singleton: "find
// or create room <Room> and tell me where it lives." Empty Room means
// "create a new one with a fresh code."
type JoinOrCreate struct {
	Room       string
	PlayerID   string
	PlayerName string
}

// JoinOrCreateResult is the LobbyActor's reply. RoomName is the
// cluster-stable actor name; the gateway resolves it via ActorOf so it
// works regardless of which node hosts the room.
type JoinOrCreateResult struct {
	RoomCode string
	RoomName string
	Err      string
}

// PlayerHello is the session's first message to the RoomActor on
// connect: "I'm here, here's who I am, please add me." The room
// identifies the session via ctx.Sender() (which is cluster-aware).
type PlayerHello struct {
	PlayerID    string
	Name        string
	SessionName string // cluster-stable identity used in GoodbyePlayer
}

// GoodbyePlayer is the fast-path cleanup signal from the gateway on a
// graceful WS close. The room also Watches every session PID — that's
// the safety net for ungraceful gateway/session deaths — but a Watch
// notification only arrives after the cluster's failure detector marks
// the peer down, which takes seconds. GoodbyePlayer fires immediately
// so room state shrinks the moment the socket goes away.
type GoodbyePlayer struct {
	SessionName string
}

// PlayerInput is the wrapped browser input forwarded by the session.
// Carrying the player id lets the room validate "only the drawer may
// draw" without having to look up sender → player.
type PlayerInput struct {
	PlayerID string
	In       WSIn
}

// Room-internal scheduled messages.
type tickCountdown struct{}
type autoPickFirst struct{}
type nextRound struct{}
type shutdownRoom struct{}

// Profile-grain wire payloads.
type GetProfile struct{}
type RecordGame struct {
	WonThisGame bool
	GameScore   int
}
