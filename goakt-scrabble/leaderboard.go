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
	"sort"
	"sync"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/crdt"
	"github.com/tochemey/goakt/v4/extension"
)

const (
	leaderboardOpTimeout   = 2 * time.Second
	LeaderboardExtensionID = "scrabble_leaderboard"

	// playerSetKeyPrefix indexes which player ids hold a counter for
	// each language: "scrabble.players.en", "scrabble.players.fr", etc.
	playerSetKeyPrefix = "scrabble.players."
)

// Leaderboard tracks per-(playerID, language) win counts via CRDT
// PNCounters and a per-language ORSet index. Falls back to a process-
// local map if the cluster's CRDT replicator is unavailable.
type Leaderboard struct {
	system actor.ActorSystem

	mu       sync.Mutex
	fallback map[string]int64
	names    map[string]string
}

var _ extension.Extension = (*Leaderboard)(nil)

func NewLeaderboard() *Leaderboard {
	return &Leaderboard{
		fallback: make(map[string]int64),
		names:    make(map[string]string),
	}
}

func (l *Leaderboard) Bind(system actor.ActorSystem) { l.system = system }

func (l *Leaderboard) ID() string { return LeaderboardExtensionID }

func leaderboardFromExtension(system actor.ActorSystem) *Leaderboard {
	for _, ext := range system.Extensions() {
		if ext.ID() == LeaderboardExtensionID {
			if leaderboard, ok := ext.(*Leaderboard); ok {
				return leaderboard
			}
		}
	}

	return nil
}

// RememberName records the latest known display name for a player so
// that Top can return it without contacting the profile grain.
func (l *Leaderboard) RememberName(playerID, name string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	l.names[playerID] = name
	l.mu.Unlock()
}

// RecordWin increments the per-(player, language) PNCounter and adds
// the player to the per-language ORSet index.
func (l *Leaderboard) RecordWin(ctx context.Context, language, playerID, name string) error {
	if l == nil {
		return nil
	}

	l.RememberName(playerID, name)

	if l.system == nil || l.system.Replicator() == nil {
		l.mu.Lock()
		l.fallback[fallbackKey(language, playerID)]++
		l.mu.Unlock()
		return nil
	}

	rep := l.system.Replicator()
	cctx, cancel := context.WithTimeout(ctx, leaderboardOpTimeout)
	defer cancel()

	incrUpd := &crdt.Update{
		Key:     crdt.PNCounterKey(counterKey(language, playerID)),
		Initial: crdt.NewPNCounter(),
		Modify: func(d crdt.ReplicatedData) crdt.ReplicatedData {
			return d.(*crdt.PNCounter).Increment(l.nodeID(), 1)
		},
	}

	if _, err := actor.Ask(cctx, rep, incrUpd, leaderboardOpTimeout); err != nil {
		return fmt.Errorf("crdt increment win: %w", err)
	}

	addUpd := &crdt.Update{
		Key:     crdt.ORSetKey(indexKey(language)),
		Initial: crdt.NewORSet(),
		Modify: func(d crdt.ReplicatedData) crdt.ReplicatedData {
			return d.(*crdt.ORSet).Add(l.nodeID(), playerID)
		},
	}

	if _, err := actor.Ask(cctx, rep, addUpd, leaderboardOpTimeout); err != nil {
		return fmt.Errorf("crdt index player: %w", err)
	}

	return nil
}

// Top returns the top-n players for a language by win count, descending.
func (l *Leaderboard) Top(ctx context.Context, language string, n int) ([]LeaderboardEntry, error) {
	if l == nil {
		return nil, nil
	}

	if l.system == nil || l.system.Replicator() == nil {
		return l.topFromFallback(language, n), nil
	}

	rep := l.system.Replicator()
	cctx, cancel := context.WithTimeout(ctx, leaderboardOpTimeout)
	defer cancel()

	resp, err := actor.Ask(cctx, rep, &crdt.Get{Key: crdt.ORSetKey(indexKey(language))}, leaderboardOpTimeout)
	if err != nil {
		return nil, fmt.Errorf("crdt get player set: %w", err)
	}

	setResp, ok := resp.(*crdt.GetResponse)
	if !ok || setResp.Data == nil {
		return nil, nil
	}
	set := setResp.Data.(*crdt.ORSet)

	ids := set.Elements()
	entries := make([]LeaderboardEntry, 0, len(ids))

	for _, raw := range ids {
		pid, ok := raw.(string)
		if !ok {
			continue
		}

		cresp, err := actor.Ask(cctx, rep,
			&crdt.Get{Key: crdt.PNCounterKey(counterKey(language, pid))},
			leaderboardOpTimeout)
		if err != nil {
			continue
		}

		cnt, _ := cresp.(*crdt.GetResponse)
		var wins int64
		if cnt != nil && cnt.Data != nil {
			wins = cnt.Data.(*crdt.PNCounter).Value()
		}

		entries = append(entries, LeaderboardEntry{
			PlayerID: pid,
			Name:     l.nameFor(pid),
			Wins:     wins,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Wins > entries[j].Wins })

	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}

	return entries, nil
}

func (l *Leaderboard) nameFor(pid string) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if name, ok := l.names[pid]; ok {
		return name
	}

	return pid
}

func (l *Leaderboard) topFromFallback(language string, n int) []LeaderboardEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	prefix := language + "|"
	entries := make([]LeaderboardEntry, 0, len(l.fallback))

	for key, wins := range l.fallback {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		pid := key[len(prefix):]
		entries = append(entries, LeaderboardEntry{
			PlayerID: pid,
			Name:     l.names[pid],
			Wins:     wins,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Wins > entries[j].Wins })

	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}

	return entries
}

func (l *Leaderboard) nodeID() string {
	if l.system == nil {
		return "local"
	}

	return fmt.Sprintf("%s:%d", l.system.Host(), l.system.Port())
}

func counterKey(language, playerID string) string {
	return LeaderboardKeyPrefix + language + "." + playerID
}

func indexKey(language string) string {
	return playerSetKeyPrefix + language
}

func fallbackKey(language, playerID string) string {
	return language + "|" + playerID
}
