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
	leaderboardOpTimeout = 2 * time.Second

	// LeaderboardExtensionID lets actors locate the shared Leaderboard
	// via ctx.ActorSystem().Extensions() — exactly the same pattern as
	// ProfileStoreExtensionID. SpawnOn re-instantiates actors via the
	// kind registry (`new(T)`), which bypasses constructor-injected
	// deps, so anything load-bearing must be reachable from the system.
	LeaderboardExtensionID = "pictograph_leaderboard"

	// playerSetKey holds the ORSet of all player ids that have ever
	// scored at least one win. We need this index because the CRDT
	// store doesn't enumerate keys by prefix — the only way to know
	// whose PNCounter to read is to keep a side index.
	playerSetKey = "pictograph.players"
)

// Leaderboard wraps the system CRDT Replicator with a per-player win
// counter API. One PNCounter per player at key (LeaderboardKeyPrefix +
// playerID); a single ORSet at playerSetKey enumerates which player ids
// have a counter. Both CRDTs converge across the cluster via the
// Replicator's gossip on the goakt.crdt.deltas topic.
//
// The Replicator path is bypassed and writes go to a process-local
// map under l.mu in any of these cases:
//   - the receiver is nil (no extension registered);
//   - l.system is nil (Bind hasn't been called yet);
//   - l.system.Replicator() is nil (the cluster config did not call WithCRDT).
//
// Display names are kept in a separate process-local map (l.names),
// populated by RememberName on every join. Top() consults this cache
// to render each LeaderboardEntry.Name; the cache is NOT replicated
// across nodes, so a node that has never seen a given player will
// render their id instead of their name.
type Leaderboard struct {
	// system is set after the actor system starts (Bind). nodeID() is
	// derived lazily from it so the Leaderboard can be constructed
	// before the system exists — required because it is registered as
	// an extension at system-build time.
	system actor.ActorSystem

	// mu guards fallback and names. Both maps are process-local and
	// hold no CRDT state.
	mu       sync.Mutex
	fallback map[string]int64  // playerID → wins; only written when the Replicator path is bypassed
	names    map[string]string // playerID → most-recently-seen display name
}

// NewLeaderboard returns an unbound Leaderboard. Call Bind once the
// actor system has started so RecordWin / Top can locate the
// Replicator. Before Bind, both methods are still safe to call —
// they'll silently fall through to the local in-memory map.
func NewLeaderboard() *Leaderboard {
	return &Leaderboard{
		fallback: make(map[string]int64),
		names:    make(map[string]string),
	}
}

// Bind attaches the started actor system. Safe to call exactly once.
func (l *Leaderboard) Bind(system actor.ActorSystem) { l.system = system }

// ID satisfies extension.Extension so the Leaderboard can be looked up
// from inside any actor via ctx.ActorSystem().Extensions().
func (l *Leaderboard) ID() string { return LeaderboardExtensionID }

var _ extension.Extension = (*Leaderboard)(nil)

// leaderboardFromExtension fetches the registered Leaderboard from
// the actor system. Returns nil if none was registered.
func leaderboardFromExtension(system actor.ActorSystem) *Leaderboard {
	for _, ext := range system.Extensions() {
		if ext.ID() == LeaderboardExtensionID {
			if l, ok := ext.(*Leaderboard); ok {
				return l
			}
		}
	}
	return nil
}

// RememberName records the latest known display name for a playerID so
// that Top() can return it without a round-trip to the profile grain.
// Called by the gateway on every join.
//
// Tolerates a nil receiver so misconfigured deployments (no Leaderboard
// extension registered) degrade gracefully instead of crashing.
func (l *Leaderboard) RememberName(playerID, name string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	l.names[playerID] = name
	l.mu.Unlock()
}

// RecordWin (a) increments the player's PNCounter and (b) adds the
// player to the index ORSet. Both go through the Replicator as Asks
// so the next local read sees the write; cross-node convergence is
// gossip-eventual.
//
// The two writes are NOT atomic: if (a) succeeds and (b) fails (timeout
// or context cancel between calls), the counter is incremented but the
// player won't appear in Top() until a subsequent RecordWin re-indexes
// them. Acceptable for the demo — a real deployment would either
// retry, or store the win-count inside an ORMap entry that updates
// atomically with the index.
func (l *Leaderboard) RecordWin(ctx context.Context, playerID, name string) error {
	if l == nil {
		return nil
	}

	l.RememberName(playerID, name)

	if l.system == nil {
		l.mu.Lock()
		l.fallback[playerID]++
		l.mu.Unlock()
		return nil
	}

	rep := l.system.Replicator()
	if rep == nil {
		l.mu.Lock()
		l.fallback[playerID]++
		l.mu.Unlock()
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, leaderboardOpTimeout)
	defer cancel()

	incrUpd := &crdt.Update{
		Key:     crdt.PNCounterKey(LeaderboardKeyPrefix + playerID),
		Initial: crdt.NewPNCounter(),
		Modify: func(d crdt.ReplicatedData) crdt.ReplicatedData {
			return d.(*crdt.PNCounter).Increment(l.nodeID(), 1)
		},
	}

	if _, err := actor.Ask(cctx, rep, incrUpd, leaderboardOpTimeout); err != nil {
		return fmt.Errorf("crdt increment win: %w", err)
	}

	addUpd := &crdt.Update{
		Key:     crdt.ORSetKey(playerSetKey),
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

// Top returns the top-n players by win count, descending. Reads the
// index ORSet, then one PNCounter per indexed player. Player names are
// resolved from RememberName cache; unknown players show their id.
func (l *Leaderboard) Top(ctx context.Context, n int) ([]LeaderboardEntry, error) {
	if l == nil {
		return nil, nil
	}

	if l.system == nil {
		return l.topFromFallback(n), nil
	}

	rep := l.system.Replicator()
	if rep == nil {
		return l.topFromFallback(n), nil
	}

	cctx, cancel := context.WithTimeout(ctx, leaderboardOpTimeout)
	defer cancel()

	resp, err := actor.Ask(cctx, rep, &crdt.Get{Key: crdt.ORSetKey(playerSetKey)}, leaderboardOpTimeout)
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
			&crdt.Get{Key: crdt.PNCounterKey(LeaderboardKeyPrefix + pid)},
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
	if n, ok := l.names[pid]; ok {
		return n
	}
	return pid
}

func (l *Leaderboard) topFromFallback(n int) []LeaderboardEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]LeaderboardEntry, 0, len(l.fallback))
	for pid, wins := range l.fallback {
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
